package manifests

import (
	"path"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	publicZoneOne = "/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/dnszones/test-one.com"
	publicZoneTwo = "/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/dnszones/test-two.com"
	publicZones   = []string{publicZoneOne, publicZoneTwo}

	privateZoneOne = "/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-one.com"
	privateZoneTwo = "/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-two.com"
	privateZones   = []string{privateZoneOne, privateZoneTwo}

	publicDnsConfig = &ExternalDnsConfig{
		ResourceName:       "external-dns-public",
		TenantId:           "test-tenant-id",
		Subscription:       "test-subscription-id",
		ResourceGroup:      "test-resource-group-public",
		DnsZoneResourceIDs: publicZones,
		Provider:           Provider,
	}

	privateDnsConfig = &ExternalDnsConfig{
		ResourceName:       "external-dns-private",
		TenantId:           "test-tenant-id",
		Subscription:       "test-subscription-id",
		ResourceGroup:      "test-resource-group-private",
		DnsZoneResourceIDs: privateZones,
		Provider:           PrivateProvider,
	}

	testCases = []struct {
		Name       string
		Conf       *config.Config
		Deploy     *appsv1.Deployment
		DnsConfigs []*ExternalDnsConfig
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
			DnsConfigs: []*ExternalDnsConfig{publicDnsConfig, privateDnsConfig},
		},
		{
			Name:       "no-ownership",
			Conf:       &config.Config{NS: "test-namespace"},
			DnsConfigs: []*ExternalDnsConfig{publicDnsConfig},
		},
		{
			Name: "private-only",
			Conf: &config.Config{NS: "test-namespace"},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfigs: []*ExternalDnsConfig{privateDnsConfig},
		},
	}
)

func TestExternalDnsResources(t *testing.T) {
	for _, tc := range testCases {
		objs := []client.Object{}
		for _, dnsConfig := range tc.DnsConfigs {
			objs = append(objs, ExternalDnsResources(tc.Conf, tc.Deploy, dnsConfig)...)
		}

		fixture := path.Join("fixtures", "external_dns", tc.Name) + ".json"
		AssertFixture(t, fixture, objs)
	}
}
