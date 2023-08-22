// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewIngressCertConfigReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, cfgv1alpha2.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := &testutils.FakeManager{Client: client, Scheme: scheme}

	err := NewIngressCertConfigReconciler(manager, &config.Config{})
	require.NoError(t, err)
}

func TestIngressCertConfigReconcilerIntegration(t *testing.T) {
	conf := &cfgv1alpha2.MeshConfig{}
	conf.Name = osmMeshConfigName
	conf.Namespace = osmNamespace

	scheme := runtime.NewScheme()
	require.NoError(t, cfgv1alpha2.AddToScheme(scheme))

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(conf).
		Build()

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: osmNamespace, Name: osmMeshConfigName}}
	e := &IngressCertConfigReconciler{client: c}

	// Initial reconcile
	beforeErrCount := testutils.GetErrMetricCount(t, ingressCertConfigControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, ingressCertConfigControllerName, metrics.LabelSuccess)
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressCertConfigControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressCertConfigControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Prove config is correct
	actual := &cfgv1alpha2.MeshConfig{}
	require.NoError(t, e.client.Get(ctx, client.ObjectKeyFromObject(conf), actual))

	expected := &cfgv1alpha2.IngressGatewayCertSpec{
		ValidityDuration: "24h",
		SubjectAltNames:  []string{"ingress-nginx.ingress.cluster.local"},
		Secret: corev1.SecretReference{
			Name:      "osm-ingress-client-cert",
			Namespace: "kube-system",
		},
	}
	assert.Equal(t, expected, actual.Spec.Certificate.IngressGateway)

	// Cover no-op updates
	beforeErrCount = testutils.GetErrMetricCount(t, ingressCertConfigControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, ingressCertConfigControllerName, metrics.LabelSuccess)
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressCertConfigControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressCertConfigControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Update the resource to incorrect values and reconcile back
	actual.Spec.Certificate.IngressGateway.SubjectAltNames = []string{"incorrect"}
	actual.Spec.Certificate.IngressGateway.ValidityDuration = "12h"
	actual.Spec.Certificate.IngressGateway.Secret.Name = "foo"
	actual.Spec.Certificate.IngressGateway.Secret.Namespace = "bar"

	beforeErrCount = testutils.GetErrMetricCount(t, ingressCertConfigControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, ingressCertConfigControllerName, metrics.LabelSuccess)
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressCertConfigControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressCertConfigControllerName, metrics.LabelSuccess), beforeReconcileCount)

	require.NoError(t, e.client.Get(ctx, client.ObjectKeyFromObject(conf), actual))
	assert.Equal(t, expected, actual.Spec.Certificate.IngressGateway)
}
