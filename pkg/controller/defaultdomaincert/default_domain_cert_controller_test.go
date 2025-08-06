package defaultdomaincert

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/store"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ErrorClient wraps a client to inject errors for testing
type ErrorClient struct {
	client.Client
	GetError    error
	CreateError error
	UpdateError error
	DeleteError error
	PatchError  error
}

func (e *ErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if e.GetError != nil {
		return e.GetError
	}
	return e.Client.Get(ctx, key, obj, opts...)
}

func (e *ErrorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if e.CreateError != nil {
		return e.CreateError
	}
	return e.Client.Create(ctx, obj, opts...)
}

func (e *ErrorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if e.UpdateError != nil {
		return e.UpdateError
	}
	return e.Client.Update(ctx, obj, opts...)
}

func (e *ErrorClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if e.PatchError != nil {
		return e.PatchError
	}
	return e.Client.Patch(ctx, obj, patch, opts...)
}

func (e *ErrorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if e.DeleteError != nil {
		return e.DeleteError
	}
	return e.Client.Delete(ctx, obj, opts...)
}

const (
	testNamespace  = "test-namespace"
	testSecretName = "test-secret"
	testCertPath   = "/path/to/cert.crt"
	testKeyPath    = "/path/to/key.key"
)

func generateTestCertificate(t *testing.T) ([]byte, []byte) {
	// Generate a private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	subject := pkix.Name{
		Country:      []string{"US"},
		Organization: []string{"Test Org"},
		CommonName:   "test.example.com",
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               subject,
		Issuer:                subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0), // Valid for 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"example.com", "www.example.com"},
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to marshal private key: %v", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	return certPEM, keyPEM
}

// mockStore implements the store.Store interface for testing
type mockStore struct {
	files         map[string][]byte
	addFileErr    error
	shouldExist   map[string]bool
	addFileCalls  int
	addFileKeyErr error // Error to return specifically for key file (second call)
}

func newMockStore() *mockStore {
	return &mockStore{
		files:       make(map[string][]byte),
		shouldExist: make(map[string]bool),
	}
}

func (m *mockStore) AddFile(path string) error {
	m.addFileCalls++
	if m.addFileErr != nil {
		return m.addFileErr
	}
	// If this is the second call and we have a specific key error, return it
	if m.addFileCalls == 2 && m.addFileKeyErr != nil {
		return m.addFileKeyErr
	}
	m.shouldExist[path] = true
	return nil
}

func (m *mockStore) GetContent(path string) ([]byte, bool) {
	if content, exists := m.files[path]; exists && m.shouldExist[path] {
		return content, true
	}
	return nil, false
}

func (m *mockStore) RotationEvents() <-chan store.RotationEvent {
	ch := make(chan store.RotationEvent)
	close(ch)
	return ch
}

func (m *mockStore) Errors() <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}

func (m *mockStore) setFileContent(path string, content []byte) {
	m.files[path] = content
	m.shouldExist[path] = true
}

// mockStoreWithRotationEvents extends mockStore to support rotation events
type mockStoreWithRotationEvents struct {
	*mockStore
	rotationCh chan store.RotationEvent
	errorCh    chan error
}

func newMockStoreWithRotationEvents() *mockStoreWithRotationEvents {
	return &mockStoreWithRotationEvents{
		mockStore:  newMockStore(),
		rotationCh: make(chan store.RotationEvent, 10),
		errorCh:    make(chan error, 10),
	}
}

func (m *mockStoreWithRotationEvents) RotationEvents() <-chan store.RotationEvent {
	return m.rotationCh
}

func (m *mockStoreWithRotationEvents) Errors() <-chan error {
	return m.errorCh
}

func (m *mockStoreWithRotationEvents) sendRotationEvent(path string) {
	m.rotationCh <- store.RotationEvent{Path: path}
}

func createTestReconciler(client client.Client, store store.Store) *defaultDomainCertControllerReconciler {
	conf := &config.Config{
		DefaultDomainCertPath: testCertPath,
		DefaultDomainKeyPath:  testKeyPath,
	}

	return &defaultDomainCertControllerReconciler{
		client: client,
		events: &record.FakeRecorder{},
		conf:   conf,
		store:  store,
	}
}

func createTestDefaultDomainCertificate(name, namespace, secretName string) *approutingv1alpha1.DefaultDomainCertificate {
	var secretPtr *string
	if secretName != "" {
		secretPtr = &secretName
	}
	// If secretName is empty string, secretPtr remains nil

	return &approutingv1alpha1.DefaultDomainCertificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "DefaultDomainCertificate",
			APIVersion: approutingv1alpha1.GroupVersion.String(),
		},
		Spec: approutingv1alpha1.DefaultDomainCertificateSpec{
			Target: approutingv1alpha1.DefaultDomainCertificateTarget{
				Secret: secretPtr,
			},
		},
	}
}

func TestReconcile_SuccessfulReconciliation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc).
		WithStatusSubresource(ddc).
		Build()

	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(client, mockStore)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify secret was created
	var secret corev1.Secret
	err = client.Get(ctx, types.NamespacedName{Name: testSecretName, Namespace: testNamespace}, &secret)
	require.NoError(t, err)

	assert.Equal(t, testSecretName, secret.Name)
	assert.Equal(t, testNamespace, secret.Namespace)
	assert.Equal(t, corev1.SecretTypeTLS, secret.Type)
	assert.Equal(t, []byte(cert), secret.Data["tls.crt"])
	assert.Equal(t, []byte(key), secret.Data["tls.key"])
	assert.Equal(t, manifests.GetTopLevelLabels(), secret.Labels)

	// Verify owner references
	assert.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, ddc.Name, secret.OwnerReferences[0].Name)
	assert.Equal(t, ddc.UID, secret.OwnerReferences[0].UID)
	assert.True(t, *secret.OwnerReferences[0].Controller)

	// Verify DefaultDomainCertificate status was updated
	require.NoError(t, client.Get(ctx, types.NamespacedName{Name: ddc.Name, Namespace: ddc.Namespace}, ddc))
	assert.Equal(t, metav1.ConditionTrue, ddc.GetCondition(v1alpha1.DefaultDomainCertificateConditionTypeAvailable).Status)
}

func TestReconcile_DefaultDomainCertificateNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mockStore := newMockStore()
	reconciler := createTestReconciler(client, mockStore)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_ErrorGettingDefaultDomainCertificate(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))

	// Create a client that will return an error other than NotFound
	client := &ErrorClient{
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		GetError: errors.New("internal server error"),
	}

	mockStore := newMockStore()
	reconciler := createTestReconciler(client, mockStore)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal server error")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_NoTargetSecretSpecified(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, "")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc).
		Build()

	mockStore := newMockStore()
	reconciler := createTestReconciler(client, mockStore)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DefaultDomainCertificate has no target secret specified")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_FailedToGetSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc).
		Build()

	_, key := generateTestCertificate(t)

	mockStore := newMockStore()
	// Don't set cert content, should cause error
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(client, mockStore)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generating Secret for DefaultDomainCertificate: getting and verifying cert and key: failed to get default domain cert from store")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_FailedToUpsertSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	// Create a client that will fail on Patch operations (which Upsert uses)
	client := &ErrorClient{
		Client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(ddc).Build(),
		PatchError: errors.New("failed to patch secret"),
	}

	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(client, mockStore)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to patch secret")
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_FailedToUpsertSecret_RecordsEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	// Create a client that will fail on Patch operations (which Upsert uses)
	client := &ErrorClient{
		Client:     fake.NewClientBuilder().WithScheme(scheme).WithObjects(ddc).Build(),
		PatchError: errors.New("failed to patch secret"),
	}

	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	// Use a fake event recorder to capture events
	fakeRecorder := &record.FakeRecorder{Events: make(chan string, 10)}

	reconciler := &defaultDomainCertControllerReconciler{
		client: client,
		events: fakeRecorder,
		conf: &config.Config{
			DefaultDomainCertPath: testCertPath,
			DefaultDomainKeyPath:  testKeyPath,
		},
		store: mockStore,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to patch secret")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify that the warning event was recorded
	select {
	case event := <-fakeRecorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, "ApplyingCertificateSecretFailed")
		assert.Contains(t, event, "Failed to apply Secret for DefaultDomainCertificate")
		assert.Contains(t, event, "failed to patch secret")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected event was not recorded within timeout")
	}
}

func TestReconcile_SecretAlreadyExists_UpdatesExistingSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	// Create an existing secret with different content
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNamespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte("old cert content"),
			"tls.key": []byte("old key content"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc, existingSecret).
		WithStatusSubresource(ddc).
		Build()

	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(client, mockStore)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify secret was updated with new content
	var secret corev1.Secret
	err = client.Get(ctx, types.NamespacedName{Name: testSecretName, Namespace: testNamespace}, &secret)
	require.NoError(t, err)

	assert.Equal(t, testSecretName, secret.Name)
	assert.Equal(t, testNamespace, secret.Namespace)
	assert.Equal(t, corev1.SecretTypeTLS, secret.Type)
	assert.Equal(t, []byte(cert), secret.Data["tls.crt"])
	assert.Equal(t, []byte(key), secret.Data["tls.key"])
}

func TestReconcile_StatusUpdateFails(t *testing.T) {
	// Test that error is returned when status update fails
	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ddc).Build()

	// Create error client that fails on status updates
	errorClient := &ErrorClient{
		Client: baseClient,
		// Allow Get and Create/Update for secret to succeed, but fail status update
	}

	// We need a custom client that wraps the status client specifically
	statusClient := &StatusErrorClient{
		Client:            errorClient,
		StatusUpdateError: errors.New("status update failed"),
	}

	reconciler := createTestReconciler(statusClient, mockStore)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      ddc.Name,
			Namespace: ddc.Namespace,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)

	// Should return error when status update fails
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status update failed")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify that the secret was still created successfully
	secret := &corev1.Secret{}
	err = baseClient.Get(ctx, types.NamespacedName{
		Name:      testSecretName,
		Namespace: testNamespace,
	}, secret)
	require.NoError(t, err)
}

func TestReconcile_MultipleStatusConditionUpdates(t *testing.T) {
	// Test multiple reconciles to ensure status conditions are updated correctly
	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)
	ddc.Generation = 1

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc).
		WithStatusSubresource(ddc).
		Build()
	reconciler := createTestReconciler(client, mockStore)

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      ddc.Name,
			Namespace: ddc.Namespace,
		},
	}

	// First reconcile
	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Get the updated object
	updatedDDC := &approutingv1alpha1.DefaultDomainCertificate{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      ddc.Name,
		Namespace: ddc.Namespace,
	}, updatedDDC)
	require.NoError(t, err)

	firstCondition := updatedDDC.GetCondition(approutingv1alpha1.DefaultDomainCertificateConditionTypeAvailable)
	require.NotNil(t, firstCondition)
	firstLastTransitionTime := firstCondition.LastTransitionTime

	// Update generation to simulate a change
	updatedDDC.Generation = 2
	err = client.Update(ctx, updatedDDC)
	require.NoError(t, err)

	// Second reconcile
	result, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Get the updated object again
	finalDDC := &approutingv1alpha1.DefaultDomainCertificate{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      ddc.Name,
		Namespace: ddc.Namespace,
	}, finalDDC)
	require.NoError(t, err)

	secondCondition := finalDDC.GetCondition(approutingv1alpha1.DefaultDomainCertificateConditionTypeAvailable)
	require.NotNil(t, secondCondition)

	// ObservedGeneration should be updated
	assert.Equal(t, int64(2), secondCondition.ObservedGeneration)

	// LastTransitionTime should be updated since generation changed
	assert.True(t, secondCondition.LastTransitionTime.After(firstLastTransitionTime.Time) ||
		secondCondition.LastTransitionTime.Equal(&firstLastTransitionTime))
}

func TestGenerateSecret_SuccessfulSecretCreation(t *testing.T) {
	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)
	ddc.UID = "test-uid"

	secret, err := reconciler.generateSecret(ddc)

	require.NoError(t, err)
	require.NotNil(t, secret)

	assert.Equal(t, testSecretName, secret.Name)
	assert.Equal(t, testNamespace, secret.Namespace)
	assert.Equal(t, corev1.SecretTypeTLS, secret.Type)
	assert.Equal(t, []byte(cert), secret.Data["tls.crt"])
	assert.Equal(t, []byte(key), secret.Data["tls.key"])
	assert.Equal(t, manifests.GetTopLevelLabels(), secret.Labels)

	// Verify owner references
	assert.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, ddc.Name, secret.OwnerReferences[0].Name)
	assert.Equal(t, ddc.UID, secret.OwnerReferences[0].UID)
	assert.True(t, *secret.OwnerReferences[0].Controller)
}

func TestGenerateSecret_CertificateNotFoundInStore(t *testing.T) {
	_, key := generateTestCertificate(t)

	mockStore := newMockStore()
	// Only set key content, not cert content
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	secret, err := reconciler.generateSecret(ddc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting and verifying cert and key: failed to get default domain cert from store")
	assert.Nil(t, secret)
}

func TestGenerateSecret_KeyNotFoundInStore(t *testing.T) {
	cert, _ := generateTestCertificate(t)

	mockStore := newMockStore()
	// Only set cert content, not key content
	mockStore.setFileContent(testCertPath, []byte(cert))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	secret, err := reconciler.generateSecret(ddc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting and verifying cert and key: failed to get default domain key from store")
	assert.Nil(t, secret)
}

func TestGenerateSecret_CertificateContentIsNil(t *testing.T) {
	_, key := generateTestCertificate(t)

	mockStore := newMockStore()
	// Set files to exist but with nil content
	mockStore.files[testCertPath] = nil
	mockStore.shouldExist[testCertPath] = true
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	secret, err := reconciler.generateSecret(ddc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting and verifying cert and key: failed to get default domain cert from store")
	assert.Nil(t, secret)
}

func TestGenerateSecret_KeyContentIsNil(t *testing.T) {
	cert, _ := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	// Set key file to exist but with nil content
	mockStore.files[testKeyPath] = nil
	mockStore.shouldExist[testKeyPath] = true

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	secret, err := reconciler.generateSecret(ddc)

	require.Error(t, err)
	require.Contains(t, err.Error(), "getting and verifying cert and key: failed to get default domain key from store")
	require.Nil(t, secret)
}

func TestGenerateSecret_EmptyNamespace(t *testing.T) {
	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", "", testSecretName)

	secret, err := reconciler.generateSecret(ddc)

	require.NoError(t, err)
	require.NotNil(t, secret)

	assert.Equal(t, testSecretName, secret.Name)
	assert.Equal(t, "", secret.Namespace)
}

func TestGenerateSecret_ValidatesOwnerReferences(t *testing.T) {
	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)
	ddc.UID = "test-uid-12345"
	// Set TypeMeta properly so GetOwnerRefs can extract the GVK
	ddc.TypeMeta = metav1.TypeMeta{
		APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
		Kind:       "DefaultDomainCertificate",
	}

	secret, err := reconciler.generateSecret(ddc)

	require.NoError(t, err)
	require.NotNil(t, secret)

	// Verify owner references are set correctly
	require.Len(t, secret.OwnerReferences, 1)
	ownerRef := secret.OwnerReferences[0]
	assert.Equal(t, "approuting.kubernetes.azure.com/v1alpha1", ownerRef.APIVersion)
	assert.Equal(t, "DefaultDomainCertificate", ownerRef.Kind)
	assert.Equal(t, ddc.Name, ownerRef.Name)
	assert.Equal(t, ddc.UID, ownerRef.UID)
	assert.True(t, *ownerRef.Controller)
	// Note: BlockOwnerDeletion is not set by GetOwnerRefs function
	assert.Nil(t, ownerRef.BlockOwnerDeletion)
}

func TestGenerateSecret_LargeFileContent(t *testing.T) {
	// Test with larger certificate/key content to ensure no size limitations
	largeCertContent := strings.Repeat("LARGE CERT CONTENT ", 1000)
	largeKeyContent := strings.Repeat("LARGE KEY CONTENT ", 1000)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(largeCertContent))
	mockStore.setFileContent(testKeyPath, []byte(largeKeyContent))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	secret, err := reconciler.generateSecret(ddc)

	require.Contains(t, err.Error(), "getting and verifying cert and key: validating cert and key: failed to decode PEM certificate block")
	require.Nil(t, secret)
}

func TestGenerateSecret_SpecialCharactersInContent(t *testing.T) {
	// Test with special characters, unicode, etc.
	specialCertContent := "-----BEGIN CERTIFICATE-----\n测试特殊字符\n🔒🔑\n-----END CERTIFICATE-----"
	specialKeyContent := "-----BEGIN PRIVATE KEY-----\nñáéíóú\n\x00\x01\x02\n-----END PRIVATE KEY-----"

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(specialCertContent))
	mockStore.setFileContent(testKeyPath, []byte(specialKeyContent))

	reconciler := createTestReconciler(nil, mockStore)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	secret, err := reconciler.generateSecret(ddc)

	require.Contains(t, err.Error(), "getting and verifying cert and key: validating cert and key: failed to decode PEM certificate block")
	require.Nil(t, secret)
}

func TestNewReconciler_AddCertFileError(t *testing.T) {
	mockStore := newMockStore()
	mockStore.addFileErr = errors.New("failed to add cert file")

	conf := &config.Config{
		DefaultDomainCertPath: testCertPath,
		DefaultDomainKeyPath:  testKeyPath,
	}

	err := NewReconciler(conf, &testutils.FakeManager{}, mockStore)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "adding default domain cert")
	assert.Contains(t, err.Error(), "failed to add cert file")
}

func TestNewReconciler_AddKeyFileError(t *testing.T) {
	mockStore := newMockStore()
	// Set specific error for the key file (second AddFile call)
	mockStore.addFileKeyErr = errors.New("failed to add key file")

	conf := &config.Config{
		DefaultDomainCertPath: testCertPath,
		DefaultDomainKeyPath:  testKeyPath,
	}

	err := NewReconciler(conf, &testutils.FakeManager{}, mockStore)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "adding default domain key")
	assert.Contains(t, err.Error(), "failed to add key file")
}

// StatusErrorClient wraps a client to inject errors specifically for status updates
type StatusErrorClient struct {
	client.Client
	StatusUpdateError error
}

func (s *StatusErrorClient) Status() client.StatusWriter {
	return &StatusErrorWriter{
		StatusWriter: s.Client.Status(),
		UpdateError:  s.StatusUpdateError,
	}
}

// StatusErrorWriter wraps a status writer to inject update errors
type StatusErrorWriter struct {
	client.StatusWriter
	UpdateError error
}

func (s *StatusErrorWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if s.UpdateError != nil {
		return s.UpdateError
	}
	return s.StatusWriter.Update(ctx, obj, opts...)
}

func (s *StatusErrorWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if s.UpdateError != nil {
		return s.UpdateError
	}
	return s.StatusWriter.Patch(ctx, obj, patch, opts...)
}

func TestGetAndVerifyCertAndKeySuccess(t *testing.T) {
	cert, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))
	reconciler := createTestReconciler(nil, mockStore)

	certContent, keyContent, err := reconciler.getAndVerifyCertAndKey()
	require.NoError(t, err)
	require.NotNil(t, certContent)
	require.NotNil(t, keyContent)
	assert.Equal(t, []byte(cert), certContent)
	assert.Equal(t, []byte(key), keyContent)
}

func TestGetAndVerifyCertAndKeyNonCert(t *testing.T) {
	cert, key := generateTestCertificate(t)
	cert = []byte("non-cert content")

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	mockStore.setFileContent(testKeyPath, []byte(key))
	reconciler := createTestReconciler(nil, mockStore)

	_, _, err := reconciler.getAndVerifyCertAndKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM certificate block")
}

func TestGetAndVerifyCertAndKeyMissingKey(t *testing.T) {
	cert, _ := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testCertPath, []byte(cert))
	reconciler := createTestReconciler(nil, mockStore)

	_, _, err := reconciler.getAndVerifyCertAndKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get default domain key from store")
}

func TestGetAndVerifyCertAndKeyMissingCert(t *testing.T) {
	_, key := generateTestCertificate(t)

	mockStore := newMockStore()
	mockStore.setFileContent(testKeyPath, []byte(key))
	reconciler := createTestReconciler(nil, mockStore)
	_, _, err := reconciler.getAndVerifyCertAndKey()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get default domain cert from store")
}

func TestSendRotationEvents_CertificateFileRotation(t *testing.T) {
	// Create fake client with scheme
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))

	// Create test DefaultDomainCertificate resources
	ddc1 := &approutingv1alpha1.DefaultDomainCertificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert-1",
			Namespace: testNamespace,
		},
		Spec: approutingv1alpha1.DefaultDomainCertificateSpec{
			Target: approutingv1alpha1.DefaultDomainCertificateTarget{
				Secret: util.ToPtr("test-secret-1"),
			},
		},
	}

	ddc2 := &approutingv1alpha1.DefaultDomainCertificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert-2",
			Namespace: testNamespace,
		},
		Spec: approutingv1alpha1.DefaultDomainCertificateSpec{
			Target: approutingv1alpha1.DefaultDomainCertificateTarget{
				Secret: util.ToPtr("test-secret-2"),
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc1, ddc2).
		Build()

	store := newMockStoreWithRotationEvents()
	reconciler := createTestReconciler(client, store)

	// Create a test context and work queue
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a mock queue to capture reconcile requests
	queue := &mockWorkQueue{}

	// Start sendRotationEvents in a goroutine
	errCh := make(chan error, 1)
	go func() {
		err := reconciler.sendRotationEvents(ctx, queue)
		errCh <- err
	}()

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send a rotation event for the certificate file
	store.sendRotationEvent(testCertPath)

	// Wait for the event to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that reconcile requests were added to the queue for both DDCs
	assert.Eventually(t, func() bool {
		return len(queue.requests) >= 2
	}, 1*time.Second, 50*time.Millisecond, "Expected 2 reconcile requests to be queued")

	// Check that the correct requests were queued
	expectedRequests := map[types.NamespacedName]bool{
		{Name: "test-cert-1", Namespace: testNamespace}: false,
		{Name: "test-cert-2", Namespace: testNamespace}: false,
	}

	for _, req := range queue.requests {
		if _, exists := expectedRequests[req.NamespacedName]; exists {
			expectedRequests[req.NamespacedName] = true
		}
	}

	for name, found := range expectedRequests {
		assert.True(t, found, "Expected reconcile request for %s to be queued", name)
	}

	// Cancel context and verify goroutine exits
	cancel()
	select {
	case err := <-errCh:
		// Expected - goroutine should exit when context is cancelled
		require.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("sendRotationEvents goroutine did not exit after context cancellation")
	}
}

func TestSendRotationEvents_KeyFileRotation(t *testing.T) {
	// Create fake client with scheme
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))

	// Create test DefaultDomainCertificate resource
	ddc := &approutingv1alpha1.DefaultDomainCertificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert",
			Namespace: testNamespace,
		},
		Spec: approutingv1alpha1.DefaultDomainCertificateSpec{
			Target: approutingv1alpha1.DefaultDomainCertificateTarget{
				Secret: util.ToPtr("test-secret"),
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc).
		Build()

	store := newMockStoreWithRotationEvents()
	reconciler := createTestReconciler(client, store)

	// Create a test context and work queue
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queue := &mockWorkQueue{}

	// Start sendRotationEvents in a goroutine
	errCh := make(chan error, 1)
	go func() {
		err := reconciler.sendRotationEvents(ctx, queue)
		errCh <- err
	}()

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send a rotation event for the key file
	store.sendRotationEvent(testKeyPath)

	// Wait for the event to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that a reconcile request was added to the queue
	assert.Eventually(t, func() bool {
		return len(queue.requests) >= 1
	}, 1*time.Second, 50*time.Millisecond, "Expected 1 reconcile request to be queued")

	// Check that the correct request was queued
	assert.Equal(t, "test-cert", queue.requests[0].NamespacedName.Name)
	assert.Equal(t, testNamespace, queue.requests[0].NamespacedName.Namespace)

	// Cancel context and verify goroutine exits
	cancel()
	select {
	case err := <-errCh:
		// Expected - goroutine should exit when context is cancelled
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("sendRotationEvents goroutine did not exit after context cancellation")
	}
}

func TestSendRotationEvents_NonCertificateFileIgnored(t *testing.T) {
	// Create fake client with scheme
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))

	// Create test DefaultDomainCertificate resource
	ddc := &approutingv1alpha1.DefaultDomainCertificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert",
			Namespace: testNamespace,
		},
		Spec: approutingv1alpha1.DefaultDomainCertificateSpec{
			Target: approutingv1alpha1.DefaultDomainCertificateTarget{
				Secret: util.ToPtr("test-secret"),
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc).
		Build()

	store := newMockStoreWithRotationEvents()
	reconciler := createTestReconciler(client, store)

	// Create a test context and work queue
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queue := &mockWorkQueue{}

	// Start sendRotationEvents in a goroutine
	errCh := make(chan error, 1)
	go func() {
		err := reconciler.sendRotationEvents(ctx, queue)
		errCh <- err
	}()

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send rotation events for non-certificate files
	store.sendRotationEvent("/some/other/file.txt")
	store.sendRotationEvent("/path/to/config.yaml")

	// Wait a moment to ensure events are processed
	time.Sleep(200 * time.Millisecond)

	// Verify that no reconcile requests were added to the queue
	assert.Empty(t, queue.requests, "Expected no reconcile requests for non-certificate files")

	// Cancel context and verify goroutine exits
	cancel()
	select {
	case err := <-errCh:
		// Expected - goroutine should exit when context is cancelled
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("sendRotationEvents goroutine did not exit after context cancellation")
	}
}

func TestSendRotationEvents_ListError(t *testing.T) {
	// Create a fake client that will return an error when listing DDCs
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))

	client := &ErrorClient{
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		GetError: errors.New("list error"),
	}

	store := newMockStoreWithRotationEvents()
	reconciler := createTestReconciler(client, store)

	// Create a test context and work queue
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queue := &mockWorkQueue{}

	// Start sendRotationEvents in a goroutine
	errCh := make(chan error, 1)
	go func() {
		err := reconciler.sendRotationEvents(ctx, queue)
		errCh <- err
	}()

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send a rotation event for the certificate file
	store.sendRotationEvent(testCertPath)

	// Wait for the event to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify that no reconcile requests were added (due to list error)
	assert.Empty(t, queue.requests, "Expected no reconcile requests when list fails")

	// Cancel context and verify goroutine exits
	cancel()
	select {
	case err := <-errCh:
		// Expected - goroutine should exit when context is cancelled
		require.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("sendRotationEvents goroutine did not exit after context cancellation")
	}
}

func TestSendRotationEvents_ContextCancellation(t *testing.T) {
	// Create fake client with scheme
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	store := newMockStoreWithRotationEvents()
	reconciler := createTestReconciler(client, store)

	// Create a test context and work queue
	ctx, cancel := context.WithCancel(context.Background())
	queue := &mockWorkQueue{}

	// Start sendRotationEvents in a goroutine
	errCh := make(chan error, 1)
	go func() {
		err := reconciler.sendRotationEvents(ctx, queue)
		errCh <- err
	}()

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel the context immediately
	cancel()

	// Verify that the goroutine exits gracefully
	select {
	case err := <-errCh:
		// Expected - goroutine should exit when context is cancelled
		require.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("sendRotationEvents goroutine did not exit after context cancellation")
	}

	// Verify no requests were queued
	assert.Empty(t, queue.requests, "Expected no reconcile requests after context cancellation")
}

func TestSendRotationEvents_EmptyDDCList(t *testing.T) {
	// Create fake client with no DDCs
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	store := newMockStoreWithRotationEvents()
	reconciler := createTestReconciler(client, store)

	// Create a test context and work queue
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	queue := &mockWorkQueue{}

	// Start sendRotationEvents in a goroutine
	errCh := make(chan error, 1)
	go func() {
		err := reconciler.sendRotationEvents(ctx, queue)
		errCh <- err
	}()

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Send a rotation event for the certificate file
	store.sendRotationEvent(testCertPath)

	// Wait for the event to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that no reconcile requests were added (no DDCs exist)
	assert.Empty(t, queue.requests, "Expected no reconcile requests when no DDCs exist")

	// Cancel context and verify goroutine exits
	cancel()
	select {
	case err := <-errCh:
		// Expected - goroutine should exit when context is cancelled
		require.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("sendRotationEvents goroutine did not exit after context cancellation")
	}
}

// mockWorkQueue implements workqueue.TypedRateLimitingInterface for testing
type mockWorkQueue struct {
	requests []reconcile.Request
	mu       sync.Mutex
}

func (m *mockWorkQueue) Add(item reconcile.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, item)
}

func (m *mockWorkQueue) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func (m *mockWorkQueue) Get() (item reconcile.Request, shutdown bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		return reconcile.Request{}, false
	}
	item = m.requests[0]
	m.requests = m.requests[1:]
	return item, false
}

func (m *mockWorkQueue) Done(item reconcile.Request) {}

func (m *mockWorkQueue) ShutDown() {}

func (m *mockWorkQueue) ShutDownWithDrain() {}

func (m *mockWorkQueue) ShuttingDown() bool {
	return false
}

func (m *mockWorkQueue) AddAfter(item reconcile.Request, duration time.Duration) {
	m.Add(item)
}

func (m *mockWorkQueue) AddRateLimited(item reconcile.Request) {
	m.Add(item)
}

func (m *mockWorkQueue) Forget(item reconcile.Request) {}

func (m *mockWorkQueue) NumRequeues(item reconcile.Request) int {
	return 0
}
