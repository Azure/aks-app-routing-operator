package spc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	parseTestVaultName   = "myvault"
	parseTestCertName    = "mycert"
	parseTestVersion     = "abc123"
	parseTestVaultDomain = "vault.azure.net"
	parseTestInvalidUri  = "not-a-url"
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
			certURI: "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates/" + parseTestCertName + "/" + parseTestVersion,
			expected: certReference{
				vaultName:     parseTestVaultName,
				certName:      parseTestCertName,
				objectVersion: parseTestVersion,
			},
			expectError: false,
		},
		{
			name:    "valid uri without version",
			certURI: "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates/" + parseTestCertName,
			expected: certReference{
				vaultName:     parseTestVaultName,
				certName:      parseTestCertName,
				objectVersion: "",
			},
			expectError: false,
		},
		{
			name:    "valid uri with dashes in names",
			certURI: "https://my-vault-123." + parseTestVaultDomain + "/certificates/my-cert-456",
			expected: certReference{
				vaultName:     "my-vault-123",
				certName:      "my-cert-456",
				objectVersion: "",
			},
			expectError: false,
		},
		{
			name:        "invalid uri - malformed url",
			certURI:     parseTestInvalidUri,
			expectError: true,
		},
		{
			name:        "invalid uri - missing secret name",
			certURI:     "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates",
			expectError: true,
		},
		{
			name:        "empty uri",
			certURI:     "",
			expectError: true,
		},
		{
			name:        "no certificate name",
			certURI:     "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates/",
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
