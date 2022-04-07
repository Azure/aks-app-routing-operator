// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package service

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

func TestIngressReconcilerIntegration(t *testing.T) {
	svc := &corev1.Service{}
	svc.UID = "test-svc-uid"
	svc.Name = "test-service"
	svc.Namespace = "test-ns"
	svc.Spec.Ports = []corev1.ServicePort{{
		Port: 123,
	}}

	c := fake.NewClientBuilder().WithObjects(svc).Build()
	p := &IngressReconciler{client: c}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// No ingress is created for service without any of our annotations
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}
	_, err := p.Reconcile(ctx, req)
	require.NoError(t, err)

	ing := &netv1.Ingress{}
	ing.Name = svc.Name
	ing.Namespace = svc.Namespace
	assert.True(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(ing), ing)))

	// Add required annotations and prove the expected ingress is created
	svc.Annotations = map[string]string{
		"aks.io/ingress-host":          "test-host",
		"aks.io/tls-cert-keyvault-uri": "test-cert-uri",
	}
	require.NoError(t, c.Update(ctx, svc))
	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)

	pt := netv1.PathTypePrefix
	expected := &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            svc.Name,
			Namespace:       svc.Namespace,
			ResourceVersion: "1",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				BlockOwnerDeletion: util.BoolPtr(true),
				Controller:         util.BoolPtr(true),
				Kind:               "Service",
				Name:               svc.Name,
				UID:                svc.UID,
			}},
			Annotations: map[string]string{
				"aks.io/tls-cert-keyvault-uri":                      "test-cert-uri",
				"aks.io/use-osm-mtls":                               "true",
				"nginx.ingress.kubernetes.io/backend-protocol":      "HTTPS",
				"nginx.ingress.kubernetes.io/configuration-snippet": "\nproxy_ssl_name \"default.test-ns.cluster.local\";",
				"nginx.ingress.kubernetes.io/proxy-ssl-secret":      "kube-system/osm-ingress-client-cert",
				"nginx.ingress.kubernetes.io/proxy-ssl-verify":      "on",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: util.StringPtr("webapprouting.aks.io"),
			Rules: []netv1.IngressRule{{
				Host: "test-host",
				IngressRuleValue: netv1.IngressRuleValue{
					HTTP: &netv1.HTTPIngressRuleValue{
						Paths: []netv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pt,
							Backend: netv1.IngressBackend{
								Service: &netv1.IngressServiceBackend{
									Name: svc.Name,
									Port: netv1.ServiceBackendPort{Number: svc.Spec.Ports[0].Port},
								},
							},
						}},
					},
				},
			}},
			TLS: []netv1.IngressTLS{{
				Hosts:      []string{"test-host"},
				SecretName: "keyvault-test-service",
			}},
		},
	}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ing), ing))
	assert.Equal(t, expected, ing)

	// Override the default service account name
	svc.Annotations["aks.io/service-account-name"] = "test-sa"
	require.NoError(t, c.Update(ctx, svc))
	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)

	expected.Annotations["nginx.ingress.kubernetes.io/configuration-snippet"] = "\nproxy_ssl_name \"test-sa.test-ns.cluster.local\";"
	expected.ResourceVersion = "2"
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ing), ing))
	assert.Equal(t, expected, ing)
}
