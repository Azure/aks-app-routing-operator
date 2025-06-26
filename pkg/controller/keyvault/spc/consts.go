package spc

const (
	keyVaultUriKey           = "kubernetes.azure.com/tls-cert-keyvault-uri"
	certUriTLSOption         = keyVaultUriKey
	tlsCertManagedAnnotation = "kubernetes.azure.com/tls-cert-keyvault-managed"
)

const istioGatewayClassName = "istio"
