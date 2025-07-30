// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/require"
)

const (
	validCrdPath      = "../../config/crd/bases/"
	notADirectoryPath = "./config.go"
	notAValidPath     = "./does/not/exist"
)

var validateTestCases = []struct {
	Name    string
	Conf    *Config
	Error   string
	DnsZone string
}{
	{
		Name: "valid-minimal",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  validCrdPath,
		},
	},
	{
		Name: "valid-full",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  validCrdPath,
		},
	},
	{
		Name: "missing operator deployment",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			CrdPath:                  validCrdPath,
		},
		Error: "--operator-deployment is required",
	},
	{
		Name: "nonexistent crd path",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  notAValidPath,
		},
		Error: fmt.Sprintf("crd path %s does not exist", notAValidPath),
	},
	{
		Name: "non-directory crd path",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  notADirectoryPath,
		},
		Error: fmt.Sprintf("crd path %s is not a directory", notADirectoryPath),
	},
	{
		Name: "missing-namespace",
		Conf: &Config{
			DefaultController:        Standard,
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			OperatorDeployment:       "app-routing-operator",
			ClusterUid:               "test-cluster-uid",
		},
		Error: "--namespace is required",
	},
	{
		Name: "missing-registry",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			OperatorDeployment:       "app-routing-operator",
			ClusterUid:               "test-cluster-uid",
		},
		Error: "--registry is required",
	},
	{
		Name: "missing-msi",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			OperatorDeployment:       "app-routing-operator",
			ClusterUid:               "test-cluster-uid",
		},
		Error: "--msi is required",
	},
	{
		Name: "missing-tenant-id",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			OperatorDeployment:       "app-routing-operator",
			ClusterUid:               "test-cluster-uid",
		},
		Error: "--tenant-id is required",
	},
	{
		Name: "missing-cloud",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			OperatorDeployment:       "app-routing-operator",
			ClusterUid:               "test-cluster-uid",
		},
		Error: "--cloud is required",
	},
	{
		Name: "missing-location",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			OperatorDeployment:       "app-routing-operator",
			ClusterUid:               "test-cluster-uid",
		},
		Error: "--location is required",
	},
	{
		Name: "low-concurrency-watchdog-thres",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 100,
			ConcurrencyWatchdogVotes: 2,
			OperatorDeployment:       "app-routing-operator",
			ClusterUid:               "test-cluster-uid",
		},
		Error: "--concurrency-watchdog-threshold must be greater than 100",
	},
	{
		Name: "missing-concurrency-watchdog-thres",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorDeployment:       "app-routing-operator",
			ConcurrencyWatchdogThres: 101,
		},
		Error: "--concurrency-watchdog-votes must be a positive number",
	},
	{
		Name: "missing-cluster-uid",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorDeployment:       "app-routing-operator",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
		},
		Error: "--cluster-uid is required",
	},
	{
		Name: "invalid-dns-zone-id",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorDeployment:       "app-routing-operator",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
		},
		Error:   "while parsing dns zone resource ID invalid: parsing failed for invalid. Invalid resource Id format",
		DnsZone: "invalid,dns,zone",
	},
	{
		Name: "valid-enable-default-domain-with-cert-path",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  validCrdPath,
			EnableDefaultDomain:      true,
			DefaultDomainCertPath:    "./test_default_domain_cert_path.txt",
		},
	},
	{
		Name: "invalid-enable-default-domain-missing-cert-path",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  validCrdPath,
			EnableDefaultDomain:      true,
			DefaultDomainCertPath:    "",
		},
		Error: "--default-domain-cert-path is required when --enable-default-domain is set",
	},
	{
		Name: "invalid-default-domain-cert-path-without-enable",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  validCrdPath,
			EnableDefaultDomain:      false,
			DefaultDomainCertPath:    "./test_default_domain_cert_path.txt",
		},
		Error: "--default-domain-cert-path is not allowed when --enable-default-domain is not set",
	},
	{
		Name: "valid-disable-default-domain-no-cert-path",
		Conf: &Config{
			DefaultController:        Standard,
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			OperatorDeployment:       "app-routing-operator",
			CrdPath:                  validCrdPath,
			EnableDefaultDomain:      false,
			DefaultDomainCertPath:    "",
		},
	},
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range validateTestCases {
		t.Run(tc.Name, func(t *testing.T) {
			dnsZonesString = tc.DnsZone
			err := tc.Conf.Validate()
			if tc.Error == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.Error)
			}
		})
	}
}

var (
	privateZoneOne = "/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-one.com"
	privateZoneTwo = "/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-two.com"
	privateZones   = map[string]struct{}{
		strings.ToLower(privateZoneOne): {},
		strings.ToLower(privateZoneTwo): {},
	}

	publicZoneOne = "/subscriptions/test-public-subscription/resourceGroups/test-rg-public/providers/Microsoft.Network/dnszones/test-one.com"
	publicZoneTwo = "/subscriptions/test-public-subscription/resourceGroups/test-rg-public/providers/Microsoft.Network/dnszones/test-two.com"
	publicZones   = map[string]struct{}{
		strings.ToLower(publicZoneOne): {},
		strings.ToLower(publicZoneTwo): {},
	}

	parseTestCases = []struct {
		name                 string
		zonesString          string
		expectedError        error
		expectedPrivateZones map[string]struct{}
		expectedPublicZones  map[string]struct{}
	}{
		{
			name:                 "full",
			zonesString:          strings.Join(append(util.Keys(privateZones), util.Keys(publicZones)...), ","),
			expectedPrivateZones: privateZones,
			expectedPublicZones:  publicZones,
		},
		{
			name:                 "private-only",
			zonesString:          strings.Join(util.Keys(privateZones), ","),
			expectedPrivateZones: privateZones,
			expectedPublicZones:  nil,
		},
		{
			name:                 "public-only",
			zonesString:          strings.Join(util.Keys(publicZones), ","),
			expectedPrivateZones: nil,
			expectedPublicZones:  publicZones,
		},
		{
			name:                 "full-with-duplicates",
			zonesString:          strings.Join(append(util.Keys(privateZones), append([]string{publicZoneOne, publicZoneTwo}, util.Keys(publicZones)...)...), ","),
			expectedPrivateZones: privateZones,
			expectedPublicZones:  publicZones,
		},
		{
			name:                 "private-diff-casing-duplicates",
			zonesString:          strings.Join([]string{strings.ToLower(privateZoneTwo), strings.ToUpper(privateZoneTwo)}, ","),
			expectedPrivateZones: map[string]struct{}{strings.ToLower(privateZoneTwo): {}},
			expectedPublicZones:  nil,
		},
		{
			name:                 "public-diff-casing-duplicates",
			zonesString:          strings.Join([]string{strings.ToLower(publicZoneOne), strings.ToUpper(publicZoneOne)}, ","),
			expectedPrivateZones: nil,
			expectedPublicZones:  map[string]struct{}{strings.ToLower(publicZoneOne): {}},
		},
		{
			name:          "bad-provider",
			zonesString:   strings.Join(util.Keys(publicZones), ",") + ",/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.FakeRP/privatednszones/test-one.com",
			expectedError: errors.New("invalid resource provider Microsoft.FakeRP from zone /subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.FakeRP/privatednszones/test-one.com: resource ID must be a public or private DNS Zone resource ID from provider Microsoft.Network"),
		},
		{
			name:          "bad-resource-type",
			zonesString:   strings.Join(util.Keys(publicZones), ",") + ",/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/hybriddnszones/test-one.com",
			expectedError: errors.New("while parsing dns zone resource ID /subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/hybriddnszones/test-one.com: detected invalid resource type hybriddnszones"),
		},
		{
			name:          "bad-resource-group",
			zonesString:   strings.Join(append(util.Keys(privateZones), util.Keys(publicZones)...), ",") + ",/subscriptions/test-private-subscription/resourceGroups/another-rg-private/providers/Microsoft.Network/privatednszones/test-two.com",
			expectedError: errors.New("while parsing resource IDs for privatednszones: detected multiple resource groups another-rg-private and test-rg-private"),
		},
		{
			name:                 "ok-resource-group",
			zonesString:          strings.Join(append(util.Keys(privateZones), util.Keys(publicZones)...), ",") + ",/subscriptions/test-private-subscription/resourceGroups/TEST-RG-PRIVATE/providers/Microsoft.Network/privatednszones/test-two.com",
			expectedPublicZones:  publicZones,
			expectedPrivateZones: util.MergeMaps(privateZones, map[string]struct{}{"/subscriptions/test-private-subscription/resourcegroups/test-rg-private/providers/microsoft.network/privatednszones/test-two.com": {}}),
		},
		{
			name:          "bad-subscription",
			zonesString:   strings.Join(append(util.Keys(privateZones), util.Keys(publicZones)...), ",") + ",/subscriptions/another-public-subscription/resourceGroups/test-rg-public/providers/Microsoft.Network/dnszones/test-two.com",
			expectedError: errors.New("while parsing resource IDs for dnszones: detected multiple subscriptions another-public-subscription and test-public-subscription"),
		},
	}
)

func TestConfigParse(t *testing.T) {
	for _, tc := range parseTestCases {
		t.Run(tc.name, func(t *testing.T) {
			conf := &Config{}
			err := conf.ParseAndValidateZoneIDs(tc.zonesString)
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedPrivateZones, conf.PrivateZoneConfig.ZoneIds)
				require.Equal(t, tc.expectedPublicZones, conf.PublicZoneConfig.ZoneIds)
			}
		})
	}
}
