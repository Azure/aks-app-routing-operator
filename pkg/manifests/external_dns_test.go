package manifests

import (
	"path"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	dnsConfig = &ExternalDnsConfig{
		ResourceName:  "external-dns",
		TenantId:      "test-tenant-id",
		Subscription:  "test-subscription",
		ResourceGroup: "test-rg",
		Domain:        "test-domain",
		RecordId:      "test-record-id",
		IsPrivate:     false,
	}

	testCases = []struct {
		Name      string
		Conf      *config.Config
		Deploy    *appsv1.Deployment
		DnsConfig *ExternalDnsConfig
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
			DnsConfig: dnsConfig,
		},
		{
			Name:      "no-ownership",
			Conf:      &config.Config{NS: "test-namespace"},
			DnsConfig: dnsConfig,
		},
		{
			Name: "private",
			Conf: &config.Config{NS: "test-namespace"},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfig: &ExternalDnsConfig{
				ResourceName:  "private-dns",
				TenantId:      "test-tenant",
				Subscription:  "test-subscription",
				ResourceGroup: "test-rg",
				Domain:        "test.domain.com",
				RecordId:      "test-record-id",
				IsPrivate:     true,
			},
		},
	}
)

func TestExternalDnsResources(t *testing.T) {
	for _, tc := range testCases {
		objs := ExternalDnsResources(tc.Conf, tc.Deploy, tc.DnsConfig)
		fixture := path.Join("fixtures", "external_dns", tc.Name) + ".json"
		AssertFixture(t, fixture, objs)
	}
}
