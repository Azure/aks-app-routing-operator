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

const (
	reconcileTestNamespace  = "test-ns"
	reconcileTestDeployment = "test-deployment"
	reconcileTestUID        = "test-uid"
	reconcileTestSPC        = "test-spc"
	reconcileTestController = "test-controller"
	reconcileTestClientId   = "test-client-id"
	reconcileTestTenantId   = "test-tenant-id"
	reconcileTestVaultName  = "test-vault"
	reconcileTestCertName   = "test-cert"
	reconcileTestSecret     = "test-secret"
	reconcileTestProvider   = "azure"
	reconcileTestCertUri    = "https://keyvault.vault.azure.net/secrets/certificate"
)

// Test successful case when reconciling updates an existing SecretProviderClass with new values
func TestReconcileUpdateExistingSecretProviderClass(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
			Annotations: map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": reconcileTestCertUri,
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
	}

	// Create the initial SPC with original values
	existingSpc := &secv1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestSPC,
			Namespace: reconcileTestNamespace,
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       reconcileTestDeployment,
					UID:        reconcileTestUID,
					Controller: util.ToPtr(true),
				},
			},
		},
		Spec: secv1.SecretProviderClassSpec{
			Provider: "azure",
			Parameters: map[string]string{
				"keyvaultName":           "original-vault",
				"useVMManagedIdentity":   "true",
				"userAssignedIdentityID": "original-client-id",
				"tenantId":               "original-tenant-id",
				"objects":                "{\"array\":[\"{\\\"objectName\\\":\\\"original-cert\\\",\\\"objectType\\\":\\\"secret\\\"}\"]}",
			},
			SecretObjects: []*secv1.SecretObject{
				{
					SecretName: "original-secret",
					Type:       "kubernetes.io/tls",
					Data: []*secv1.SecretObjectData{
						{ObjectName: "original-cert", Key: "tls.key"},
						{ObjectName: "original-cert", Key: "tls.crt"},
					},
				},
			},
		},
	}

	firstReconcile := true
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment, existingSpc).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New("test-controller"),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				if firstReconcile {
					// First reconcile - return original values
					opts := spcOpts{
						action:        actionReconcile,
						name:          reconcileTestSPC,
						namespace:     reconcileTestNamespace,
						clientId:      "original-client-id",
						tenantId:      "original-tenant-id",
						vaultName:     "original-vault",
						certName:      "original-cert",
						secretName:    "original-secret",
						objectVersion: "",
					}
					firstReconcile = false
					yield(opts, nil)
				} else {
					// Second reconcile - return updated values
					opts := spcOpts{
						action:        actionReconcile,
						name:          reconcileTestSPC,
						namespace:     reconcileTestNamespace,
						clientId:      "updated-client-id",
						tenantId:      "updated-tenant-id",
						vaultName:     "updated-vault",
						certName:      "updated-cert",
						secretName:    "updated-secret",
						objectVersion: "v2",
					}
					yield(opts, nil)
				}
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// First reconciliation
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics after first reconciliation
	require.Equal(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount)

	// Second reconciliation
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)
	result, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics after second reconciliation
	require.Equal(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount)

	// Verify SPC was updated with new values
	updatedSpc := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, updatedSpc)
	require.NoError(t, err)

	// Verify the updated values
	assert.Equal(t, "updated-vault", updatedSpc.Spec.Parameters["keyvaultName"])
	assert.Equal(t, "updated-client-id", updatedSpc.Spec.Parameters["userAssignedIdentityID"])
	assert.Equal(t, "updated-tenant-id", updatedSpc.Spec.Parameters["tenantId"])
	assert.Contains(t, updatedSpc.Spec.Parameters["objects"], "updated-cert")
	assert.Contains(t, updatedSpc.Spec.Parameters["objects"], "v2") // Check that object version was updated
	require.Len(t, updatedSpc.Spec.SecretObjects, 1)
	assert.Equal(t, "updated-secret", updatedSpc.Spec.SecretObjects[0].SecretName)
	require.Len(t, updatedSpc.Spec.SecretObjects[0].Data, 2)
	assert.Equal(t, "updated-cert", updatedSpc.Spec.SecretObjects[0].Data[0].ObjectName)
	assert.Equal(t, "updated-cert", updatedSpc.Spec.SecretObjects[0].Data[1].ObjectName)
}

// Test the Reconcile method with successful case
func TestReconcileSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
			Annotations: map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": reconcileTestCertUri,
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
		name:      controllername.New(reconcileTestController),
		client:    c,
		events:    events,
		config:    &config.Config{},
		toSpcOpts: getSpcOpts,
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
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
	require.NoError(t, err)

	assert.Equal(t, reconcileTestProvider, string(spc.Spec.Provider))
	assert.Equal(t, "true", spc.Spec.Parameters["useVMManagedIdentity"])
}

// Test cleanupSpcOpt with successful cleanup
func TestCleanupSpcOpt(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, secv1.AddToScheme(scheme))

	tests := []struct {
		name       string
		spc        *secv1.SecretProviderClass
		wantErrStr string
		verify     func(*testing.T, client.Client)
	}{
		{
			name: "spc with top-level labels is deleted",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      reconcileTestSPC,
					Namespace: reconcileTestNamespace,
					Labels:    manifests.GetTopLevelLabels(),
				},
				Spec: secv1.SecretProviderClassSpec{
					Provider: reconcileTestProvider,
				},
			},
			verify: func(t *testing.T, c client.Client) {
				// Verify SPC was deleted
				spc := &secv1.SecretProviderClass{}
				err := c.Get(context.Background(), types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
				require.True(t, errors.IsNotFound(err), "expected SPC to be deleted")
			},
		},
		{
			name: "spc without top-level labels is not deleted",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      reconcileTestSPC,
					Namespace: reconcileTestNamespace,
					Labels: map[string]string{
						"custom-label": "value",
					},
				},
				Spec: secv1.SecretProviderClassSpec{
					Provider: reconcileTestProvider,
				},
			},
			verify: func(t *testing.T, c client.Client) {
				// Verify SPC still exists
				spc := &secv1.SecretProviderClass{}
				err := c.Get(context.Background(), types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
				require.NoError(t, err)
				assert.Equal(t, "value", spc.Labels["custom-label"])
			},
		},
		{
			name: "spc not found returns success",
			spc:  nil,
			verify: func(t *testing.T, c client.Client) {
				// Verify attempting to get SPC returns not found
				spc := &secv1.SecretProviderClass{}
				err := c.Get(context.Background(), types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
				require.True(t, errors.IsNotFound(err))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.spc != nil {
				builder = builder.WithObjects(tt.spc)
			}
			c := builder.Build()

			reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
				client: c,
				name:   controllername.New("test-controller"),
			}

			opts := spcOpts{
				action:    actionCleanup,
				name:      reconcileTestSPC,
				namespace: reconcileTestNamespace,
			}

			err := reconciler.cleanupSpc(context.Background(), logr.Discard(), opts)
			if tt.wantErrStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				return
			}
			require.NoError(t, err)

			if tt.verify != nil {
				tt.verify(t, c)
			}
		})
	}
}

// Test toSpcOpts with user error
func TestToSpcOptsUserError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New(reconcileTestController),
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
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
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
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
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
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
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
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
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
					name:      reconcileTestSPC,
					namespace: reconcileTestNamespace,
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
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
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
					name:      reconcileTestSPC,
					namespace: reconcileTestNamespace,
					modifyOwner: func(obj client.Object) error {
						obj.SetAnnotations(map[string]string{"new": "annotation"})
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
func TestReconcileObjectNotExists(t *testing.T) {
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
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestDeployment}}

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
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
	require.True(t, errors.IsNotFound(err), "expected no SPC to be created")
}

// TestBuildSpcMatrix tests various combinations of inputs for buildSpc
func TestBuildSpc(t *testing.T) {
	testDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
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
				name:       reconcileTestSPC,
				namespace:  reconcileTestNamespace,
				clientId:   reconcileTestClientId,
				tenantId:   reconcileTestTenantId,
				vaultName:  reconcileTestVaultName,
				certName:   reconcileTestCertName,
				secretName: reconcileTestSecret,
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				assert.Equal(t, reconcileTestSPC, spc.Name)
				assert.Equal(t, reconcileTestNamespace, spc.Namespace)
				assert.Equal(t, reconcileTestProvider, string(spc.Spec.Provider))
				assert.Equal(t, reconcileTestVaultName, spc.Spec.Parameters["keyvaultName"])
				assert.Equal(t, "true", spc.Spec.Parameters["useVMManagedIdentity"])
				assert.Equal(t, reconcileTestClientId, spc.Spec.Parameters["userAssignedIdentityID"])
				assert.Equal(t, reconcileTestTenantId, spc.Spec.Parameters["tenantId"])
				assert.Empty(t, spc.Spec.Parameters["cloud"])
				assert.Empty(t, spc.Spec.Parameters["objectVersion"])
			},
		},
		{
			name: "with object version",
			opts: spcOpts{
				name:          reconcileTestSPC,
				namespace:     reconcileTestNamespace,
				clientId:      reconcileTestClientId,
				tenantId:      reconcileTestTenantId,
				vaultName:     reconcileTestVaultName,
				certName:      reconcileTestCertName,
				objectVersion: "1234",
				secretName:    reconcileTestSecret,
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				assert.Contains(t, spc.Spec.Parameters["objects"], `1234`)
			},
		},
		{
			name: "with custom cloud",
			opts: spcOpts{
				name:       reconcileTestSPC,
				namespace:  reconcileTestNamespace,
				clientId:   reconcileTestClientId,
				tenantId:   reconcileTestTenantId,
				vaultName:  reconcileTestVaultName,
				certName:   reconcileTestCertName,
				secretName: reconcileTestSecret,
				cloud:      "AzureChinaCloud",
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				assert.Equal(t, "AzureChinaCloud", spc.Spec.Parameters["cloud"])
			},
		},
		{
			name: "verify TLS secret type and data keys",
			opts: spcOpts{
				name:       reconcileTestSPC,
				namespace:  reconcileTestNamespace,
				clientId:   reconcileTestClientId,
				tenantId:   reconcileTestTenantId,
				vaultName:  reconcileTestVaultName,
				certName:   reconcileTestCertName,
				secretName: reconcileTestSecret,
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
				name:       reconcileTestSPC,
				namespace:  reconcileTestNamespace,
				clientId:   reconcileTestClientId,
				tenantId:   reconcileTestTenantId,
				vaultName:  reconcileTestVaultName,
				certName:   reconcileTestCertName,
				secretName: reconcileTestSecret,
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				require.Len(t, spc.OwnerReferences, 1)
				owner := spc.OwnerReferences[0]
				assert.Equal(t, reconcileTestDeployment, owner.Name)
				assert.Equal(t, reconcileTestUID, string(owner.UID))
				assert.Equal(t, "Deployment", owner.Kind)
				assert.Equal(t, "apps/v1", owner.APIVersion)
				assert.True(t, *owner.Controller)
			},
		},
		{
			name: "workload identity enabled",
			opts: spcOpts{
				name:             reconcileTestSPC,
				namespace:        reconcileTestNamespace,
				clientId:         reconcileTestClientId,
				tenantId:         reconcileTestTenantId,
				vaultName:        reconcileTestVaultName,
				certName:         reconcileTestCertName,
				secretName:       reconcileTestSecret,
				workloadIdentity: true,
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				// When workload identity is enabled, clientID should be used instead of userAssignedIdentityID
				assert.Equal(t, reconcileTestClientId, spc.Spec.Parameters["clientID"])
				assert.Empty(t, spc.Spec.Parameters["userAssignedIdentityID"])
				assert.Empty(t, spc.Spec.Parameters["useVMManagedIdentity"])
			},
		},
		{
			name: "workload identity disabled (default behavior)",
			opts: spcOpts{
				name:             reconcileTestSPC,
				namespace:        reconcileTestNamespace,
				clientId:         reconcileTestClientId,
				tenantId:         reconcileTestTenantId,
				vaultName:        reconcileTestVaultName,
				certName:         reconcileTestCertName,
				secretName:       reconcileTestSecret,
				workloadIdentity: false,
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				// When workload identity is disabled, userAssignedIdentityID and useVMManagedIdentity should be used
				assert.Equal(t, reconcileTestClientId, spc.Spec.Parameters["userAssignedIdentityID"])
				assert.Equal(t, "true", spc.Spec.Parameters["useVMManagedIdentity"])
				assert.Empty(t, spc.Spec.Parameters["clientID"])
			},
		},
		{
			name: "workload identity with object version and custom cloud",
			opts: spcOpts{
				name:             reconcileTestSPC,
				namespace:        reconcileTestNamespace,
				clientId:         reconcileTestClientId,
				tenantId:         reconcileTestTenantId,
				vaultName:        reconcileTestVaultName,
				certName:         reconcileTestCertName,
				objectVersion:    "v2",
				secretName:       reconcileTestSecret,
				cloud:            "AzureChinaCloud",
				workloadIdentity: true,
			},
			verify: func(t *testing.T, spc *secv1.SecretProviderClass) {
				// Verify workload identity parameters
				assert.Equal(t, reconcileTestClientId, spc.Spec.Parameters["clientID"])
				assert.Empty(t, spc.Spec.Parameters["userAssignedIdentityID"])
				assert.Empty(t, spc.Spec.Parameters["useVMManagedIdentity"])

				// Verify other parameters still work with workload identity
				assert.Equal(t, "AzureChinaCloud", spc.Spec.Parameters["cloud"])
				assert.Contains(t, spc.Spec.Parameters["objects"], "v2")
				assert.Equal(t, reconcileTestVaultName, spc.Spec.Parameters["keyvaultName"])
				assert.Equal(t, reconcileTestTenantId, spc.Spec.Parameters["tenantId"])
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
func getSpcOpts(ctx context.Context, c client.Client, obj *appsv1.Deployment) iter.Seq2[spcOpts, error] {
	return func(yield func(spcOpts, error) bool) {
		if certURI, ok := obj.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]; ok {
			certRef, err := parseKeyVaultCertURI(certURI)
			if err != nil {
				yield(spcOpts{}, err)
				return
			}

			opts := spcOpts{
				action:        actionReconcile,
				name:          reconcileTestSPC,
				namespace:     obj.Namespace,
				clientId:      reconcileTestClientId,
				tenantId:      reconcileTestTenantId,
				vaultName:     certRef.vaultName,
				certName:      certRef.certName,
				objectVersion: certRef.objectVersion,
				secretName:    reconcileTestSecret,
			}
			yield(opts, nil)
		}
	}
}

// Test reconcile with multiple spcOpts
func TestReconcileMultipleSpcOpts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
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
				// First SPC opt for reconciliation
				opts1 := spcOpts{
					action:    actionReconcile,
					name:      "test-spc-1",
					namespace: reconcileTestNamespace,
					modifyOwner: func(obj client.Object) error {
						annotations := map[string]string{"test1": "value1"}
						if existing := obj.GetAnnotations(); existing != nil {
							for k, v := range existing {
								annotations[k] = v
							}
						}
						obj.SetAnnotations(annotations)
						return nil
					},
				}
				if !yield(opts1, nil) {
					return
				}

				// Second SPC opt for reconciliation
				opts2 := spcOpts{
					action:    actionReconcile,
					name:      "test-spc-2",
					namespace: reconcileTestNamespace,
					modifyOwner: func(obj client.Object) error {
						annotations := map[string]string{"test2": "value2"}
						if existing := obj.GetAnnotations(); existing != nil {
							for k, v := range existing {
								annotations[k] = v
							}
						}
						obj.SetAnnotations(annotations)
						return nil
					},
				}
				yield(opts2, nil)
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with multiple opts
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount)

	// Verify deployment was updated with both annotations
	err = c.Get(ctx, req.NamespacedName, deployment)
	require.NoError(t, err)
	assert.Equal(t, "value1", deployment.Annotations["test1"])
	assert.Equal(t, "value2", deployment.Annotations["test2"])

	// Verify both SPCs were created
	spc1 := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: "test-spc-1"}, spc1)
	require.NoError(t, err)
	spc2 := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: "test-spc-2"}, spc2)
}

// Test reconcile with multiple spcOpts where one returns an error
func TestReconcileMultipleSpcOptsWithError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
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
	expectedError := fmt.Errorf("test error from second spcOpt")
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New("test-controller"),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				// First SPC opt succeeds
				opts1 := spcOpts{
					action:    actionReconcile,
					name:      "test-spc-1",
					namespace: reconcileTestNamespace,
					modifyOwner: func(obj client.Object) error {
						annotations := map[string]string{"test1": "value1"}
						if existing := obj.GetAnnotations(); existing != nil {
							for k, v := range existing {
								annotations[k] = v
							}
						}
						obj.SetAnnotations(annotations)
						return nil
					},
				}
				if !yield(opts1, nil) {
					return
				}

				yield(spcOpts{
					action:    actionReconcile,
					name:      "test-spc-2",
					namespace: reconcileTestNamespace,
				}, expectedError)
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with multiple opts where one fails
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError)

	result, err := reconciler.Reconcile(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), expectedError.Error())
	require.Equal(t, ctrl.Result{}, result)

	// Verify error metrics were incremented
	require.Greater(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError), beforeReconcileCount)

	// Verify first SPC was created successfully despite second one failing
	spc1 := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: "test-spc-1"}, spc1)
	require.NoError(t, err)

	// Verify second SPC was not created
	spc2 := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: "test-spc-2"}, spc2)
	require.True(t, errors.IsNotFound(err), "expected second SPC to not be created")

	// Verify deployment does not have the annotation from the first successful opt
	err = c.Get(ctx, req.NamespacedName, deployment)
	require.NoError(t, err)
	assert.NotEqual(t, "value1", deployment.Annotations["test1"])
}

// Test reconcile when object is being cleaned up
func TestReconcileObjectCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	now := metav1.Now()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              reconcileTestDeployment,
			Namespace:         reconcileTestNamespace,
			UID:               reconcileTestUID,
			DeletionTimestamp: &now,
			Finalizers:        []string{"test-finalizer"},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
	}

	existingSpc := &secv1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestSPC,
			Namespace: reconcileTestNamespace,
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       reconcileTestDeployment,
					UID:        reconcileTestUID,
					Controller: util.ToPtr(true),
				},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment, existingSpc).
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
					action:    actionCleanup,
					name:      reconcileTestSPC,
					namespace: reconcileTestNamespace,
				}
				yield(opts, nil)
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with cleanup
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount)

	// Verify SPC was deleted
	err = c.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "test-spc"}, &secv1.SecretProviderClass{})
	require.True(t, errors.IsNotFound(err), "expected SPC to be deleted")

	// Verify no events were recorded
	require.Len(t, events.Events, 0, "expected no events to be recorded")
}

// Test error handling when checking if an ingress is managed
func TestReconcileIngressManagedError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
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
		name:   controllername.New(reconcileTestController),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				// Simulate an error that could occur when checking if ingress is managed
				err := fmt.Errorf("failed to check if ingress is managed: some error")
				yield(spcOpts{}, err)
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with error checking if ingress is managed
	beforeErrCount := testutils.GetErrMetricCount(t, reconciler.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError)

	result, err := reconciler.Reconcile(ctx, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check if ingress is managed: some error")
	require.Equal(t, ctrl.Result{}, result)

	// Verify error metrics were incremented
	require.Greater(t, testutils.GetErrMetricCount(t, reconciler.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelError), beforeReconcileCount)

	// Verify no SPC was created
	spc := &secv1.SecretProviderClass{}
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, spc)
	require.True(t, errors.IsNotFound(err), "expected SPC to not be created")

	// Verify no events were sent (non-user errors don't generate events)
	require.Len(t, events.Events, 0, "expected no events to be recorded")
}

// Test cleanup of SPC when deployment has top-level labels
func TestReconcileWithTopLevelLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	// Create a deployment with top-level labels
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestDeployment,
			Namespace: reconcileTestNamespace,
			UID:       reconcileTestUID,
			Labels:    manifests.GetTopLevelLabels(),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
	}

	// Create an existing SPC that should be cleaned up
	existingSpc := &secv1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reconcileTestSPC,
			Namespace: reconcileTestNamespace,
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       reconcileTestDeployment,
					UID:        reconcileTestUID,
					Controller: util.ToPtr(true),
				},
			},
		},
		Spec: secv1.SecretProviderClassSpec{
			Provider: reconcileTestProvider,
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment, existingSpc).
		Build()

	events := record.NewFakeRecorder(10)
	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{
		name:   controllername.New(reconcileTestController),
		client: c,
		events: events,
		config: &config.Config{},
		toSpcOpts: func(_ context.Context, _ client.Client, _ *appsv1.Deployment) iter.Seq2[spcOpts, error] {
			return func(yield func(spcOpts, error) bool) {
				// Return cleanup action since this deployment has top-level labels
				opts := spcOpts{
					action:    actionCleanup,
					name:      reconcileTestSPC,
					namespace: reconcileTestNamespace,
				}
				yield(opts, nil)
			}
		},
	}

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: deployment.Namespace, Name: deployment.Name}}

	// Test reconcile with deployment having top-level labels
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess)

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)

	// Verify metrics
	require.Greater(t, testutils.GetReconcileMetricCount(t, reconciler.name, metrics.LabelSuccess), beforeReconcileCount)

	// Verify existing SPC was cleaned up due to deployment having top-level labels
	err = c.Get(ctx, types.NamespacedName{Namespace: reconcileTestNamespace, Name: reconcileTestSPC}, &secv1.SecretProviderClass{})
	require.True(t, errors.IsNotFound(err), "expected existing SPC to be deleted")

	// Verify no events were recorded (cleanup operations don't generate events)
	require.Len(t, events.Events, 0, "expected no events to be recorded")
}
