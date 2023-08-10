// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"
	"os"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

var (
	env        *envtest.Environment
	restConfig *rest.Config
	err        error
	ing        = &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Namespace:   "test-ns",
			Annotations: map[string]string{"kubernetes.azure.com/use-osm-mtls": "true"},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: util.StringPtr("test-ingress-class"),
			Rules: []netv1.IngressRule{{}, {
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{{}, {
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: "test-service",
									Port: netv1.ServiceBackendPort{Number: 123},
								},
							},
						}},
					},
				},
			}},
		},
	}
)

func TestMain(m *testing.M) {
	restConfig, env, err = testutils.StartTestingEnv()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testutils.CleanupTestingEnv(env)

	os.Exit(code)
}

func TestIngressBackendReconcilerIntegration(t *testing.T) {
	c := fake.NewClientBuilder().WithObjects(ing).Build()
	require.NoError(t, policyv1alpha1.AddToScheme(c.Scheme()))

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name}}
	e := &IngressBackendReconciler{
		client:                 c,
		config:                 &config.Config{NS: "test-config-ns"},
		ingressControllerNamer: NewIngressControllerNamer(map[string]string{*ing.Spec.IngressClassName: "test-name"}),
	}

	// Initial reconcile
	beforeErrCount := testutils.GetErrMetricCount(t, ingressBackendControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess)
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressBackendControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Prove config is correct
	actual := &policyv1alpha1.IngressBackend{}
	actual.Name = ing.Name
	actual.Namespace = ing.Namespace
	require.NoError(t, e.client.Get(ctx, client.ObjectKeyFromObject(actual), actual))
	require.Len(t, actual.Spec.Backends, 1)
	assert.Equal(t, policyv1alpha1.BackendSpec{
		Name: "test-service",
		Port: policyv1alpha1.PortSpec{Number: 123, Protocol: "https"},
	}, actual.Spec.Backends[0])

	// Cover no-op updates
	beforeErrCount = testutils.GetErrMetricCount(t, ingressBackendControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess)
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressBackendControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Remove the annotation
	ing.Annotations = map[string]string{}
	require.NoError(t, c.Update(ctx, ing))
	beforeErrCount = testutils.GetErrMetricCount(t, ingressBackendControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess)
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressBackendControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess), beforeReconcileCount)
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove the ingress backend was cleaned up
	require.True(t, errors.IsNotFound(e.client.Get(ctx, client.ObjectKeyFromObject(actual), actual)))

	// Cover no-op deletions
	beforeErrCount = testutils.GetErrMetricCount(t, ingressBackendControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess)
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressBackendControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressBackendControllerName, metrics.LabelSuccess), beforeReconcileCount)
}

func TestNewIngressBackendReconciler(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{MetricsBindAddress: "0"})
	require.NoError(t, err)

	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}
	ingressControllerName := NewIngressControllerNamer(map[string]string{*ing.Spec.IngressClassName: "test-name"})
	err = NewIngressBackendReconciler(m, conf, ingressControllerName)
	require.NoError(t, err, "should not error")

}
