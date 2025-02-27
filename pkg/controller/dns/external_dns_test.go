package dns

import (
	"os"
	"reflect"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	env        *envtest.Environment
	restConfig *rest.Config
	err        error
	uid        = uuid.New().String()
	noZones    = config.Config{
		ClusterUid:        uid,
		MSIClientID:       "client-id",
		NS:                "test-ns",
		PrivateZoneConfig: config.DnsZoneConfig{},
		PublicZoneConfig:  config.DnsZoneConfig{},
	}
	onlyPubZones = config.Config{
		ClusterUid:        uid,
		MSIClientID:       "client-id",
		NS:                "test-ns",
		PrivateZoneConfig: config.DnsZoneConfig{},
		PublicZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       map[string]struct{}{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/dnszones/test.com": {}},
		},
	}
	onlyPrivZones = config.Config{
		ClusterUid:       uid,
		MSIClientID:      "client-id",
		NS:               "test-ns",
		PublicZoneConfig: config.DnsZoneConfig{},
		PrivateZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       map[string]struct{}{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/privatednszones/test.com": {}},
		},
	}
	allZones = config.Config{
		ClusterUid:  uid,
		MSIClientID: "client-id",
		NS:          "test-ns",
		PublicZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       map[string]struct{}{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/dnszones/test.com": {}},
		},
		PrivateZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       map[string]struct{}{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/privatednszones/test.com": {}},
		},
	}
	gvr1 = schema.GroupVersionResource{
		Group:    "group",
		Version:  "v1",
		Resource: "resources",
	}
	gvk1 = schema.GroupVersionKind{
		Group:   gvr1.Group,
		Version: gvr1.Version,
		Kind:    "Resource",
	}
)

func TestMain(m *testing.M) {
	restConfig, env, err = testutils.StartTestingEnv()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testutils.CleanupTestingEnv(env)

	os.Exit(code)
}
func TestPublicConfig(t *testing.T) {
	tests := []struct {
		name             string
		conf             *config.Config
		expectedDnsZones []string
		expectedLabels   map[string]string
	}{
		{
			name:             "all zones config",
			conf:             &allZones,
			expectedDnsZones: util.Keys(allZones.PublicZoneConfig.ZoneIds),
			expectedLabels:   map[string]string{"app.kubernetes.io/name": "external-dns"},
		},
		{
			name:             "ony private config",
			conf:             &onlyPrivZones,
			expectedDnsZones: util.Keys(onlyPrivZones.PublicZoneConfig.ZoneIds),
			expectedLabels:   map[string]string{"app.kubernetes.io/name": "external-dns"},
		},
	}

	for _, test := range tests {
		got, err := publicConfigForIngress(test.conf)
		require.NoError(t, err)
		require.Equal(t, test.expectedDnsZones, got.DnsZoneResourceIds(), "zones don't match for %s", test.name)
		require.Equal(t, test.expectedLabels, got.Labels(), "labels don't match for %s", test.name)
	}
}

func TestPrivateConfig(t *testing.T) {
	tests := []struct {
		name             string
		conf             *config.Config
		expectedDnsZones []string
		expectedLabels   map[string]string
	}{
		{
			name:             "all zones config",
			conf:             &allZones,
			expectedDnsZones: util.Keys(allZones.PrivateZoneConfig.ZoneIds),
			expectedLabels:   map[string]string{"app.kubernetes.io/name": "external-dns-private"},
		},
		{
			name:             "ony private config",
			conf:             &onlyPrivZones,
			expectedDnsZones: util.Keys(onlyPrivZones.PrivateZoneConfig.ZoneIds),
			expectedLabels:   map[string]string{"app.kubernetes.io/name": "external-dns-private"},
		},
	}

	for _, test := range tests {
		got, err := privateConfigForIngress(test.conf)
		require.NoError(t, err)
		require.Equal(t, test.expectedDnsZones, got.DnsZoneResourceIds(), "zones don't match for %s", test.name)
		require.Equal(t, test.expectedLabels, got.Labels(), "labels don't match for %s", test.name)
	}
}

func TestInstances(t *testing.T) {
	noZonesPublic, err := manifests.NewExternalDNSConfig(&noZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PublicProvider), TenantId: noZones.TenantID, ClientId: noZones.MSIClientID, Namespace: noZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(noZones.PublicZoneConfig.ZoneIds)})
	require.NoError(t, err)

	noZonesPrivate, err := manifests.NewExternalDNSConfig(&noZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PrivateProvider), TenantId: noZones.TenantID, ClientId: noZones.MSIClientID, Namespace: noZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(noZones.PrivateZoneConfig.ZoneIds)})
	require.NoError(t, err)

	_, err = manifests.NewExternalDNSConfig(&config.Config{}, manifests.InputExternalDNSConfig{TenantId: "tenant", ClientId: "client", Namespace: "ns", IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: []string{}})
	require.Error(t, err)
	require.Equal(t, err.Error(), "provider must be specified via inputconfig if no DNS zones are provided")

	onlyPrivZonesPublic, err := manifests.NewExternalDNSConfig(&onlyPrivZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PublicProvider), TenantId: onlyPrivZones.TenantID, ClientId: onlyPrivZones.MSIClientID, Namespace: onlyPrivZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(onlyPrivZones.PublicZoneConfig.ZoneIds)})
	require.NoError(t, err)

	onlyPrivZonesPrivate, err := manifests.NewExternalDNSConfig(&onlyPrivZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PrivateProvider), TenantId: onlyPrivZones.TenantID, ClientId: onlyPrivZones.MSIClientID, Namespace: onlyPrivZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(onlyPrivZones.PrivateZoneConfig.ZoneIds)})
	require.NoError(t, err)

	publicDeployPublic, err := manifests.NewExternalDNSConfig(&onlyPubZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PublicProvider), TenantId: onlyPubZones.TenantID, ClientId: onlyPubZones.MSIClientID, Namespace: onlyPubZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(onlyPubZones.PublicZoneConfig.ZoneIds)})
	require.NoError(t, err)

	publicDeployPrivate, err := manifests.NewExternalDNSConfig(&onlyPubZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PrivateProvider), TenantId: onlyPubZones.TenantID, ClientId: onlyPubZones.MSIClientID, Namespace: onlyPubZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(onlyPubZones.PrivateZoneConfig.ZoneIds)})
	require.NoError(t, err)

	allDeployPublic, err := manifests.NewExternalDNSConfig(&allZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PublicProvider), TenantId: allZones.TenantID, ClientId: allZones.MSIClientID, Namespace: allZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(allZones.PublicZoneConfig.ZoneIds)})
	require.NoError(t, err)

	allDeployPrivate, err := manifests.NewExternalDNSConfig(&allZones, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PrivateProvider), TenantId: allZones.TenantID, ClientId: allZones.MSIClientID, Namespace: allZones.NS, IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: util.Keys(allZones.PrivateZoneConfig.ZoneIds)})
	require.NoError(t, err)

	tests := []struct {
		name     string
		conf     *config.Config
		expected []instance
	}{
		{
			name: "all clean",
			conf: &noZones,
			expected: []instance{
				{
					config:    noZonesPublic,
					resources: noZonesPublic.Resources(),
					action:    clean,
				},
				{
					config:    noZonesPrivate,
					resources: noZonesPrivate.Resources(),
					action:    clean,
				},
			},
		},
		{
			name: "private deploy",
			conf: &onlyPrivZones,
			expected: []instance{
				{
					config:    onlyPrivZonesPublic,
					resources: onlyPrivZonesPublic.Resources(),
					action:    clean,
				},
				{
					config:    onlyPrivZonesPrivate,
					resources: onlyPrivZonesPrivate.Resources(),
					action:    deploy,
				},
			},
		},
		{
			name: "public deploy",
			conf: &onlyPubZones,
			expected: []instance{
				{
					config:    publicDeployPublic,
					resources: publicDeployPublic.Resources(),
					action:    deploy,
				},
				{
					config:    publicDeployPrivate,
					resources: publicDeployPrivate.Resources(),
					action:    clean,
				},
			},
		},
		{
			name: "all deploy",
			conf: &allZones,
			expected: []instance{
				{
					config:    allDeployPublic,
					resources: allDeployPublic.Resources(),
					action:    deploy,
				},
				{
					config:    allDeployPrivate,
					resources: allDeployPrivate.Resources(),
					action:    deploy,
				},
			},
		},
	}

	for _, test := range tests {
		instances, err := instances(test.conf)
		require.NoError(t, err)
		if !reflect.DeepEqual(instances, test.expected) {
			expectedObjects := make([]client.Object, 0)
			for _, expectedInstance := range test.expected {
				expectedObjects = append(expectedObjects, expectedInstance.resources...)
			}
			actualObjects := make([]client.Object, 0)
			for _, actualInstance := range instances {
				actualObjects = append(actualObjects, actualInstance.resources...)
			}
			t.Error(
				"For", test.name,
				"expected objects", expectedObjects,
				"\n",
				"got", actualObjects,
			)
		}
	}
}

func TestFilterAction(t *testing.T) {
	allClean, err := instances(&noZones)
	require.NoError(t, err)
	allDeploy, err := instances(&allZones)
	require.NoError(t, err)
	oneDeployOneClean, err := instances(&onlyPrivZones)
	require.NoError(t, err)

	tests := []struct {
		name      string
		instances []instance
		action    action
		expected  []instance
	}{
		{
			name:      "all clean returns all",
			instances: allClean,
			action:    clean,
			expected:  allClean,
		},
		{
			name:      "all clean returns none",
			instances: allClean,
			action:    deploy,
			expected:  []instance{},
		},
		{
			name:      "all deploy returns all",
			instances: allDeploy,
			action:    deploy,
			expected:  allDeploy,
		},
		{
			name:      "all deploy returns none",
			instances: allDeploy,
			action:    clean,
			expected:  []instance{},
		},
		{
			name:      "one deploy one clean returns one deploy",
			instances: oneDeployOneClean,
			action:    deploy,
			expected:  []instance{oneDeployOneClean[1]},
		},
		{
			name:      "one deploy one clean returns one clean",
			instances: oneDeployOneClean,
			action:    clean,
			expected:  []instance{oneDeployOneClean[0]},
		},
	}

	for _, test := range tests {
		got := filterAction(test.instances, test.action)
		if !reflect.DeepEqual(got, test.expected) &&
			(len(got) != 0 && len(test.expected) != 0) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", test.expected,
			)
		}
	}

}

func TestGetResources(t *testing.T) {
	instances, err := instances(&noZones)
	require.NoError(t, err)
	got := getResources(instances)
	var expected []client.Object
	for _, instance := range instances {
		expected = append(expected, instance.resources...)
	}
	require.Equal(t, expected, got, "expected is not equal to got")
	require.True(t, len(got) != 0, "got is not empty")

	var empty []client.Object
	require.Equal(t, empty, getResources(nil), "failed to handle nil instances")
	require.Equal(t, empty, getResources([]instance{}), "failed to handle empty instances")
}

func TestGetLabels(t *testing.T) {
	onlyPrivZonesInstances, err := instances(&onlyPrivZones)
	require.NoError(t, err)

	onlyPubZonesInstances, err := instances(&onlyPubZones)
	require.NoError(t, err)

	allZonesInstances, err := instances(&allZones)
	require.NoError(t, err)

	tests := []struct {
		name      string
		instances []instance
		expected  map[string]string
	}{
		{
			name:      "always returns top level",
			instances: []instance{},
			expected:  manifests.GetTopLevelLabels(),
		},
		{
			name:      "top level and private",
			instances: filterAction(onlyPrivZonesInstances, deploy),
			expected:  util.MergeMaps(manifests.GetTopLevelLabels(), map[string]string{"app.kubernetes.io/name": "external-dns-private"}),
		},
		{
			name:      "top level and public",
			instances: filterAction(onlyPubZonesInstances, deploy),
			expected:  util.MergeMaps(manifests.GetTopLevelLabels(), map[string]string{"app.kubernetes.io/name": "external-dns"}),
		},
		{
			name:      "all labels",
			instances: allZonesInstances,
			expected:  util.MergeMaps(manifests.GetTopLevelLabels(), map[string]string{"app.kubernetes.io/name": "external-dns"}, map[string]string{"app.kubernetes.io/name": "external-dns-private"}),
		},
	}

	for _, test := range tests {
		got := getLabels(test.instances...)
		require.Equal(t, test.expected, got, "expected labels do not match got")
	}
}

func TestCleanObjs(t *testing.T) {
	onlyPrivZonesInstances, err := instances(&onlyPrivZones)
	require.NoError(t, err)

	onlyPubZonesInstances, err := instances(&onlyPubZones)
	require.NoError(t, err)

	noZoneInstances, err := instances(&noZones)
	require.NoError(t, err)

	allZonesInstances, err := instances(&allZones)
	require.NoError(t, err)

	tests := []struct {
		name      string
		instances []instance
		expected  []cleanObj
	}{
		{
			name:      "private dns clean",
			instances: onlyPubZonesInstances,
			expected: []cleanObj{{
				resources: onlyPubZonesInstances[1].resources,
				labels:    util.MergeMaps(manifests.GetTopLevelLabels(), map[string]string{"app.kubernetes.io/name": "external-dns-private"}),
			}},
		},
		{
			name:      "public dns clean",
			instances: onlyPrivZonesInstances,
			expected: []cleanObj{{
				resources: onlyPrivZonesInstances[0].resources,
				labels:    util.MergeMaps(manifests.GetTopLevelLabels(), map[string]string{"app.kubernetes.io/name": "external-dns"}),
			}},
		},
		{
			name:      "all dns clean",
			instances: noZoneInstances,
			expected: []cleanObj{
				{
					resources: noZoneInstances[0].resources,
					labels:    util.MergeMaps(manifests.GetTopLevelLabels(), map[string]string{"app.kubernetes.io/name": "external-dns"}),
				},
				{
					resources: noZoneInstances[1].resources,
					labels:    util.MergeMaps(manifests.GetTopLevelLabels(), map[string]string{"app.kubernetes.io/name": "external-dns-private"}),
				}},
		},
		{
			name:      "no dns clean",
			instances: allZonesInstances,
			expected:  []cleanObj(nil),
		},
	}

	for _, test := range tests {
		got := cleanObjs(test.instances)
		require.Equal(t, test.expected, got)
	}
}

func TestActionFromConfig(t *testing.T) {
	emptyDns, err := manifests.NewExternalDNSConfig(&config.Config{}, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PublicProvider), TenantId: "tenant", ClientId: "client", Namespace: "ns", IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: []string{}})
	require.NoError(t, err)
	oneDns, err := manifests.NewExternalDNSConfig(&config.Config{}, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PublicProvider), TenantId: "tenant", ClientId: "client", Namespace: "ns", IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: []string{"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test"}})
	require.NoError(t, err)
	multipleDns, err := manifests.NewExternalDNSConfig(&config.Config{}, manifests.InputExternalDNSConfig{Provider: to.Ptr(manifests.PublicProvider), TenantId: "tenant", ClientId: "client", Namespace: "ns", IdentityType: manifests.IdentityTypeMSI, ResourceTypes: map[manifests.ResourceType]struct{}{manifests.ResourceTypeIngress: struct{}{}}, DnsZoneresourceIDs: []string{"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test", "/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2"}})
	require.NoError(t, err)

	tests := []struct {
		name     string
		conf     *manifests.ExternalDnsConfig
		expected action
	}{
		{
			name:     "empty dns",
			conf:     emptyDns,
			expected: clean,
		},
		{
			name:     "one dns",
			conf:     oneDns,
			expected: deploy,
		},
		{
			name:     "multiple dns",
			conf:     multipleDns,
			expected: deploy,
		},
	}

	for _, test := range tests {
		got := actionFromConfig(test.conf)
		require.Equal(t, test.expected, got)
	}
}

func TestAddExternalDnsReconciler(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)
	err = addExternalDnsReconciler(m, []client.Object{obj(gvk1, nil)})
	require.NoError(t, err)
}

func TestAddExternalDnsCleaner(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)

	testInstances, err := instances(&noZones)
	require.NoError(t, err)

	err = addExternalDnsCleaner(m, testInstances)
	require.NoError(t, err)
}

func TestNewExternalDns(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)

	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}
	err = NewExternalDns(m, conf)
	require.NoError(t, err)
}

func obj(gvk schema.GroupVersionKind, labels map[string]string) client.Object {
	o := &unstructured.Unstructured{}
	o.SetLabels(labels)
	o.SetGroupVersionKind(gvk)
	return o
}
