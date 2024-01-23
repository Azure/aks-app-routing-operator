package keyvault

import (
	"context"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIngressTlsReconciler(t *testing.T) {
	managedIngressClassName := "managed.ingress.class"

	c := fake.NewClientBuilder().Build()
	i := &ingressTlsReconciler{
		client: c,
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if *ing.Spec.IngressClassName == managedIngressClassName {
				return true, nil
			}

			return false, nil
		}),
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// prove it does nothing to an unmanaged ingress
	unmanagedIngress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unmanaged",
		},
		Spec: netv1.IngressSpec{
			IngressClassName: util.ToPtr("unmanaged.ingress.class"),
			Rules: []netv1.IngressRule{
				{
					Host: "unmanaged.example.com",
				},
			},
		},
	}
	require.NoError(t, c.Create(ctx, unmanagedIngress), "there should be no error creating an unmanaged ingress")
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: unmanagedIngress.Namespace, Name: unmanagedIngress.Name}}
	beforeErrCount := testutils.GetErrMetricCount(t, ingressTlsControllerName)
	beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
	_, err := i.Reconcile(ctx, req)
	require.NoError(t, err, "there should be no error reconciling an unmanaged ingress")
	require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling an unmanaged ingress")
	require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling an unmanaged ingress")

	got := &netv1.Ingress{}
	require.NoError(t, c.Get(ctx, req.NamespacedName, got))
	require.Nil(t, got.Spec.TLS, "we shouldn't have changed an unmanaged ingress")

	// prove it does nothing to an ingress with no annotation
	managedIngressNoAnnotation := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "managed-no-annotation",
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &managedIngressClassName,
			Rules: []netv1.IngressRule{
				{
					Host: "managed.example.com",
				},
			},
		},
	}
	require.NoError(t, c.Create(ctx, managedIngressNoAnnotation), "there should be no error creating a managed ingress")
	req = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressNoAnnotation.Namespace, Name: managedIngressNoAnnotation.Name}}
	beforeErrCount = testutils.GetErrMetricCount(t, ingressTlsControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err, "there should be no error reconciling a managed ingress")
	require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
	require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

	got = &netv1.Ingress{}
	require.NoError(t, c.Get(ctx, req.NamespacedName, got))
	require.Nil(t, got.Spec.TLS, "we shouldn't have changed a managed ingress with no annotation")

	// prove it does nothing to an ingress with an annotation but pre-existing TLS
	managedIngressDefinedTls := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "managed-defined-tls",
			Annotations: map[string]string{
				tlsCertKvUriAnnotation: "https://mykv.vault.azure.net/secrets/mycert",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &managedIngressClassName,
			TLS: []netv1.IngressTLS{
				{
					SecretName: "custom-tls-secret",
					Hosts:      []string{"custom.host"},
				},
			},
			Rules: []netv1.IngressRule{
				{
					Host: "managed.example.com",
				},
			},
		},
	}
	require.NoError(t, c.Create(ctx, managedIngressDefinedTls), "there should be no error creating a managed ingress")
	req = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressDefinedTls.Namespace, Name: managedIngressDefinedTls.Name}}
	beforeErrCount = testutils.GetErrMetricCount(t, ingressTlsControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err, "there should be no error reconciling a managed ingress")
	require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
	require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

	got = &netv1.Ingress{}
	require.NoError(t, c.Get(ctx, req.NamespacedName, got))
	require.Equal(t, managedIngressDefinedTls.Spec.TLS, got.Spec.TLS, "we shouldn't have changed a managed ingress with an annotation but pre-existing TLS")

	// prove it properly reconciles single host
	managedIngress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "managed-single-host",
			Annotations: map[string]string{
				tlsCertKvUriAnnotation: "https://mykv.vault.azure.net/secrets/mycert",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &managedIngressClassName,
			Rules: []netv1.IngressRule{
				{
					Host: "managed.example.com",
				},
			},
		},
	}
	require.NoError(t, c.Create(ctx, managedIngress), "there should be no error creating a managed ingress")
	req = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngress.Namespace, Name: managedIngress.Name}}
	beforeErrCount = testutils.GetErrMetricCount(t, ingressTlsControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err, "there should be no error reconciling a managed ingress")
	require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
	require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

	got = &netv1.Ingress{}
	require.NoError(t, c.Get(ctx, req.NamespacedName, got))
	require.NotNil(t, got.Spec.TLS, "we should have added TLS to a managed ingress")
	require.Equal(t, 1, len(got.Spec.TLS), "we should have added TLS to a managed ingress")
	require.Equal(t, "managed.example.com", got.Spec.TLS[0].Hosts[0], "we should have added TLS to a managed ingress")
	require.Equal(t, certSecretName(managedIngress.Name), got.Spec.TLS[0].SecretName, "we should have added TLS to a managed ingress")

	// prove it properly reconciles multiple hosts
	managedIngressMultipleHosts := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "managed-multiple-hosts",
			Annotations: map[string]string{
				tlsCertKvUriAnnotation: "https://mykv.vault.azure.net/secrets/mycert",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &managedIngressClassName,
			Rules: []netv1.IngressRule{
				{
					Host: "managed.example.com",
				},
				{
					Host: "managed2.example.com",
				},
				{
					Host: "managed3.example.com",
				},
			},
		},
	}
	require.NoError(t, c.Create(ctx, managedIngressMultipleHosts), "there should be no error creating a managed ingress")
	req = ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressMultipleHosts.Namespace, Name: managedIngressMultipleHosts.Name}}
	beforeErrCount = testutils.GetErrMetricCount(t, ingressTlsControllerName)
	beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
	_, err = i.Reconcile(ctx, req)
	require.NoError(t, err, "there should be no error reconciling a managed ingress")
	require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
	require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

	got = &netv1.Ingress{}
	require.NoError(t, c.Get(ctx, req.NamespacedName, got))
	require.NotNil(t, got.Spec.TLS, "we should have added TLS to a managed ingress")
	require.Equal(t, 1, len(got.Spec.TLS), "we should have added TLS to a managed ingress")
	require.Equal(t, 3, len(got.Spec.TLS[0].Hosts), "we should have added TLS to a managed ingress")
	require.Equal(t, "managed.example.com", got.Spec.TLS[0].Hosts[0], "we should have added TLS to a managed ingress")
	require.Equal(t, "managed2.example.com", got.Spec.TLS[0].Hosts[1], "we should have added TLS to a managed ingress")
	require.Equal(t, "managed3.example.com", got.Spec.TLS[0].Hosts[2], "we should have added TLS to a managed ingress")
	require.Equal(t, certSecretName(managedIngressMultipleHosts.Name), got.Spec.TLS[0].SecretName, "we should have added TLS to a managed ingress")
}
