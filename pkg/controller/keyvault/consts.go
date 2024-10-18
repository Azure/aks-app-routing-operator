package keyvault

const (
	tlsCertKvUriAnnotation   = "kubernetes.azure.com/tls-cert-keyvault-uri"
	tlsCertManagedAnnotation = "kubernetes.azure.com/tls-cert-keyvault-managed"

	certUriTLSOption        = "kubernetes.azure.com/tls-cert-keyvault-uri"
	clientIdTLSOption       = "kubernetes.azure.com/tls-cert-client-id"
	serviceAccountTLSOption = "kubernetes.azure.com/tls-cert-service-account"

	wiSaClientIdAnnotation = "azure.workload.identity/client-id"
)
