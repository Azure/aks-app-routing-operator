package spc

const (
	keyVaultUriKey          = "kubernetes.azure.com/tls-cert-keyvault-uri"
	certUriTLSOption        = keyVaultUriKey
	serviceAccountTLSOption = "kubernetes.azure.com/tls-cert-service-account"
)

const istioGatewayClassName = "istio"

const wiSaClientIdAnnotation = "azure.workload.identity/client-id"
