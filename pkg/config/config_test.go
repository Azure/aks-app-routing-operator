// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			CrdPath:                  validCrdPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			ClusterUid:               "test-cluster-uid",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			CrdPath:                  validCrdPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
	},
	{
		Name: "valid-with-init-container-true",
		Conf: &Config{
			IsInitContainer:        true,
			OperatorNs:             "kube-system",
			OperatorWebhookService: "app-routing-operator-webhook",
			CertDir:                "/certs",
			CertName:               "test-cert-name",
			KeyName:                "test-key-name",
			CaName:                 "test-ca-name",
			CertSecretName:         "test-cert-secret-name",
		},
	},
	{
		Name: "missing webhook port",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			CertDir:                  "/certs",
			CrdPath:                  validCrdPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: "--webhook-port is required",
	},
	{
		Name: "missing operator ns",
		Conf: &Config{
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
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			CrdPath:                  validCrdPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: "--operator-namespace is required",
	},
	{
		Name: "missing operator deployment",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorNs:               "kube-system",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			CrdPath:                  validCrdPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: "--operator-deployment is required",
	},
	{
		Name: "missing operator webhook service",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			CrdPath:                  validCrdPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: "--operator-webhook-service is required",
	},
	{
		Name: "missing cert dir",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			WebhookPort:              9443,
			OperatorWebhookService:   "app-routing-operator-webhook",
			CrdPath:                  validCrdPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: "--cert-dir is required",
	},
	{
		Name: "nonexistent crd path",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			CrdPath:                  notAValidPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: fmt.Sprintf("crd path %s does not exist", notAValidPath),
	},
	{
		Name: "non-directory crd path",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "test-cluster-uid",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			CrdPath:                  notADirectoryPath,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: fmt.Sprintf("crd path %s is not a directory", notADirectoryPath),
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ClusterUid:               "test-cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ClusterUid:               "test-cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ClusterUid:               "test-cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ClusterUid:               "test-cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ClusterUid:               "test-cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ClusterUid:               "test-cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ClusterUid:               "test-cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
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
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ConcurrencyWatchdogThres: 101,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: "--concurrency-watchdog-votes must be a positive number",
	},
	{
		Name: "missing-cluster-uid",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error: "--cluster-uid is required",
	},
	{
		Name: "invalid-dns-zone-id",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
			CaName:                   "test-ca-name",
			CertSecretName:           "test-cert-secret-name",
		},
		Error:   "while parsing dns zone resource ID invalid: parsing failed for invalid. Invalid resource Id format",
		DnsZone: "invalid,dns,zone",
	},
	{
		Name: "missing-cert-name",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
		},
		Error: "--cert-name is required",
	},
	{
		Name: "missing-key-name",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			CertName:                 "test-cert-name",
		},
		Error: "--key-name is required",
	},
	{
		Name: "missing-ca-name",
		Conf: &Config{
			NS:                       "test-namespace",
			Registry:                 "test-registry",
			MSIClientID:              "test-msi-client-id",
			TenantID:                 "test-tenant-id",
			Cloud:                    "test-cloud",
			Location:                 "test-location",
			OperatorNs:               "kube-system",
			OperatorDeployment:       "app-routing-operator",
			OperatorWebhookService:   "app-routing-operator-webhook",
			WebhookPort:              9443,
			CertDir:                  "/certs",
			ConcurrencyWatchdogThres: 101,
			ConcurrencyWatchdogVotes: 2,
			ClusterUid:               "cluster-uid",
			CertName:                 "test-cert-name",
			KeyName:                  "test-key-name",
		},
		Error: "--ca-name is required",
	},
	{
		Name: "missing-cert-secret-name-with-init-container-true",
		Conf: &Config{
			IsInitContainer:        true,
			OperatorNs:             "kube-system",
			OperatorWebhookService: "app-routing-operator-webhook",
			CertDir:                "/certs",
			CertName:               "test-cert-name",
			KeyName:                "test-key-name",
			CaName:                 "test-ca-name",
		},
		Error: "--cert-secret is required",
	},
}

func TestConfigValidate(t *testing.T) {
	for _, tc := range validateTestCases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.DnsZone != "" {
				dnsZonesString = tc.DnsZone
			}
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
