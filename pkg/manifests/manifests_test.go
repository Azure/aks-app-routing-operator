// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var integrationTestCases = []struct {
	Name string
	Conf *config.Config
}{
	{
		Name: "full",
		Conf: &config.Config{
			NS:            "test-namespace",
			Registry:      "test-registry",
			MSIClientID:   "test-msi-client-id",
			TenantID:      "test-tenant-id",
			Cloud:         "test-cloud",
			Location:      "test-location",
			DNSZoneRG:     "test-dns-zone-rg",
			DNSZoneSub:    "test-dns-zone-sub",
			DNSZoneDomain: "test-dns-zone-domain",
		},
	},
	{
		Name: "no-dns",
		Conf: &config.Config{
			NS:          "test-namespace",
			Registry:    "test-registry",
			MSIClientID: "test-msi-client-id",
			TenantID:    "test-tenant-id",
			Cloud:       "test-cloud",
			Location:    "test-location",
		},
	},
}

func TestIngressControllerResources(t *testing.T) {
	for _, tc := range integrationTestCases {
		objs := IngressControllerResources(tc.Conf)

		actual, err := json.MarshalIndent(&objs, "  ", "  ")
		require.NoError(t, err)

		fixture := path.Join("fixtures", tc.Name) + ".json"
		if os.Getenv("GENERATE_FIXTURES") != "" {
			err = ioutil.WriteFile(fixture, actual, 0644)
			require.NoError(t, err)
			continue
		}

		expected, err := ioutil.ReadFile(fixture)
		require.NoError(t, err)

		assert.JSONEq(t, string(expected), string(actual))
	}
}
