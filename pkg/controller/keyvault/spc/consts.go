package spc

import "github.com/Azure/aks-app-routing-operator/pkg/util"

const (
	keyVaultUriKey           = "kubernetes.azure.com/tls-cert-keyvault-uri"
	certUriTLSOption         = keyVaultUriKey
	tlsCertManagedAnnotation = "kubernetes.azure.com/tls-cert-keyvault-managed"
	// IngressServiceAccountTLSAnnotation is the annotation used to specify the TLS workload identity sa
	IngressServiceAccountTLSAnnotation = util.ServiceAccountTLSOption
)

const istioGatewayClassName = "istio"
