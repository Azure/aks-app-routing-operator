// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
)

var (
	spcTestIngressClassName = "webapprouting.kubernetes.azure.com"
	spcTestIngress          = &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "default",
			Annotations: map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &spcTestIngressClassName,
		},
	}
)

func TestIngressSecretProviderClassReconcilerIntegration(t *testing.T) {
	// Create the ingress
	ing := spcTestIngress.DeepCopy()

	c := fake.NewClientBuilder().WithObjects(ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	i := &IngressSecretProviderClassReconciler{
		client: c,
		config: &config.Config{
			TenantID:    "test-tenant-id",
			MSIClientID: "test-msi-client-id",
		},
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if *ing.Spec.IngressClassName == spcTestIngressClassName {
				return true, nil
			}

			return false, nil
		}),
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create the secret provider class
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name}}
	beforeErrCount := testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err := i.Reconcile(ctx, req)
	require.NoError(t, err)

	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)

	// Prove it exists
	spc := &secv1.SecretProviderClass{}
	spc.Name = "keyvault-" + ing.Name
	spc.Namespace = ing.Namespace
	spc.Labels = manifests.GetTopLevelLabels()
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
	beforeErrCount = testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)

	// Remove the cert's version from the ingress
	ing.Annotations = map[string]string{
		"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert",
	}
	require.NoError(t, i.client.Update(ctx, ing))
	beforeErrCount = testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)

	// Prove the objectVersion property was removed
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(spc), spc))
	expected.Spec.Parameters["objects"] = "{\"array\":[\"{\\\"objectName\\\":\\\"testcert\\\",\\\"objectType\\\":\\\"secret\\\"}\"]}"
	assert.Equal(t, expected.Spec, spc.Spec)

	// Remove the cert annotation from the ingress
	ing.Annotations = map[string]string{}
	require.NoError(t, i.client.Update(ctx, ing))
	beforeErrCount = testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)

	// Prove secret class was removed
	require.True(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(spc), spc)))

	// Check for idempotence
	beforeErrCount = testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)
}

func TestIngressSecretProviderClassReconcilerIntegrationWithoutSPCLabels(t *testing.T) {
	// Create the ingress
	ing := spcTestIngress.DeepCopy()

	c := fake.NewClientBuilder().WithObjects(ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	i := &IngressSecretProviderClassReconciler{
		client: c,
		config: &config.Config{
			TenantID:    "test-tenant-id",
			MSIClientID: "test-msi-client-id",
		},
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if *ing.Spec.IngressClassName == spcTestIngressClassName {
				return true, nil
			}

			return false, nil
		}),
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create the secret provider class
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name}}
	beforeErrCount := testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err := i.Reconcile(ctx, req)
	require.NoError(t, err)

	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)

	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("keyvault-%s", ing.Name),
			Namespace: ing.Namespace,
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ing.APIVersion,
				Controller: util.ToPtr(true),
				Kind:       ing.Kind,
				Name:       ing.Name,
				UID:        ing.UID,
			}},
		},
	}

	// Get secret provider class
	require.False(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(spc), spc)))
	assert.Equal(t, len(manifests.GetTopLevelLabels()), len(spc.Labels))

	// Remove the labels
	spc.Labels = map[string]string{}
	require.NoError(t, i.client.Update(ctx, spc))
	assert.Equal(t, 0, len(spc.Labels))

	// Remove the cert annotation from the ingress
	ing.Annotations = map[string]string{}
	require.NoError(t, i.client.Update(ctx, ing))

	// Reconcile both changes
	beforeErrCount = testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)

	// Prove secret class was not removed
	require.False(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(spc), spc)))
	assert.Equal(t, 0, len(spc.Labels))
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(spc), spc))

	// Check secret provider class Spec after Reconcile
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
	beforeErrCount = testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, ingressSecretProviderControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelSuccess), beforeRequestCount)
}

func TestIngressSecretProviderClassReconcilerInvalidURL(t *testing.T) {
	// Create the ingress
	ing := spcTestIngress.DeepCopy()
	ing.Annotations = map[string]string{
		"kubernetes.azure.com/tls-cert-keyvault-uri": "inv@lid URL",
	}

	c := fake.NewClientBuilder().WithObjects(ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	recorder := record.NewFakeRecorder(10)
	i := &IngressSecretProviderClassReconciler{
		client: c,
		config: &config.Config{
			TenantID:    "test-tenant-id",
			MSIClientID: "test-msi-client-id",
		},
		events: recorder,
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if *ing.Spec.IngressClassName == spcTestIngressClassName {
				return true, nil
			}

			return false, nil
		}),
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	metrics.InitControllerMetrics(ingressSecretProviderControllerName)

	// get the before value of the error metrics
	beforeErrCount := testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelError)

	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ing.Namespace, Name: ing.Name}}
	_, err := i.Reconcile(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, "Warning InvalidInput error while processing Keyvault reference: invalid secret uri: inv@lid URL", <-recorder.Events)
	//even though no error was returned, we should expect the error count to be incremented
	afterErrCount := testutils.GetErrMetricCount(t, ingressSecretProviderControllerName)
	afterRequestCount := testutils.GetReconcileMetricCount(t, ingressSecretProviderControllerName, metrics.LabelError)

	assert.Greater(t, afterErrCount, beforeErrCount)
	assert.Greater(t, afterRequestCount, beforeRequestCount)
}

func TestIngressSecretProviderClassReconcilerBuildSPCInvalidURLs(t *testing.T) {
	i := &IngressSecretProviderClassReconciler{
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if *ing.Spec.IngressClassName == spcTestIngressClassName {
				return true, nil
			}

			return false, nil
		}),
	}

	invalidURLIng := &netv1.Ingress{
		Spec: netv1.IngressSpec{
			IngressClassName: &spcTestIngressClassName,
		},
	}

	t.Run("missing ingress class", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		ing.Spec.IngressClassName = nil
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": "inv@lid URL"}

		ok, err := i.buildSPC(ing, &secv1.SecretProviderClass{})
		assert.False(t, ok)
		require.NoError(t, err)
	})

	t.Run("incorrect ingress class", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		incorrect := "some-other-ingress-class"
		ing.Spec.IngressClassName = &incorrect
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": "inv@lid URL"}

		ok, err := i.buildSPC(ing, &secv1.SecretProviderClass{})
		assert.False(t, ok)
		require.NoError(t, err)
	})

	t.Run("nil annotations", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()

		ok, err := i.buildSPC(ing, &secv1.SecretProviderClass{})
		assert.False(t, ok)
		require.NoError(t, err)
	})

	t.Run("empty url", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": ""}

		ok, err := i.buildSPC(ing, &secv1.SecretProviderClass{})
		assert.False(t, ok)
		require.NoError(t, err)
	})

	t.Run("url with control character", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		cc := string([]byte{0x7f})
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": cc}

		ok, err := i.buildSPC(ing, &secv1.SecretProviderClass{})
		assert.False(t, ok)
		_, expectedErr := url.Parse(cc) // the exact error depends on operating system
		require.EqualError(t, err, fmt.Sprintf("%s", expectedErr))
	})

	t.Run("url with one path segment", func(t *testing.T) {
		ing := invalidURLIng.DeepCopy()
		ing.Annotations = map[string]string{"kubernetes.azure.com/tls-cert-keyvault-uri": "http://test.com/foo"}

		ok, err := i.buildSPC(ing, &secv1.SecretProviderClass{})
		assert.False(t, ok)
		require.EqualError(t, err, "invalid secret uri: http://test.com/foo")
	})
}

func TestIngressSecretProviderClassReconcilerBuildSPCCloud(t *testing.T) {
	cases := []struct {
		name, configCloud, spcCloud string
		expected                    bool
	}{
		{
			name:        "empty config cloud",
			configCloud: "",
			expected:    false,
		},
		{
			name:        "public cloud",
			configCloud: "AzurePublicCloud",
			spcCloud:    "AzurePublicCloud",
			expected:    true,
		},
		{
			name:        "sov cloud",
			configCloud: "AzureUSGovernmentCloud",
			spcCloud:    "AzureUSGovernmentCloud",
			expected:    true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			i := &IngressSecretProviderClassReconciler{
				config: &config.Config{
					Cloud: c.configCloud,
				},
				ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
					if *ing.Spec.IngressClassName == spcTestIngressClassName {
						return true, nil
					}

					return false, nil
				}),
			}

			ing := spcTestIngress.DeepCopy()
			ing.Annotations = map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test.vault.azure.net/secrets/test-secret",
			}

			spc := &secv1.SecretProviderClass{}
			ok, err := i.buildSPC(ing, spc)
			require.NoError(t, err, "building SPC should not error")
			require.True(t, ok, "SPC should be built")

			spcCloud, ok := spc.Spec.Parameters[kvcsi.CloudNameParameter]
			require.Equal(t, c.expected, ok, "SPC cloud annotation unexpected")
			require.Equal(t, c.spcCloud, spcCloud, "SPC cloud annotation doesn't match")
		})
	}
}

func TestIngressSecretProviderClassReconcilerBuildSPCFailedIsManaging(t *testing.T) {
	i := &IngressSecretProviderClassReconciler{
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			return false, fmt.Errorf("failed to get ingress")
		}),
	}

	ing := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-ingress",
			Annotations: map[string]string{},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &spcTestIngressClassName,
		},
	}
	spc := &secv1.SecretProviderClass{}

	ok, err := i.buildSPC(ing, spc)
	require.False(t, ok)
	require.Error(t, err)
	require.ErrorContains(t, err, "determining if ingress is managed")
}

func TestCertSecretName(t *testing.T) {
	require.Equal(t, "keyvault-ingressname", certSecretName("ingressname"))
	require.Equal(t, "keyvault-anotheringressname", certSecretName("anotheringressname"))
}
