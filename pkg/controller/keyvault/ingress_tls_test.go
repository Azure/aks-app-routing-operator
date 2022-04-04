// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func TestIngressTLSReconcilerIntegration(t *testing.T) {
	ing := &netv1.Ingress{}
	ing.Name = "test-ingress"
	ing.Namespace = "default"
	ingressClass := "webapprouting.aks.io"
	ing.Spec.IngressClassName = &ingressClass
	ing.Annotations = map[string]string{
		"aks.io/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
	}
	ing.Spec.Rules = []netv1.IngressRule{{
		Host: "test-host",
	}}

	c := fake.NewClientBuilder().WithObjects(ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	i := &IngressTLSReconciler{client: c}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create initial rule
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name}}
	_, err := i.Reconcile(ctx, req)
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ing), ing))
	assert.Equal(t, []netv1.IngressTLS{{
		SecretName: "keyvault-" + ing.Name,
		Hosts:      []string{"test-host"},
	}}, ing.Spec.TLS)

	// Check for idempotence
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ing), ing))
	assert.Equal(t, []netv1.IngressTLS{{
		SecretName: "keyvault-" + ing.Name,
		Hosts:      []string{"test-host"},
	}}, ing.Spec.TLS)

	// Add another rule
	ing.Spec.Rules = append(ing.Spec.Rules, netv1.IngressRule{Host: "test-host-2"})
	require.NoError(t, c.Update(ctx, ing))

	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ing), ing))
	assert.Equal(t, []netv1.IngressTLS{{
		SecretName: "keyvault-" + ing.Name,
		Hosts:      []string{"test-host", "test-host-2"},
	}}, ing.Spec.TLS)

	// Remove rule
	ing.Spec.Rules = ing.Spec.Rules[0:1]
	require.NoError(t, c.Update(ctx, ing))

	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ing), ing))
	assert.Equal(t, []netv1.IngressTLS{{
		SecretName: "keyvault-" + ing.Name,
		Hosts:      []string{"test-host"},
	}}, ing.Spec.TLS)

	// Add another TLS rule
	ing.Spec.TLS = append(ing.Spec.TLS, netv1.IngressTLS{})
	require.NoError(t, c.Update(ctx, ing))

	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)

	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ing), ing))
	assert.Equal(t, []netv1.IngressTLS{{
		SecretName: "keyvault-" + ing.Name,
		Hosts:      []string{"test-host"},
	}}, ing.Spec.TLS)
}
