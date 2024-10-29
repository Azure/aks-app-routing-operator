package keyvault

import (
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func modifyGateway(gwObj *gatewayv1.Gateway, modifier func(*gatewayv1.Gateway)) *gatewayv1.Gateway {
	ret := gwObj.DeepCopy()
	modifier(ret)
	return ret
}

var (
	gwWithCertWithoutOthers = &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
						},
					},
				},
			},
		},
	}

	gatewayWithOnlyClientId = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
						},
					},
				},
			},
		},
	}

	gatewayWithMultipleListenersAndOnlyOneClientId = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
						},
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id",
						},
					},
				},
			},
		},
	}

	gwWithSa = &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
							"kubernetes.azure.com/tls-cert-service-account": "test-sa",
						},
					},
				},
			},
		},
	}

	gatewayWithCid = &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
							"kubernetes.azure.com/tls-cert-client-id":    "test-client-id",
						},
					},
				},
			},
		},
	}

	gatewayWithCidListenerAndSaListener = &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-keyvault-uri": "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
							"kubernetes.azure.com/tls-cert-client-id":    "test-client-id",
						},
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a35",
							"kubernetes.azure.com/tls-cert-service-account": "test-sa",
						},
					},
				},
			},
		},
	}

	gwWithCidAndSaInSameListener = &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34",
							"kubernetes.azure.com/tls-cert-client-id":       "test-client-id",
							"kubernetes.azure.com/tls-cert-service-account": "test-sa",
						},
					},
				},
			},
		},
	}

	gatewayWithOnlyServiceAccounts = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-service-account": "test-sa-1",
						},
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-service-account": "test-sa-2",
						},
					},
				},
			},
		},
	}

	gwWithNoCertMultipleCid = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id-1",
						},
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"kubernetes.azure.com/tls-cert-client-id": "test-client-id-2",
						},
					},
				},
			},
		},
	}

	gwWithoutTls = &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: "gateway.networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
					},
				},
			},
		},
	}

	nilOptionsGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{
				{
					Name: "test-listener-1",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: nil,
					},
				},
				{
					Name: "test-listener-2",
					TLS: &gatewayv1.GatewayTLSConfig{
						Options: nil,
					},
				},
			},
		},
	}

	noListenersGateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners:        []gatewayv1.Listener{},
		},
	}

	nonIstioGateway = modifyGateway(gwWithSa, func(gwObj *gatewayv1.Gateway) { gwObj.Spec.GatewayClassName = "" })

	clientIdSpc = &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kv-gw-cert-test-gw-test-listener",
			Namespace: "test-ns",
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "gateway.networking.k8s.io/v1",
				Controller: util.ToPtr(true),
				Kind:       "Gateway",
				Name:       "test-gw",
			}},
		},
		Spec: secv1.SecretProviderClassSpec{
			Provider: secv1.Provider("azure"),
			SecretObjects: []*secv1.SecretObject{{
				SecretName: "kv-gw-cert-test-gw-test-listener",
				Type:       "kubernetes.io/tls",
				Data: []*secv1.SecretObjectData{
					{
						ObjectName: "testcert",
						Key:        "tls.key",
					},
					{
						ObjectName: "testcert",
						Key:        "tls.crt",
					},
				},
			}},
			// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/getting-started/usage/#create-your-own-secretproviderclass-object
			Parameters: map[string]string{
				"keyvaultName":           "testvault",
				"useVMManagedIdentity":   "true",
				"userAssignedIdentityID": "test-client-id",
				"tenantId":               "test-tenant-id",
				"objects":                "{\"array\":[\"{\\\"objectName\\\":\\\"testcert\\\",\\\"objectType\\\":\\\"secret\\\",\\\"objectVersion\\\":\\\"f8982febc6894c0697b884f946fb1a34\\\"}\"]}",
			},
		},
	}

	serviceAccountSpc = &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kv-gw-cert-test-gw-test-listener-2",
			Namespace: "test-ns",
			Labels:    manifests.GetTopLevelLabels(),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "gateway.networking.k8s.io/v1",
				Controller: util.ToPtr(true),
				Kind:       "Gateway",
				Name:       "test-gw",
			}},
		},
		Spec: secv1.SecretProviderClassSpec{
			Provider: secv1.Provider("azure"),
			SecretObjects: []*secv1.SecretObject{{
				SecretName: "kv-gw-cert-test-gw-test-listener-2",
				Type:       "kubernetes.io/tls",
				Data: []*secv1.SecretObjectData{
					{
						ObjectName: "testcert",
						Key:        "tls.key",
					},
					{
						ObjectName: "testcert",
						Key:        "tls.crt",
					},
				},
			}},
			// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/getting-started/usage/#create-your-own-secretproviderclass-object
			Parameters: map[string]string{
				"keyvaultName":           "testvault",
				"useVMManagedIdentity":   "true",
				"userAssignedIdentityID": "test-client-id",
				"tenantId":               "test-tenant-id",
				"objects":                "{\"array\":[\"{\\\"objectName\\\":\\\"testcert\\\",\\\"objectType\\\":\\\"secret\\\",\\\"objectVersion\\\":\\\"f8982febc6894c0697b884f946fb1a35\\\"}\"]}",
			},
		},
	}

	annotatedServiceAccount = &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"azure.workload.identity/client-id": "test-client-id",
			},
		},
	}

	appRoutingSa = &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "azure-app-routing-kv",
			Namespace:   "test-ns",
			Annotations: map[string]string{"azure.workload.identity/client-id": "test-client-id"},
		},
	}
)
