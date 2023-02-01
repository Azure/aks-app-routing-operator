// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var integrationTestCases = []struct {
	Name   string
	Conf   *config.Config
	Deploy *appsv1.Deployment
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
		Deploy: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-operator-deploy",
				UID:  "test-operator-deploy-uid",
			},
		},
	},
	{
		Name: "no-ownership",
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
		Name: "kube-system",
		Conf: &config.Config{
			NS:            "kube-system",
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
		Name: "optional-features-disabled",
		Conf: &config.Config{
			NS:              "test-namespace",
			Registry:        "test-registry",
			MSIClientID:     "test-msi-client-id",
			TenantID:        "test-tenant-id",
			Cloud:           "test-cloud",
			Location:        "test-location",
			DisableKeyvault: true,
			DisableOSM:      true,
		},
	},
}

func TestIngressControllerResources(t *testing.T) {
	for _, tc := range integrationTestCases {
		objs := NginxIngressControllerResources(tc.Conf, tc.Deploy)

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
