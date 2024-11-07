package manifests

import (
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	publicZoneOne = strings.ToLower("/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/dnszones/test-one.com")
	publicZoneTwo = strings.ToLower("/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/dnszones/test-two.com")
	publicZones   = []string{publicZoneOne, publicZoneTwo}

	privateZoneOne = strings.ToLower("/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-three.com")
	privateZoneTwo = strings.ToLower("/subscriptions/test-subscription/resourceGroups/test-rg-private/providers/Microsoft.Network/privatednszones/test-four.com")
	privateZones   = []string{privateZoneOne, privateZoneTwo}

	clusterUid = "test-cluster-uid"

	publicDnsConfig = &externalDnsConfig{
		tenantId:           "test-tenant-id",
		subscription:       "test-subscription-id",
		resourceGroup:      "test-resource-group-public",
		namespace:          "test-namespace",
		clientId:           "test-client-id",
		identityType:       IdentityTypeMSI,
		resourceTypes:      []ResourceType{ResourceTypeIngress},
		dnsZoneResourceIDs: publicZones,
		provider:           PublicProvider,
	}
	privateDnsConfig = &externalDnsConfig{
		tenantId:           "test-tenant-id",
		subscription:       "test-subscription-id",
		resourceGroup:      "test-resource-group-private",
		namespace:          "test-namespace",
		clientId:           "test-client-id",
		identityType:       IdentityTypeMSI,
		resourceTypes:      []ResourceType{ResourceTypeIngress},
		dnsZoneResourceIDs: privateZones,
		provider:           PrivateProvider,
	}

	publicGwConfig = &externalDnsConfig{
		tenantId:           "test-tenant-id",
		subscription:       "test-subscription-id",
		resourceGroup:      "test-resource-group-public",
		namespace:          "test-namespace",
		clientId:           "test-client-id",
		identityType:       IdentityTypeWorkloadIdentity,
		resourceTypes:      []ResourceType{ResourceTypeGateway},
		dnsZoneResourceIDs: publicZones,
		provider:           PublicProvider,
		serviceAccountName: "test-service-account",
		crdName:            "test-dns-config",
	}

	privateGwConfig = &externalDnsConfig{
		tenantId:           "test-tenant-id",
		subscription:       "test-subscription-id",
		resourceGroup:      "test-resource-group-private",
		namespace:          "test-namespace",
		clientId:           "test-client-id",
		identityType:       IdentityTypeWorkloadIdentity,
		resourceTypes:      []ResourceType{ResourceTypeGateway},
		dnsZoneResourceIDs: privateZones,
		provider:           PrivateProvider,
		serviceAccountName: "test-private-service-account",
		crdName:            "test-dns-config-private",
	}

	privateGwIngressConfig = &externalDnsConfig{
		tenantId:           "test-tenant-id",
		subscription:       "test-subscription-id",
		resourceGroup:      "test-resource-group-private",
		namespace:          "test-namespace",
		clientId:           "test-client-id",
		identityType:       IdentityTypeWorkloadIdentity,
		resourceTypes:      []ResourceType{ResourceTypeGateway, ResourceTypeIngress},
		dnsZoneResourceIDs: privateZones,
		provider:           PrivateProvider,
		serviceAccountName: "test-private-service-account",
		crdName:            "test-dns-config-private",
	}

	testCases = []struct {
		Name       string
		Conf       *config.Config
		Deploy     *appsv1.Deployment
		DnsConfigs []*externalDnsConfig
	}{
		{
			Name: "full",
			Conf: &config.Config{ClusterUid: clusterUid, DnsSyncInterval: time.Minute * 3},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfigs: []*externalDnsConfig{publicDnsConfig, privateDnsConfig},
		},
		{
			Name:       "no-ownership",
			Conf:       &config.Config{NS: "test-namespace", ClusterUid: clusterUid, DnsSyncInterval: time.Minute * 3},
			DnsConfigs: []*externalDnsConfig{publicDnsConfig},
		},
		{
			Name: "private",
			Conf: &config.Config{NS: "test-namespace", ClusterUid: clusterUid, DnsSyncInterval: time.Minute * 3},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfigs: []*externalDnsConfig{privateDnsConfig},
		},
		{
			Name: "short-sync-interval",
			Conf: &config.Config{NS: "test-namespace", ClusterUid: clusterUid, DnsSyncInterval: time.Second * 10},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfigs: []*externalDnsConfig{publicDnsConfig, privateDnsConfig},
		},
		{
			Name: "all-possibilities",
			Conf: &config.Config{NS: "test-namespace", ClusterUid: clusterUid, DnsSyncInterval: time.Second * 10},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfigs: []*externalDnsConfig{publicDnsConfig, privateDnsConfig, publicGwConfig, privateGwConfig},
		},
		{
			Name: "private-gateway",
			Conf: &config.Config{NS: "test-namespace", ClusterUid: clusterUid, DnsSyncInterval: time.Second * 10},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfigs: []*externalDnsConfig{privateGwConfig},
		},
		{
			Name: "private-ingress-gateway",
			Conf: &config.Config{NS: "test-namespace", ClusterUid: clusterUid, DnsSyncInterval: time.Second * 10},
			Deploy: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-operator-deploy",
					UID:  "test-operator-deploy-uid",
				},
			},
			DnsConfigs: []*externalDnsConfig{privateGwIngressConfig},
		},
	}
)

func TestExternalDnsResources(t *testing.T) {
	for _, tc := range testCases {

		objs := externalDnsResources(tc.Conf, tc.DnsConfigs)

		fixture := path.Join("fixtures", "external_dns", tc.Name) + ".yaml"
		AssertFixture(t, fixture, objs)
		GatekeeperTest(t, fixture)
	}
}

func TestExternalDNSConfig(t *testing.T) {
	conf := &config.Config{
		ClusterUid:      clusterUid,
		DnsSyncInterval: time.Minute * 3,
		Cloud:           "test-cloud",
	}

	noOsmConf := &config.Config{
		ClusterUid:      clusterUid,
		DnsSyncInterval: time.Minute * 3,
		DisableOSM:      true,
		Cloud:           "test-cloud",
	}

	testCases := []struct {
		name               string
		conf               *config.Config
		tenantId           string
		subscription       string
		resourceGroup      string
		msiclientID        string
		serviceAccountName string
		namespace          string
		crdName            string
		identityType       IdentityType
		resourceTypes      []ResourceType
		provider           provider
		dnsZoneResourceIDs []string
		expectedObjects    []client.Object
		expectedLabels     map[string]string
	}{
		{
			name:               "public ingress no osm",
			conf:               noOsmConf,
			tenantId:           "test-tenant-id",
			subscription:       "test-subscription-id",
			resourceGroup:      "test-resource-group-public",
			msiclientID:        "test-client-id",
			serviceAccountName: "test-sa",
			namespace:          "test-namespace",
			crdName:            "",
			identityType:       IdentityTypeMSI,
			resourceTypes:      []ResourceType{ResourceTypeIngress},
			provider:           PublicProvider,
			dnsZoneResourceIDs: []string{publicZoneOne, publicZoneTwo},
			expectedLabels:     map[string]string{"app.kubernetes.io/name": "external-dns"},
			expectedObjects:    externalDnsResources(noOsmConf, []*externalDnsConfig{publicDnsConfig}),
		},
		{
			name:               "private ingress",
			conf:               conf,
			tenantId:           "test-tenant-id",
			subscription:       "test-subscription-id",
			resourceGroup:      "test-resource-group-private",
			msiclientID:        "test-client-id",
			serviceAccountName: "test-sa",
			namespace:          "test-namespace",
			crdName:            "",
			identityType:       IdentityTypeMSI,
			resourceTypes:      []ResourceType{ResourceTypeIngress},
			provider:           PrivateProvider,
			dnsZoneResourceIDs: []string{privateZoneOne, privateZoneTwo},
			expectedLabels:     map[string]string{"app.kubernetes.io/name": "external-dns-private"},
			expectedObjects:    externalDnsResources(conf, []*externalDnsConfig{privateDnsConfig}),
		},
		{
			name:               "public gateway",
			conf:               conf,
			tenantId:           "test-tenant-id",
			subscription:       "test-subscription-id",
			resourceGroup:      "test-resource-group-public",
			msiclientID:        "test-client-id",
			serviceAccountName: "test-service-account",
			namespace:          "test-namespace",
			crdName:            "test-dns-config",
			identityType:       IdentityTypeWorkloadIdentity,
			resourceTypes:      []ResourceType{ResourceTypeGateway},
			provider:           PublicProvider,
			dnsZoneResourceIDs: []string{publicZoneOne, publicZoneTwo},
			expectedLabels:     map[string]string{"app.kubernetes.io/name": "test-dns-config-external-dns"},
			expectedObjects:    externalDnsResources(conf, []*externalDnsConfig{publicGwConfig}),
		},
		{
			name:               "private gateway no osm",
			conf:               noOsmConf,
			tenantId:           "test-tenant-id",
			subscription:       "test-subscription-id",
			resourceGroup:      "test-resource-group-private",
			msiclientID:        "test-client-id",
			serviceAccountName: "test-private-service-account",
			namespace:          "test-namespace",
			crdName:            "test-dns-config-private",
			identityType:       IdentityTypeWorkloadIdentity,
			resourceTypes:      []ResourceType{ResourceTypeGateway},
			provider:           PrivateProvider,
			dnsZoneResourceIDs: []string{privateZoneOne, privateZoneTwo},
			expectedLabels:     map[string]string{"app.kubernetes.io/name": "test-dns-config-private-external-dns"},
			expectedObjects:    externalDnsResources(noOsmConf, []*externalDnsConfig{privateGwConfig}),
		},
	}
	for _, tc := range testCases {
		ret := NewExternalDNSConfig(tc.conf, tc.tenantId, tc.subscription, tc.resourceGroup, tc.msiclientID, tc.serviceAccountName, tc.namespace, tc.crdName, tc.identityType, tc.resourceTypes, tc.provider, tc.dnsZoneResourceIDs)
		actualObjs := ret.Resources()
		actualLabels := ret.Labels()
		require.Equal(t, tc.expectedObjects, actualObjs, "objects do not match for case %s", tc.name)
		require.Equal(t, tc.expectedLabels, actualLabels, "labels do not match for case %s", tc.name)

	}
}
