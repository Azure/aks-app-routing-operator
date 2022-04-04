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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

func TestIngressSecretProviderClassReconcilerIntegration(t *testing.T) {
	ing := &netv1.Ingress{}
	ing.Name = "test-ingress"
	ing.Namespace = "default"
	ingressClass := "webapprouting.aks.io"
	ing.Spec.IngressClassName = &ingressClass
	ing.Annotations = map[string]string{
		"aks.io/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
	}

	c := fake.NewClientBuilder().WithObjects(ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	i := &IngressSecretProviderClassReconciler{
		client: c,
		config: &config.Config{
			TenantID:    "test-tenant-id",
			MSIClientID: "test-msi-client-id",
		},
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create the secret provider class
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name}}
	_, err := i.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove it exists
	spc := &secv1.SecretProviderClass{}
	spc.Name = "keyvault-" + ing.Name
	spc.Namespace = ing.Namespace
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(spc), spc))

	expected := &secv1.SecretProviderClass{
		Spec: secv1.SecretProviderClassSpec{
			Provider: "azure",
			Parameters: map[string]string{
				"keyvaultName":           "testvault",
				"objects":                "{\"array\":[\"{\\\"objectName\\\":\\\"testcert\\\",\\\"objectType\\\":\\\"secret\\\",\\\"objectVersion\\\":\\\"f8982febc6894c0697b884f946fb1a34\\\"}\"]}",
				"tenantId":               i.config.TenantID,
				"useVMManagedIdentity":   "true",
				"userAssignedIdentityID": i.config.MSIClientID,
			},
			SecretObjects: []*secv1.SecretObject{{
				SecretName: spc.Name,
				Type:       "kubernetes.io/tls",
				Data: []*secv1.SecretObjectData{
					{ObjectName: "testcert", Key: "tls.key"},
					{ObjectName: "testcert", Key: "tls.crt"},
				},
			}},
		},
	}
	assert.Equal(t, expected.Spec, spc.Spec)

	// Check for idempotence
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)

	// Remove the cert's version from the ingress
	ing.Annotations = map[string]string{
		"aks.io/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert",
	}
	require.NoError(t, i.client.Update(ctx, ing))
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove the objectVersion property was removed
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(spc), spc))
	expected.Spec.Parameters["objects"] = "{\"array\":[\"{\\\"objectName\\\":\\\"testcert\\\",\\\"objectType\\\":\\\"secret\\\"}\"]}"
	assert.Equal(t, expected.Spec, spc.Spec)

	// Remove the cert annotation from the ingress
	ing.Annotations = map[string]string{}
	require.NoError(t, i.client.Update(ctx, ing))
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove secret class was removed
	require.True(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(spc), spc)))

	// Check for idempotence
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)
}
