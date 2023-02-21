// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

func TestIngressBackendReconcilerIntegration(t *testing.T) {
	ing := &netv1.Ingress{
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
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

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
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)

	// Remove the annotation
	ing.Annotations = map[string]string{}
	require.NoError(t, c.Update(ctx, ing))
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove the ingress backend was cleaned up
	require.True(t, errors.IsNotFound(e.client.Get(ctx, client.ObjectKeyFromObject(actual), actual)))

	// Cover no-op deletions
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)
}
