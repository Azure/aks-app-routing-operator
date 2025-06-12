// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package spc

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

// Test the Reconcile method with successful case
func TestReconcileSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-ns",
			UID:       "test-uid",
			Annotations: map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://keyvault.vault.azure.net/secrets/certificate",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:      controllername.New("test-controller"),
		client:    c,
		events:    events,
		config:    &config.Config{},
		toSpcOpts: testToSpcOpts,
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test initial reconcile
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics
	require.Equal(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount)

	// Verify SPC was created
	spc := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "test-spc"}, spc)
	require.NoError(t, err)

	assert.Equal(t, "azure", string(spc.Spec.Provider))
	assert.Equal(t, "true", spc.Spec.Parameters["useVMManagedIdentity"])
}

// Test cleanupSpcOpt with successful cleanup
func TestCleanupSpcOpt(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, secv1.AddToScheme(scheme))

	spc := &secv1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spc",
			Namespace: "test-ns",
			Labels:    manifests.GetTopLevelLabels(),
		},
		Spec: secv1.SecretProviderClassSpec{
			Provider: "azure",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(spc).
		Build()

	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		client: c,
		name:   controllername.New("test-controller"),
	}

	opts := spcOpts{
		action:    actionCleanup,
		name:      "test-spc",
		namespace: "test-ns",
	}

	err := reconciler.cleanupSpcOpt(context.Background(), logr.Discard(), opts)
	require.NoError(t, err)

	// Verify SPC was deleted
	err = c.Get(context.Background(), types.NamespacedName{Namespace: "test-ns", Name: "test-spc"}, spc)
	require.True(t, errors.IsNotFound(err))
}

// Test toSpcOpts with user error
func TestToSpcOptsUserError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New("test-controller"),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				yield(spcOpts{}, util.NewUserError(fmt.Errorf("user error"), "user error"))
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with invalid cert URI
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err) // User errors should not return an error from Reconcile
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics for user error
	require.Equal(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Equal(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError), beforeReconcileCount)

	// Verify no SPC was created
	spc := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "test-spc"}, spc)
	require.True(t, errors.IsNotFound(err), "expected SPC to not be created")

	// Verify warning event was sent to the deployment
	require.Len(t, events.Events, 1, "expected one event to be recorded")
	event := <-events.Events
	assert.Contains(t, event, "InvalidInput")
	assert.Contains(t, event, "user error")
}

// Test toSpcOpts with non-user error
func TestToSpcOptsError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New("test-controller"),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				yield(spcOpts{}, fmt.Errorf("some internal error"))
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with internal error
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError)

	result, err := reconciler.Reconcile(ctx, req)
	require.Error(t, err) // Non-user errors should be returned from Reconcile
	assert.Contains(t, err.Error(), "building secret provider class: some internal error")
	require.Equal(t, ctrl.Result{}, result)

	// Verify error metrics were incremented
	require.Greater(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError), beforeReconcileCount)

	// Verify no SPC was created
	spc := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "test-spc"}, spc)
	require.True(t, errors.IsNotFound(err), "expected SPC to not be created")

	// Verify no events were sent (non-user errors don't generate events)
	require.Len(t, events.Events, 0, "expected no events to be recorded")
}

// Test successful case when object needs updating
func TestReconcileWithObjectUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New("test-controller"),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				opts := spcOpts{
					action:    actionReconcile,
					name:      "test-spc",
					namespace: "test-ns",
					modifyOwner: func(obj client.Object) error {
						obj.SetAnnotations(map[string]string{"test": "value"})
						return nil
					},
				}
				yield(opts, nil)
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics
	require.Equal(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount)

	// Verify deployment was updated
	err = c.Get(ctx, req.NamespacedName, deployment)
	require.NoError(t, err)
	assert.Equal(t, "value", deployment.Annotations["test"])
}

// Test error case when modifyOwner fails
func TestReconcileModifyOwnerError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	events := record.NewFakeRecorder(10)
	expectedError := fmt.Errorf("modify owner error")
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New("test-controller"),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				opts := spcOpts{
					action:    actionReconcile,
					name:      "test-spc",
					namespace: "test-ns",
					modifyOwner: func(obj client.Object) error {
						return expectedError
					},
				}
				yield(opts, nil)
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with modifyOwner error
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError)

	result, err := reconciler.Reconcile(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modifying owning object: modify owner error")
	require.Equal(t, ctrl.Result{}, result)

	// Verify error metrics were incremented
	require.Greater(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError), beforeReconcileCount)

	// Verify deployment was not modified
	err = c.Get(ctx, req.NamespacedName, deployment)
	require.NoError(t, err)
	assert.Nil(t, deployment.Annotations)

	// Verify no events were sent
	require.Len(t, events.Events, 0, "expected no events to be recorded")
}

// Test case for when getting the initial object returns not found
func TestReconcileObjectNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	// Create client with empty object store
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New("test-controller"),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				// This should not be called since object is not found
				t.Error("toSpcOpts was called unexpectedly")
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-deployment"}}

	// Test reconcile for non-existent object
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err, "Reconcile should not return error when object is not found")
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics were not incremented
	require.Equal(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Equal(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount+1)

	// Verify no events were recorded
	require.Len(t, events.Events, 0, "expected no events to be recorded")

	// Verify no SPC was created
	spc := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "test-spc"}, spc)
	require.True(t, errors.IsNotFound(err), "expected no SPC to be created")
}

// TestBuildSpcMatrix tests various combinations of inputs for buildSpc
func TestBuildSpc(t *testing.T) {
	testDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
	}

	tests := []struct {
		name   string
		opts   spcOpts
		verify func(*testing.T, *secv1.SecretProviderClass)
	}{
		{
			name: "minimal required fields",
			opts: spcOpts{
				name:       "test-spc",
				namespace:  "test-ns",
				clientId:   "test-client-id",
				tenantId:   "test-tenant-id",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "test-secret",
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				assert.Equal(t, "test-spc", spc.Name)
				assert.Equal(t, "test-ns", spc.Namespace)
				assert.Equal(t, "azure", string(spc.Spec.Provider))
				assert.Equal(t, "test-vault", spc.Spec.Parameters["keyvaultName"])
				assert.Equal(t, "true", spc.Spec.Parameters["useVMManagedIdentity"])
				assert.Equal(t, "test-client-id", spc.Spec.Parameters["userAssignedIdentityID"])
				assert.Equal(t, "test-tenant-id", spc.Spec.Parameters["tenantId"])
				assert.Empty(t, spc.Spec.Parameters["cloud"])
				assert.Empty(t, spc.Spec.Parameters["objectVersion"])
			},
		},
		{
			name: "with object version",
			opts: spcOpts{
				name:          "test-spc",
				namespace:     "test-ns",
				clientId:      "test-client-id",
				tenantId:      "test-tenant-id",
				vaultName:     "test-vault",
				certName:      "test-cert",
				objectVersion: "1234",
				secretName:    "test-secret",
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				assert.Contains(t, spc.Spec.Parameters["objects"], `1234`)
			},
		},
		{
			name: "with custom cloud",
			opts: spcOpts{
				name:       "test-spc",
				namespace:  "test-ns",
				clientId:   "test-client-id",
				tenantId:   "test-tenant-id",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "test-secret",
				cloud:      "AzureChinaCloud",
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				assert.Equal(t, "AzureChinaCloud", spc.Spec.Parameters["cloud"])
			},
		},
		{
			name: "verify TLS secret type and data keys",
			opts: spcOpts{
				name:       "test-spc",
				namespace:  "test-ns",
				clientId:   "test-client-id",
				tenantId:   "test-tenant-id",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "test-secret",
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				require.Len(t, spc.Spec.SecretObjects, 1)
				secretObj := spc.Spec.SecretObjects[0]
				assert.Equal(t, "kubernetes.io/tls", secretObj.Type)
				assert.Len(t, secretObj.Data, 2)
				assert.Equal(t, "tls.key", secretObj.Data[0].Key)
				assert.Equal(t, "tls.crt", secretObj.Data[1].Key)
				assert.Equal(t, "test-cert", secretObj.Data[0].ObjectName)
				assert.Equal(t, "test-cert", secretObj.Data[1].ObjectName)
			},
		},
		{
			name: "verify owner references",
			opts: spcOpts{
				name:       "test-spc",
				namespace:  "test-ns",
				clientId:   "test-client-id",
				tenantId:   "test-tenant-id",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "test-secret",
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				require.Len(t, spc.OwnerReferences, 1)
				owner := spc.OwnerReferences[0]
				assert.Equal(t, "test-deployment", owner.Name)
				assert.Equal(t, "test-uid", string(owner.UID))
				assert.Equal(t, "Deployment", owner.Kind)
				assert.Equal(t, "apps/v1", owner.APIVersion)
				assert.True(t, *owner.Controller)
			},
		},
	}

	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spc, err := reconciler.buildSpc(testDeployment, tt.opts)
			require.NoError(t, err)
			tt.verify(t, spc)
		})
	}
}

// Helper function to simulate toSpcOpts
func testToSpcOpts(ctx context.Context, c client.Client, obj *appsv1.Deployment) iter.Seq2[spcOpts, error] {
	return func(yield func(spcOpts, error) bool) {
		if certURI, ok := obj.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]; ok {
			certRef, err := parseKeyVaultCertURI(certURI)
			if err != nil {
				yield(spcOpts{}, err)
				return
			}

			opts := spcOpts{
				action:        actionReconcile,
				name:          "test-spc",
				namespace:     obj.Namespace,
				clientId:      "test-client-id",
				tenantId:      "test-tenant-id",
				vaultName:     certRef.vaultName,
				certName:      certRef.certName,
				objectVersion: certRef.objectVersion,
				secretName:    "test-secret",
			}
			yield(opts, nil)
		}
	}
}
