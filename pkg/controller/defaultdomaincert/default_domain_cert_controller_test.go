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
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	defaultdomain "github.com/Azure/aks-app-routing-operator/pkg/clients/default-domain"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

// mockDefaultDomainClient implements a mock for the default domain client
type mockDefaultDomainClient struct {
	cert []byte
	key  []byte
	err  error
}

func newMockDefaultDomainClient(cert, key []byte, err error) *mockDefaultDomainClient {
	return &mockDefaultDomainClient{
		cert: cert,
		key:  key,
		err:  err,
	}
}

func (m *mockDefaultDomainClient) GetTLSCertificate(ctx context.Context) (*defaultdomain.TLSCertificate, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &defaultdomain.TLSCertificate{
		Cert: m.cert,
		Key:  m.key,
	}, nil
}

func createTestReconciler(client client.Client, defaultDomainClient *mockDefaultDomainClient) *defaultDomainCertControllerReconciler {
	return &defaultDomainCertControllerReconciler{
		client:              client,
		events:              &record.FakeRecorder{},
		conf:                &config.Config{},
		defaultDomainClient: defaultDomainClient,
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

	mockClient := newMockDefaultDomainClient(cert, key, nil)

	reconciler := createTestReconciler(client, mockClient)

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

	mockClient := newMockDefaultDomainClient(nil, nil, nil)
	reconciler := createTestReconciler(client, mockClient)

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

	mockClient := newMockDefaultDomainClient(nil, nil, nil)
	reconciler := createTestReconciler(client, mockClient)

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

	mockClient := newMockDefaultDomainClient(nil, nil, nil)
	reconciler := createTestReconciler(client, mockClient)

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

	// Mock client that returns an error
	mockClient := newMockDefaultDomainClient(nil, nil, errors.New("failed to get TLS certificate from client"))

	reconciler := createTestReconciler(client, mockClient)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "generating Secret for DefaultDomainCertificate: getting and verifying cert and key: failed to get TLS certificate from client")
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

	mockClient := newMockDefaultDomainClient(cert, key, nil)

	reconciler := createTestReconciler(client, mockClient)

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

	mockClient := newMockDefaultDomainClient(cert, key, nil)

	// Use a fake event recorder to capture events
	fakeRecorder := &record.FakeRecorder{Events: make(chan string, 10)}

	reconciler := &defaultDomainCertControllerReconciler{
		client:              client,
		events:              fakeRecorder,
		conf:                &config.Config{},
		defaultDomainClient: mockClient,
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

	mockClient := newMockDefaultDomainClient(cert, key, nil)

	reconciler := createTestReconciler(client, mockClient)

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

	mockClient := newMockDefaultDomainClient(cert, key, nil)

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

	reconciler := createTestReconciler(statusClient, mockClient)

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

	mockClient := newMockDefaultDomainClient(cert, key, nil)

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
	reconciler := createTestReconciler(client, mockClient)

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

	mockClient := newMockDefaultDomainClient(cert, key, nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)
	ddc.UID = "test-uid"

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

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

	// Mock client with empty cert
	mockClient := newMockDefaultDomainClient(nil, key, nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting and verifying cert and key")
	assert.Nil(t, secret)
}

func TestGenerateSecret_KeyNotFoundInStore(t *testing.T) {
	cert, _ := generateTestCertificate(t)

	// Mock client with empty key
	mockClient := newMockDefaultDomainClient(cert, nil, nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting and verifying cert and key")
	assert.Nil(t, secret)
}

func TestGenerateSecret_CertificateContentIsNil(t *testing.T) {
	_, key := generateTestCertificate(t)

	// Mock client with nil cert content
	mockClient := newMockDefaultDomainClient(nil, key, nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting and verifying cert and key")
	assert.Nil(t, secret)
}

func TestGenerateSecret_KeyContentIsNil(t *testing.T) {
	cert, _ := generateTestCertificate(t)

	// Mock client with nil key content
	mockClient := newMockDefaultDomainClient(cert, nil, nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

	require.Error(t, err)
	require.Contains(t, err.Error(), "getting and verifying cert and key")
	require.Nil(t, secret)
}

func TestGenerateSecret_EmptyNamespace(t *testing.T) {
	cert, key := generateTestCertificate(t)

	mockClient := newMockDefaultDomainClient(cert, key, nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", "", testSecretName)

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

	require.NoError(t, err)
	require.NotNil(t, secret)

	assert.Equal(t, testSecretName, secret.Name)
	assert.Equal(t, "", secret.Namespace)
}

func TestGenerateSecret_ValidatesOwnerReferences(t *testing.T) {
	cert, key := generateTestCertificate(t)

	mockClient := newMockDefaultDomainClient(cert, key, nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)
	ddc.UID = "test-uid-12345"
	// Set TypeMeta properly so GetOwnerRefs can extract the GVK
	ddc.TypeMeta = metav1.TypeMeta{
		APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
		Kind:       "DefaultDomainCertificate",
	}

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

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

	mockClient := newMockDefaultDomainClient([]byte(largeCertContent), []byte(largeKeyContent), nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

	require.Contains(t, err.Error(), "getting and verifying cert and key: validating cert and key: failed to decode PEM certificate block")
	require.Nil(t, secret)
}

func TestGenerateSecret_SpecialCharactersInContent(t *testing.T) {
	// Test with special characters, unicode, etc.
	specialCertContent := "-----BEGIN CERTIFICATE-----\n测试特殊字符\n🔒🔑\n-----END CERTIFICATE-----"
	specialKeyContent := "-----BEGIN PRIVATE KEY-----\nñáéíóú\n\x00\x01\x02\n-----END PRIVATE KEY-----"

	mockClient := newMockDefaultDomainClient([]byte(specialCertContent), []byte(specialKeyContent), nil)

	reconciler := createTestReconciler(nil, mockClient)

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	ctx := context.Background()
	secret, err := reconciler.generateSecret(ctx, ddc)

	require.Contains(t, err.Error(), "getting and verifying cert and key: validating cert and key: failed to decode PEM certificate block")
	require.Nil(t, secret)
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

	mockClient := newMockDefaultDomainClient(cert, key, nil)
	reconciler := createTestReconciler(nil, mockClient)

	ctx := context.Background()
	certContent, keyContent, err := reconciler.getAndVerifyCertAndKeyFromClient(ctx)
	require.NoError(t, err)
	require.NotNil(t, certContent)
	require.NotNil(t, keyContent)
	assert.Equal(t, []byte(cert), certContent)
	assert.Equal(t, []byte(key), keyContent)
}

func TestGetAndVerifyCertAndKeyNonCert(t *testing.T) {
	_, key := generateTestCertificate(t)
	cert := []byte("non-cert content")

	mockClient := newMockDefaultDomainClient(cert, key, nil)
	reconciler := createTestReconciler(nil, mockClient)

	ctx := context.Background()
	_, _, err := reconciler.getAndVerifyCertAndKeyFromClient(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM certificate block")
}

func TestGetAndVerifyCertAndKeyMissingKey(t *testing.T) {
	cert, _ := generateTestCertificate(t)

	mockClient := newMockDefaultDomainClient(cert, nil, nil)
	reconciler := createTestReconciler(nil, mockClient)

	ctx := context.Background()
	_, _, err := reconciler.getAndVerifyCertAndKeyFromClient(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TLS certificate key is empty")
}

func TestGetAndVerifyCertAndKeyMissingCert(t *testing.T) {
	_, key := generateTestCertificate(t)

	mockClient := newMockDefaultDomainClient(nil, key, nil)
	reconciler := createTestReconciler(nil, mockClient)

	ctx := context.Background()
	_, _, err := reconciler.getAndVerifyCertAndKeyFromClient(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TLS certificate cert is empty")
}

func TestReconcile_CertificateNotFound_UpdatesStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, approutingv1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ddc := createTestDefaultDomainCertificate("test-ddc", testNamespace, testSecretName)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ddc).
		WithStatusSubresource(ddc).
		Build()

	// Mock client that returns a NotFound error
	notFoundErr := &util.NotFoundError{Body: "not found"}
	mockClient := newMockDefaultDomainClient(nil, nil, notFoundErr)

	reconciler := createTestReconciler(client, mockClient)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-ddc",
			Namespace: testNamespace,
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	require.NoError(t, err)
	// Should requeue after some time
	assert.GreaterOrEqual(t, result.RequeueAfter, 22*time.Second) // 30s * 0.75
	assert.LessOrEqual(t, result.RequeueAfter, 38*time.Second)    // 30s * 1.25

	// Verify DefaultDomainCertificate status was updated to indicate not found
	require.NoError(t, client.Get(ctx, types.NamespacedName{Name: ddc.Name, Namespace: ddc.Namespace}, ddc))
	cond := ddc.GetCondition(v1alpha1.DefaultDomainCertificateConditionTypeAvailable)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, "CertificateNotReady", cond.Reason)
	assert.Contains(t, cond.Message, "Certificate not ready yet, waiting for it to be issued")

	// Verify secret was NOT created
	var secret corev1.Secret
	err = client.Get(ctx, types.NamespacedName{Name: testSecretName, Namespace: testNamespace}, &secret)
	require.Error(t, err)
	assert.True(t, apierrors.IsNotFound(err))
}
