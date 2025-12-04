package defaultdomain

import "time"

// TLSCertificate represents a TLS certificate
type TLSCertificate struct {
	// Key is the private key
	Key []byte `json:"key,omitempty"`
	// Cert is the certificate
	Cert []byte `json:"cert,omitempty"`
	// ExpiresOn is the expiration date of the certificate
	ExpiresOn *time.Time `json:"expires_on,omitempty"`
}
