// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"path"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ingConfig = &NginxIngressConfig{
		ControllerClass: "webapprouting.kubernetes.azure.com/nginx",
		ResourceName:    "nginx",
		IcName:          "webapprouting.kubernetes.azure.com",
	}
	controllerTestCases = []struct {
		Name      string
		Conf      *config.Config
		Deploy    *appsv1.Deployment
		IngConfig *NginxIngressConfig
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
			IngConfig: ingConfig,
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
			IngConfig: ingConfig,
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
			IngConfig: ingConfig,
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
			IngConfig: ingConfig,
		},
		{
			Name: "internal",
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
			IngConfig: &NginxIngressConfig{
				ControllerClass: "test-controller-class",
				ResourceName:    "nginx",
				IcName:          "nginx-private",
				ServiceConfig: &ServiceConfig{
					IsInternal: true,
					Hostname:   "test.hostname.com",
				},
			},
		},
	}
	classTestCases = []struct {
		Name      string
		Conf      *config.Config
		Deploy    *appsv1.Deployment
		IngConfig *NginxIngressConfig
	}{
		{
			Name: "full",
			Conf: &config.Config{NS: "test-namespace"},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			IngConfig: ingConfig,
		},
		{
			Name:      "no-ownership",
			Conf:      &config.Config{NS: "test-namespace"},
			IngConfig: ingConfig,
		},
	}
)

func TestIngressControllerResources(t *testing.T) {
	for _, tc := range controllerTestCases {
		objs := NginxIngressControllerResources(tc.Conf, tc.Deploy, tc.IngConfig)
		fixture := path.Join("fixtures", "nginx", tc.Name) + ".json"
		AssertFixture(t, fixture, objs)
	}
}

func TestIngressClassResources(t *testing.T) {
	for _, tc := range classTestCases {
		objs := NginxIngressClass(tc.Conf, tc.Deploy, tc.IngConfig)
		fixture := path.Join("fixtures", "nginx", tc.Name) + "-ingressclass.json"
		AssertFixture(t, fixture, objs)
	}
}
