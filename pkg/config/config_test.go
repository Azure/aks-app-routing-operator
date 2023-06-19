// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var validateTestCases = []struct {
	Name  string
	Conf  *Config
	Error string
}{
	{
		Name: "valid-minimal",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "ccp-id",
		},
	},
	{
		Name: "valid-full",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
	},
	{
		Name: "missing-namespace",
		Conf: &Config{
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
		Error: "--namespace is required",
	},
	{
		Name: "missing-registry",
		Conf: &Config{
			NS:                       "test-namespace",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
		Error: "--registry is required",
	},
	{
		Name: "missing-msi",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
		Error: "--msi is required",
	},
	{
		Name: "missing-tenant-id",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
		Error: "--tenant-id is required",
	},
	{
		Name: "missing-cloud",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
		Error: "--cloud is required",
	},
	{
		Name: "missing-location",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
		Error: "--location is required",
	},
	{
		Name: "low-concurrency-watchdog-thres",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 100,
			ConcurrencyWatchdogVotes: 2,
			CCPId:                    "test-ccp-id",
		},
		Error: "--concurrency-watchdog-threshold must be greater than 100",
	},
	{
		Name: "missing-concurrency-watchdog-thres",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
		},
		Error: "--concurrency-watchdog-votes must be a positive number",
	},
	{
		Name: "missing-cluster-ccp-id",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
		},
		Error: "--ccp-id is required",
	},
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range validateTestCases {
		t.Run(tc.Name, func(t *testing.T) {
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
	privateZones   = []string{privateZoneOne, privateZoneTwo}

	publicZoneOne = "/subscriptions/test-public-subscription/resourceGroups/test-rg-public/providers/Microsoft.Network/dnszones/test-one.com"
	publicZoneTwo = "/subscriptions/test-public-subscription/resourceGroups/test-rg-public/providers/Microsoft.Network/dnszones/test-two.com"
	publicZones   = []string{publicZoneOne, publicZoneTwo}

	parseTestCases = []struct {
		name                 string
		zonesString          string
		expectedError        error
		expectedPrivateZones []string
		expectedPublicZones  []string
	}{
		{
			name:                 "full",
			zonesString:          strings.Join(append(privateZones, publicZones...), ","),
			expectedPrivateZones: privateZones,
			expectedPublicZones:  publicZones,
		},
		{
			name:                 "private-only",
			zonesString:          strings.Join(privateZones, ","),
			expectedPrivateZones: privateZones,
			expectedPublicZones:  nil,
		},
		{
			name:                 "public-only",
			zonesString:          strings.Join(publicZones, ","),
			expectedPrivateZones: nil,
			expectedPublicZones:  publicZones,
		},
		{
			name:          "bad-provider",
			zonesString:   strings.Join(publicZones, ",") + ",/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.FakeRP/privatednszones/test-one.com",
			expectedError: errors.New("invalid resource provider Microsoft.FakeRP from zone /subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.FakeRP/privatednszones/test-one.com: resource ID must be a public or private DNS Zone resource ID from provider Microsoft.Network"),
		},
		{
			name:          "bad-resource-type",
			zonesString:   strings.Join(publicZones, ",") + ",/subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/hybriddnszones/test-one.com",
			expectedError: errors.New("while parsing dns zone resource ID /subscriptions/test-private-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/hybriddnszones/test-one.com: detected invalid resource type hybriddnszones"),
		},
		{
			name:          "bad-resource-group",
			zonesString:   strings.Join(append(privateZones, publicZones...), ",") + ",/subscriptions/test-private-subscription/resourceGroups/another-rg-private/providers/Microsoft.Network/privatednszones/test-two.com",
			expectedError: errors.New("while parsing resource IDs for privatednszones: detected multiple resource groups another-rg-private and test-rg-private"),
		},
		{
			name:          "bad-subscription",
			zonesString:   strings.Join(append(privateZones, publicZones...), ",") + ",/subscriptions/another-public-subscription/resourceGroups/test-rg-public/providers/Microsoft.Network/dnszones/test-two.com",
			expectedError: errors.New("while parsing resource IDs for dnszones: detected multiple subscriptions another-public-subscription and test-public-subscription"),
		},
	}
)

func TestConfigParse(t *testing.T) {
	for _, tc := range parseTestCases {
		conf := &Config{}
		err := conf.ParseAndValidateZoneIDs(tc.zonesString)
		if tc.expectedError != nil {
			require.EqualError(t, err, tc.expectedError.Error())
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.expectedPrivateZones, conf.PrivateZoneConfig.ZoneIds)
			require.Equal(t, tc.expectedPublicZones, conf.PublicZoneConfig.ZoneIds)
		}
	}

}
