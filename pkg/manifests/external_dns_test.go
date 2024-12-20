package manifests

import (
	"errors"
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

	publicDnsConfig = &ExternalDnsConfig{
		resourceName:       "external-dns",
		tenantId:           "test-tenant-id",
		subscription:       "test-subscription-id",
		resourceGroup:      "test-resource-group-public",
		namespace:          "test-namespace",
		clientId:           "test-client-id",
		identityType:       IdentityTypeMSI,
		resourceTypes:      []ResourceType{ResourceTypeIngress},
		dnsZoneResourceIDs: publicZones,
		provider:           PublicProvider,
		serviceAccountName: "external-dns",
	}
	privateDnsConfig = &ExternalDnsConfig{
		resourceName:       "external-dns-private",
		serviceAccountName: "external-dns-private",
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

	publicGwConfig = &ExternalDnsConfig{
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
		resourceName:       "test-dns-config-external-dns",
	}

	privateGwConfig = &ExternalDnsConfig{
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
		resourceName:       "test-dns-config-private-external-dns",
	}

	privateGwIngressConfig = &ExternalDnsConfig{
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
		resourceName:       "test-dns-config-private-external-dns",
	}

	testCases = []struct {
		Name       string
		Conf       *config.Config
		Deploy     *appsv1.Deployment
		DnsConfigs []*ExternalDnsConfig
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
			DnsConfigs: []*ExternalDnsConfig{publicDnsConfig, privateDnsConfig},
		},
		{
			Name:       "no-ownership",
			Conf:       &config.Config{NS: "test-namespace", ClusterUid: clusterUid, DnsSyncInterval: time.Minute * 3},
			DnsConfigs: []*ExternalDnsConfig{publicDnsConfig},
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
			DnsConfigs: []*ExternalDnsConfig{privateDnsConfig},
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
			DnsConfigs: []*ExternalDnsConfig{publicDnsConfig, privateDnsConfig},
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
			DnsConfigs: []*ExternalDnsConfig{publicDnsConfig, privateDnsConfig, publicGwConfig, privateGwConfig},
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
			DnsConfigs: []*ExternalDnsConfig{privateGwConfig},
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
			DnsConfigs: []*ExternalDnsConfig{privateGwIngressConfig},
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
		name                   string
		conf                   *config.Config
		inputExternalDNSConfig InputExternalDNSConfig
		expectedObjects        []client.Object
		expectedLabels         map[string]string
		expectedError          error
	}{
		{
			name: "public ingress no osm",
			conf: noOsmConf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				TenantId:            "test-tenant-id",
				Subscription:        "test-subscription-id",
				ResourceGroup:       "test-resource-group-public",
				ClientId:            "test-client-id",
				InputServiceAccount: "test-sa",
				Namespace:           "test-namespace",
				InputResourceName:   "",
				IdentityType:        IdentityTypeMSI,
				ResourceTypes:       []ResourceType{ResourceTypeIngress},
				Provider:            PublicProvider,
				DnsZoneresourceIDs:  []string{publicZoneOne, publicZoneTwo},
			},

			expectedLabels:  map[string]string{"app.kubernetes.io/name": "external-dns"},
			expectedObjects: externalDnsResources(noOsmConf, []*ExternalDnsConfig{publicDnsConfig}),
		},
		{
			name: "private ingress",
			conf: conf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				TenantId:            "test-tenant-id",
				Subscription:        "test-subscription-id",
				ResourceGroup:       "test-resource-group-private",
				ClientId:            "test-client-id",
				InputServiceAccount: "test-sa",
				Namespace:           "test-namespace",
				InputResourceName:   "",
				IdentityType:        IdentityTypeMSI,
				ResourceTypes:       []ResourceType{ResourceTypeIngress},
				Provider:            PrivateProvider,
				DnsZoneresourceIDs:  []string{privateZoneOne, privateZoneTwo},
			},

			expectedLabels:  map[string]string{"app.kubernetes.io/name": "external-dns-private"},
			expectedObjects: externalDnsResources(conf, []*ExternalDnsConfig{privateDnsConfig}),
		},
		{
			name: "public gateway",
			conf: conf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				TenantId:            "test-tenant-id",
				Subscription:        "test-subscription-id",
				ResourceGroup:       "test-resource-group-public",
				ClientId:            "test-client-id",
				InputServiceAccount: "test-service-account",
				Namespace:           "test-namespace",
				InputResourceName:   "test-dns-config",
				IdentityType:        IdentityTypeWorkloadIdentity,
				ResourceTypes:       []ResourceType{ResourceTypeGateway},
				Provider:            PublicProvider,
				DnsZoneresourceIDs:  []string{publicZoneOne, publicZoneTwo},
			},

			expectedLabels:  map[string]string{"app.kubernetes.io/name": "test-dns-config-external-dns"},
			expectedObjects: externalDnsResources(conf, []*ExternalDnsConfig{publicGwConfig}),
		},
		{
			name: "private gateway no osm",
			conf: noOsmConf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				TenantId:            "test-tenant-id",
				Subscription:        "test-subscription-id",
				ResourceGroup:       "test-resource-group-private",
				ClientId:            "test-client-id",
				InputServiceAccount: "test-private-service-account",
				Namespace:           "test-namespace",
				InputResourceName:   "test-dns-config-private",
				IdentityType:        IdentityTypeWorkloadIdentity,
				ResourceTypes:       []ResourceType{ResourceTypeGateway},
				Provider:            PrivateProvider,
				DnsZoneresourceIDs:  []string{privateZoneOne, privateZoneTwo},
			},

			expectedLabels:  map[string]string{"app.kubernetes.io/name": "test-dns-config-private-external-dns"},
			expectedObjects: externalDnsResources(noOsmConf, []*ExternalDnsConfig{privateGwConfig}),
		},
		{
			name: "invalid identity type",
			conf: conf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				IdentityType: 3,
			},
			expectedError: errors.New("invalid identity type: 3"),
		},
		{
			name: "invalid resource type",
			conf: conf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				ResourceTypes: []ResourceType{ResourceTypeGateway, ResourceTypeIngress, 4},
			},
			expectedError: errors.New("invalid resource type: 4"),
		},
		{
			name: "gateway without workload identity",
			conf: noOsmConf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				TenantId:            "test-tenant-id",
				Subscription:        "test-subscription-id",
				ResourceGroup:       "test-resource-group-private",
				ClientId:            "test-client-id",
				InputServiceAccount: "test-private-service-account",
				Namespace:           "test-namespace",
				InputResourceName:   "test-resource",
				IdentityType:        IdentityTypeMSI,
				ResourceTypes:       []ResourceType{ResourceTypeGateway},
				Provider:            PrivateProvider,
				DnsZoneresourceIDs:  []string{privateZoneOne, privateZoneTwo},
			},
			expectedError: errors.New("gateway resource type can only be used with workload identity"),
		},
		{
			name: "workload identity without provided serviceaccount",
			conf: noOsmConf,
			inputExternalDNSConfig: InputExternalDNSConfig{
				TenantId:            "test-tenant-id",
				Subscription:        "test-subscription-id",
				ResourceGroup:       "test-resource-group-private",
				ClientId:            "test-client-id",
				InputServiceAccount: "",
				Namespace:           "test-namespace",
				InputResourceName:   "test-resource",
				IdentityType:        IdentityTypeWorkloadIdentity,
				ResourceTypes:       []ResourceType{ResourceTypeIngress},
				Provider:            PrivateProvider,
				DnsZoneresourceIDs:  []string{privateZoneOne, privateZoneTwo},
			},
			expectedError: errors.New("workload identity requires a service account name"),
		},
	}
	for _, tc := range testCases {
		ret, err := NewExternalDNSConfig(tc.conf, tc.inputExternalDNSConfig)
		if tc.expectedError != nil {
			require.Equal(t, tc.expectedError.Error(), err.Error(), "error does not match for case %s", tc.name)
		} else {
			require.NoError(t, err, "unexpected error for case %s", tc.name)
			actualObjs := ret.Resources()
			actualLabels := ret.Labels()
			require.Equal(t, tc.expectedObjects, actualObjs, "objects do not match for case %s", tc.name)
			require.Equal(t, tc.expectedLabels, actualLabels, "labels do not match for case %s", tc.name)
		}
	}
}
