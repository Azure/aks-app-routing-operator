// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	icName                   = "webapprouting.kubernetes.azure.com"
	controllerClass          = "webapprouting.kubernetes.azure.com/nginx"
	controllerName           = "nginx"
	differentControllerClass = "different-controller-class.com"
)

func ic(name, controller string) *netv1.IngressClass {
	return &netv1.IngressClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressClass",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: netv1.IngressClassSpec{
			Controller: controller,
		},
	}
}

var (
	integrationTestCases = []struct {
		Name            string
		Conf            *config.Config
		ControllerClass string
		ControllerName  string
		Deploy          *appsv1.Deployment
		Ic              *netv1.IngressClass
	}{
		{
			Name:            "full",
			ControllerClass: controllerClass,
			ControllerName:  controllerName,
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
			Ic: ic(icName, controllerClass),
		},
		{
			Name:            "no-ownership",
			ControllerName:  controllerName,
			ControllerClass: controllerClass,
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
			Ic: ic(icName, controllerClass),
		},
		{
			Name:            "kube-system",
			ControllerClass: controllerClass,
			ControllerName:  controllerName,
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
			Ic: ic(icName, controllerClass),
		},
		{
			Name:            "optional-features-disabled",
			ControllerName:  controllerName,
			ControllerClass: controllerClass,
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
			Ic: ic(icName, controllerClass),
		},
		{
			Name:            "controller-class",
			ControllerName:  controllerName,
			ControllerClass: differentControllerClass,
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
			Ic: ic(icName, differentControllerClass),
		},
		{
			Name:            "ingress-name",
			ControllerName:  controllerName,
			ControllerClass: controllerClass,
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
			Ic: ic("other.ingress.class.com", controllerClass),
		},
	}
)

func TestIngressControllerResources(t *testing.T) {
	for _, tc := range integrationTestCases {
		podLabels := map[string]string{"app": tc.ControllerName}
		objs := NginxIngressControllerResources(tc.Conf, tc.Deploy, tc.Ic, tc.ControllerClass, tc.ControllerName, podLabels)

		actual, err := json.MarshalIndent(&objs, "  ", "  ")
		require.NoError(t, err)

		fixture := path.Join("fixtures", "nginx", tc.Name) + ".json"
		if os.Getenv("GENERATE_FIXTURES") != "" {
			err = os.WriteFile(fixture, actual, 0644)
			require.NoError(t, err)
			continue
		}

		expected, err := os.ReadFile(fixture)
		require.NoError(t, err)

		assert.JSONEq(t, string(expected), string(actual))
	}
}
