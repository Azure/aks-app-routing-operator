package spc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseKeyVaultCertURI(t *testing.T) {
	tests := []struct {
		name        string
		certURI     string
		expected    certReference
		expectError bool
	}{
		{
			name:    "valid uri with version",
			certURI: "https://myvault.vault.azure.net/certificates/mycert/abc123",
			expected: certReference{
				vaultName:     "myvault",
				certName:      "mycert",
				objectVersion: "abc123",
			},
			expectError: false,
		},
		{
			name:    "valid uri without version",
			certURI: "https://myvault.vault.azure.net/certificates/mycert",
			expected: certReference{
				vaultName:     "myvault",
				certName:      "mycert",
				objectVersion: "",
			},
			expectError: false,
		},
		{
			name:    "valid uri with dashes in names",
			certURI: "https://my-vault-123.vault.azure.net/certificates/my-cert-456",
			expected: certReference{
				vaultName:     "my-vault-123",
				certName:      "my-cert-456",
				objectVersion: "",
			},
			expectError: false,
		},
		{
			name:        "invalid uri - malformed url",
			certURI:     "not-a-url",
			expectError: true,
		},
		{
			name:        "invalid uri - missing secret name",
			certURI:     "https://myvault.vault.azure.net/certificates",
			expectError: true,
		},
		{
			name:        "empty uri",
			certURI:     "",
			expectError: true,
		},
		{
			name:        "no certificate name",
			certURI:     "https://myvault.vault.azure.net/certificates/",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseKeyVaultCertURI(tt.certURI)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected.vaultName, result.vaultName)
			assert.Equal(t, tt.expected.certName, result.certName)
			assert.Equal(t, tt.expected.objectVersion, result.objectVersion)
		})
	}
}
