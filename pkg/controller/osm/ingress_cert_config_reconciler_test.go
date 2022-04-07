// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package osm

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	cfgv1alpha1 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIngressCertConfigReconcilerIntegration(t *testing.T) {
	conf := &cfgv1alpha1.MeshConfig{}
	conf.Name = osmMeshConfigName
	conf.Namespace = osmNamespace

	scheme := runtime.NewScheme()
	require.NoError(t, cfgv1alpha1.AddToScheme(scheme))

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(conf).
		Build()

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: osmNamespace, Name: osmMeshConfigName}}
	e := &IngressCertConfigReconciler{client: c}

	// Initial reconcile
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove config is correct
	actual := &cfgv1alpha1.MeshConfig{}
	require.NoError(t, e.client.Get(ctx, client.ObjectKeyFromObject(conf), actual))
	assert.Equal(t, &cfgv1alpha1.IngressGatewayCertSpec{
		ValidityDuration: "24h",
		SubjectAltNames:  []string{"ingress-nginx.ingress.cluster.local"},
		Secret: corev1.SecretReference{
			Name:      "osm-ingress-client-cert",
			Namespace: "kube-system",
		},
	}, actual.Spec.Certificate.IngressGateway)

	// Cover no-op updates
	_, err = e.Reconcile(ctx, req)
	require.NoError(t, err)
}
