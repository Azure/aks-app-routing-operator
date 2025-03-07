package keyvault

const (
	tlsCertKvUriAnnotation   = "kubernetes.azure.com/tls-cert-keyvault-uri"
	tlsCertManagedAnnotation = "kubernetes.azure.com/tls-cert-keyvault-managed"

	istioGatewayClassName = "istio"

	certUriTLSOption        = "kubernetes.azure.com/tls-cert-keyvault-uri"
	serviceAccountTLSOption = "kubernetes.azure.com/tls-cert-service-account"
)
