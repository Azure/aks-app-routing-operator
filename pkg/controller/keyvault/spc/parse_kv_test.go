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
		name           string
		certURI        string
		expected       certReference
		expectErrorStr string
	}{
		{
			name:    "valid uri with version",
			certURI: "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates/" + parseTestCertName + "/" + parseTestVersion,
			expected: certReference{
				vaultName:     parseTestVaultName,
				certName:      parseTestCertName,
				objectVersion: parseTestVersion,
			},
		},
		{
			name:    "valid uri without version",
			certURI: "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates/" + parseTestCertName,
			expected: certReference{
				vaultName:     parseTestVaultName,
				certName:      parseTestCertName,
				objectVersion: "",
			},
		},
		{
			name:    "valid uri with dashes in names",
			certURI: "https://my-vault-123." + parseTestVaultDomain + "/certificates/my-cert-456",
			expected: certReference{
				vaultName:     "my-vault-123",
				certName:      "my-cert-456",
				objectVersion: "",
			},
		},
		{
			name:           "invalid uri - malformed url",
			certURI:        parseTestInvalidUri,
			expectErrorStr: "uri path contains too few segments",
		},
		{
			name:           "invalid uri - missing secret name",
			certURI:        "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates",
			expectErrorStr: "uri path contains too few segments",
		},
		{
			name:           "empty uri",
			certURI:        "",
			expectErrorStr: "uri path contains too few segments",
		},
		{
			name:           "no certificate name",
			certURI:        "https://" + parseTestVaultName + "." + parseTestVaultDomain + "/certificates/",
			expectErrorStr: "vault name or secret name is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseKeyVaultCertURI(tt.certURI)

			if tt.expectErrorStr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErrorStr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expected.vaultName, result.vaultName)
			assert.Equal(t, tt.expected.certName, result.certName)
			assert.Equal(t, tt.expected.objectVersion, result.objectVersion)
		})
	}
}
