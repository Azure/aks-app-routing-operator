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
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIngressTlsReconciler(t *testing.T) {
	managedIngressClassName := "managed.ingress.class"

	c := fake.NewClientBuilder().Build()
	recorder := record.NewFakeRecorder(10)
	i := &ingressTlsReconciler{
		client: c,
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if *ing.Spec.IngressClassName == managedIngressClassName {
				return true, nil
			}

			return false, nil
		}),
		events: recorder,
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// prove it does nothing to an unmanaged ingress
	t.Run("unmanaged ingress", func(t *testing.T) {
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

		unmanagedIngress.Annotations = map[string]string{
			tlsCertKvUriOption:       "https://mykv.vault.azure.net/secrets/mycert",
			tlsCertManagedAnnotation: "true",
		}
		require.NoError(t, c.Update(ctx, unmanagedIngress), "there should be no error updating an unmanaged ingress")
		beforeErrCount = testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling an unmanaged ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling an unmanaged ingress")

		got = &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.Nil(t, got.Spec.TLS, "we shouldn't have changed an unmanaged ingress")
	})

	// prove it does nothing to an ingress with no managed annotation
	t.Run("managed ingress with no managed annotation", func(t *testing.T) {
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
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressNoAnnotation.Namespace, Name: managedIngressNoAnnotation.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling a managed ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
		require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

		got := &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.Nil(t, got.Spec.TLS, "we shouldn't have changed a managed ingress with no annotation")

		managedIngressNoAnnotation.Annotations = map[string]string{
			tlsCertKvUriOption: "https://mykv.vault.azure.net/secrets/mycert",
		}
		require.NoError(t, c.Update(ctx, managedIngressNoAnnotation), "there should be no error updating a managed ingress")
		beforeErrCount = testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount = testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling a managed ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")

		got = &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.Nil(t, got.Spec.TLS, "we shouldn't have changed a managed ingress with no managed annotation")
	})

	// prove it doesn't reconcile an ingress with only the managed annotation
	t.Run("managed ingress with only managed annotation", func(t *testing.T) {
		managedIngressOnlyManagedAnnotation := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "managed-only-managed-annotation",
				Annotations: map[string]string{
					tlsCertManagedAnnotation: "true",
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
		require.NoError(t, c.Create(ctx, managedIngressOnlyManagedAnnotation), "there should be no error creating a managed ingress")

		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressOnlyManagedAnnotation.Namespace, Name: managedIngressOnlyManagedAnnotation.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling a managed ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
		require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

		got := &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.Nil(t, got.Spec.TLS, "we shouldn't have changed a managed ingress with only the managed annotation")

		// prove we sent a warning event
		require.Equal(t, "Warning KeyvaultUriAnnotationMissing Ingress has kubernetes.azure.com/tls-cert-keyvault-managed annotation but is missing kubernetes.azure.com/tls-cert-keyvault-uri annotation. kubernetes.azure.com/tls-cert-keyvault-uri annotation is needed to manage Ingress TLS.", <-recorder.Events, "warning event should have been sent")
	})

	// prove it reconciles an ingress with an annotation but pre-existing TLS
	t.Run("managed ingress with annotation and pre-existing TLS", func(t *testing.T) {
		managedIngressDefinedTls := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "managed-defined-tls",
				Annotations: map[string]string{
					tlsCertKvUriOption:       "https://mykv.vault.azure.net/secrets/mycert",
					tlsCertManagedAnnotation: "true",
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
					{
						Host: "managed.example.com2",
					},
				},
			},
		}
		require.NoError(t, c.Create(ctx, managedIngressDefinedTls), "there should be no error creating a managed ingress")
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressDefinedTls.Namespace, Name: managedIngressDefinedTls.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling a managed ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
		require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

		got := &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.Len(t, got.Spec.TLS, 1, "we should have replaced the TLS entry")
		require.Equal(t, certSecretName(managedIngressDefinedTls.Name), got.Spec.TLS[0].SecretName, "we should have changed the preexisting TLS secret name")
		require.Equal(t, []string{"managed.example.com", "managed.example.com2"}, got.Spec.TLS[0].Hosts, "we should have changed the managed hosts to the TLS entry")
	})

	// prove it properly reconciles single host
	t.Run("managed ingress with single host", func(t *testing.T) {
		managedIngress := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "managed-single-host",
				Annotations: map[string]string{
					tlsCertKvUriOption:       "https://mykv.vault.azure.net/secrets/mycert",
					tlsCertManagedAnnotation: "true",
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
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngress.Namespace, Name: managedIngress.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling a managed ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
		require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

		got := &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.NotNil(t, got.Spec.TLS, "we should have added TLS to a managed ingress")
		require.Equal(t, 1, len(got.Spec.TLS), "we should have added TLS to a managed ingress")
		require.Equal(t, "managed.example.com", got.Spec.TLS[0].Hosts[0], "we should have added TLS to a managed ingress")
		require.Equal(t, certSecretName(managedIngress.Name), got.Spec.TLS[0].SecretName, "we should have added TLS to a managed ingress")
	})

	// prove it properly reconciles multiple hosts
	t.Run("managed ingress with multiple hosts", func(t *testing.T) {
		managedIngressMultipleHosts := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "managed-multiple-hosts",
				Annotations: map[string]string{
					tlsCertKvUriOption:       "https://mykv.vault.azure.net/secrets/mycert",
					tlsCertManagedAnnotation: "true",
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
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressMultipleHosts.Namespace, Name: managedIngressMultipleHosts.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling a managed ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
		require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

		got := &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.NotNil(t, got.Spec.TLS, "we should have added TLS to a managed ingress")
		require.Equal(t, 1, len(got.Spec.TLS), "we should have added TLS to a managed ingress")
		require.Equal(t, 3, len(got.Spec.TLS[0].Hosts), "we should have added TLS to a managed ingress")
		require.Equal(t, "managed.example.com", got.Spec.TLS[0].Hosts[0], "we should have added TLS to a managed ingress")
		require.Equal(t, "managed2.example.com", got.Spec.TLS[0].Hosts[1], "we should have added TLS to a managed ingress")
		require.Equal(t, "managed3.example.com", got.Spec.TLS[0].Hosts[2], "we should have added TLS to a managed ingress")
		require.Equal(t, certSecretName(managedIngressMultipleHosts.Name), got.Spec.TLS[0].SecretName, "we should have added TLS to a managed ingress")
	})

	// prove it properly reconciles multiple hosts
	t.Run("managed ingress with some hosts", func(t *testing.T) {
		managedIngressSomeHosts := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "managed-some-hosts",
				Annotations: map[string]string{
					tlsCertKvUriOption:       "https://mykv.vault.azure.net/secrets/mycert",
					tlsCertManagedAnnotation: "true",
				},
			},
			Spec: netv1.IngressSpec{
				IngressClassName: &managedIngressClassName,
				Rules: []netv1.IngressRule{
					{
						Host: "managed.example.com",
					},
					{}, // empty host, shouldn't do anything
					{
						Host: "managed3.example.com",
					},
				},
			},
		}
		require.NoError(t, c.Create(ctx, managedIngressSomeHosts), "there should be no error creating a managed ingress")
		req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: managedIngressSomeHosts.Namespace, Name: managedIngressSomeHosts.Name}}
		beforeErrCount := testutils.GetErrMetricCount(t, ingressTlsControllerName)
		beforeRequestCount := testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess)
		_, err = i.Reconcile(ctx, req)
		require.NoError(t, err, "there should be no error reconciling a managed ingress")
		require.Equal(t, beforeErrCount, testutils.GetErrMetricCount(t, ingressTlsControllerName), "there should be no change in the error count reconciling a managed ingress")
		require.Equal(t, beforeRequestCount+1, testutils.GetReconcileMetricCount(t, ingressTlsControllerName, metrics.LabelSuccess), "there should be one more successful reconcile count reconciling a managed ingress")

		got := &netv1.Ingress{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, got))
		require.NotNil(t, got.Spec.TLS, "we should have added TLS to a managed ingress")
		require.Equal(t, 1, len(got.Spec.TLS), "we should have added TLS to a managed ingress")
		require.Equal(t, 2, len(got.Spec.TLS[0].Hosts), "we should have added TLS to a managed ingress")
		require.Equal(t, "managed.example.com", got.Spec.TLS[0].Hosts[0], "we should have added TLS to a managed ingress")
		require.Equal(t, "managed3.example.com", got.Spec.TLS[0].Hosts[1], "we should have added TLS to a managed ingress")
		require.Equal(t, certSecretName(managedIngressSomeHosts.Name), got.Spec.TLS[0].SecretName, "we should have added TLS to a managed ingress")
	})
}
