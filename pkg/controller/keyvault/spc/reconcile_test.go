// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package spc

import (
	"context"
	"iter"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
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

// Test buildSpc method
func TestBuildSpc(t *testing.T) {
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

	reconciler := &secretProviderClassReconciler[*appsv1.Deployment]{}

	opts := spcOpts{
		name:          "test-spc",
		namespace:     "test-ns",
		clientId:      "test-client-id",
		tenantId:      "test-tenant-id",
		vaultName:     "test-vault",
		certName:      "test-cert",
		objectVersion: "v1",
		secretName:    "test-secret",
		cloud:         "AzurePublicCloud",
	}

	spc, err := reconciler.buildSpc(deployment, opts)
	require.NoError(t, err)

	// Verify SPC fields
	assert.Equal(t, "test-spc", spc.Name)
	assert.Equal(t, "test-ns", spc.Namespace)
	assert.Equal(t, "azure", string(spc.Spec.Provider))
	assert.Equal(t, "test-vault", spc.Spec.Parameters["keyvaultName"])
	assert.Equal(t, "true", spc.Spec.Parameters["useVMManagedIdentity"])
	assert.Equal(t, "test-client-id", spc.Spec.Parameters["userAssignedIdentityID"])
	assert.Equal(t, "test-tenant-id", spc.Spec.Parameters["tenantId"])
	assert.Equal(t, "AzurePublicCloud", spc.Spec.Parameters["cloud"])
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
