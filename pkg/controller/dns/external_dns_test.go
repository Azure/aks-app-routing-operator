package dns

import (
	"os"
	"reflect"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	uid     = uuid.New().String()
	noZones = config.Config{
		ClusterUid:        uid,
		PrivateZoneConfig: config.DnsZoneConfig{},
		PublicZoneConfig:  config.DnsZoneConfig{},
	}
	onlyPubZones = config.Config{
		ClusterUid:        uid,
		PrivateZoneConfig: config.DnsZoneConfig{},
		PublicZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       []string{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/publicdnszones/test.com"},
		},
	}
	onlyPrivZones = config.Config{
		ClusterUid:       uid,
		PublicZoneConfig: config.DnsZoneConfig{},
		PrivateZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       []string{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/privatednszones/test.com"},
		},
	}
	allZones = config.Config{
		ClusterUid: uid,
		PublicZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       []string{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/publicdnszones/test.com"},
		},
		PrivateZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       []string{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/privatednszones/test.com"},
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
	restConfig, _ = testutils.StartTestingEnv()

	exitCode := m.Run()

	testutils.CleanupTestingEnv()
	os.Exit(exitCode)
}

var (
	restConfig *rest.Config
	self       *appsv1.Deployment = nil
)

func TestPublicConfig(t *testing.T) {
	tests := []struct {
		name     string
		conf     *config.Config
		expected *manifests.ExternalDnsConfig
	}{
		{
			name: "all zones config",
			conf: &allZones,
			expected: &manifests.ExternalDnsConfig{
				TenantId:           allZones.TenantID,
				Subscription:       allZones.PublicZoneConfig.Subscription,
				ResourceGroup:      allZones.PublicZoneConfig.ResourceGroup,
				Provider:           manifests.PublicProvider,
				DnsZoneResourceIDs: allZones.PublicZoneConfig.ZoneIds,
			},
		},
		{
			name: "ony private config",
			conf: &onlyPrivZones,
			expected: &manifests.ExternalDnsConfig{
				TenantId:           onlyPrivZones.TenantID,
				Subscription:       onlyPrivZones.PublicZoneConfig.Subscription,
				ResourceGroup:      onlyPrivZones.PublicZoneConfig.ResourceGroup,
				Provider:           manifests.PublicProvider,
				DnsZoneResourceIDs: onlyPrivZones.PublicZoneConfig.ZoneIds,
			},
		},
	}

	for _, test := range tests {
		got := *publicConfig(test.conf)
		if !reflect.DeepEqual(got, *test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", *test.expected,
			)
		}
	}
}

func TestPrivateConfig(t *testing.T) {
	tests := []struct {
		name     string
		conf     *config.Config
		expected *manifests.ExternalDnsConfig
	}{
		{
			name: "all zones config",
			conf: &allZones,
			expected: &manifests.ExternalDnsConfig{
				TenantId:           allZones.TenantID,
				Subscription:       allZones.PrivateZoneConfig.Subscription,
				ResourceGroup:      allZones.PrivateZoneConfig.ResourceGroup,
				Provider:           manifests.PrivateProvider,
				DnsZoneResourceIDs: allZones.PrivateZoneConfig.ZoneIds,
			},
		},
		{
			name: "ony private config",
			conf: &onlyPrivZones,
			expected: &manifests.ExternalDnsConfig{
				TenantId:           onlyPrivZones.TenantID,
				Subscription:       onlyPrivZones.PrivateZoneConfig.Subscription,
				ResourceGroup:      onlyPrivZones.PrivateZoneConfig.ResourceGroup,
				Provider:           manifests.PrivateProvider,
				DnsZoneResourceIDs: onlyPrivZones.PrivateZoneConfig.ZoneIds,
			},
		},
	}

	for _, test := range tests {
		got := *privateConfig(test.conf)
		if !reflect.DeepEqual(got, *test.expected) {
			t.Error(
				"For", test.name,
				"got", got,
				"expected", *test.expected,
			)
		}
	}
}

func TestInstances(t *testing.T) {
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
					config:    publicConfig(&noZones),
					resources: manifests.ExternalDnsResources(&noZones, self, []*manifests.ExternalDnsConfig{publicConfig(&noZones)}),
					action:    clean,
				},
				{
					config:    privateConfig(&noZones),
					resources: manifests.ExternalDnsResources(&noZones, self, []*manifests.ExternalDnsConfig{privateConfig(&noZones)}),
					action:    clean,
				},
			},
		},
		{
			name: "private deploy",
			conf: &onlyPrivZones,
			expected: []instance{
				{
					config:    publicConfig(&onlyPrivZones),
					resources: manifests.ExternalDnsResources(&onlyPrivZones, self, []*manifests.ExternalDnsConfig{publicConfig(&onlyPrivZones)}),
					action:    clean,
				},
				{
					config:    privateConfig(&onlyPrivZones),
					resources: manifests.ExternalDnsResources(&onlyPrivZones, self, []*manifests.ExternalDnsConfig{privateConfig(&onlyPrivZones)}),
					action:    deploy,
				},
			},
		},
		{
			name: "public deploy",
			conf: &onlyPubZones,
			expected: []instance{
				{
					config:    publicConfig(&onlyPubZones),
					resources: manifests.ExternalDnsResources(&onlyPubZones, self, []*manifests.ExternalDnsConfig{publicConfig(&onlyPubZones)}),
					action:    deploy,
				},
				{
					config:    privateConfig(&onlyPubZones),
					resources: manifests.ExternalDnsResources(&onlyPubZones, self, []*manifests.ExternalDnsConfig{privateConfig(&onlyPubZones)}),
					action:    clean,
				},
			},
		},
		{
			name: "all deploy",
			conf: &allZones,
			expected: []instance{
				{
					config:    publicConfig(&allZones),
					resources: manifests.ExternalDnsResources(&allZones, self, []*manifests.ExternalDnsConfig{publicConfig(&allZones)}),
					action:    deploy,
				},
				{
					config:    privateConfig(&allZones),
					resources: manifests.ExternalDnsResources(&allZones, self, []*manifests.ExternalDnsConfig{privateConfig(&allZones)}),
					action:    deploy,
				},
			},
		},
	}

	for _, test := range tests {
		instances := instances(test.conf, self)
		if !reflect.DeepEqual(instances, test.expected) {
			t.Error(
				"For", test.name,
				"expected", test.expected,
				"got", instances,
			)
		}
	}
}

func TestFilterAction(t *testing.T) {
	allClean := instances(&noZones, self)
	allDeploy := instances(&allZones, self)
	oneDeployOneClean := instances(&onlyPrivZones, self)

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
	instances := instances(&noZones, self)
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
	tests := []struct {
		name      string
		instances []instance
		expected  map[string]string
	}{
		{
			name:      "always returns top level",
			instances: []instance{},
			expected:  manifests.TopLevelLabels,
		},
		{
			name:      "top level and private",
			instances: filterAction(instances(&onlyPrivZones, self), deploy),
			expected:  util.MergeMaps(manifests.TopLevelLabels, manifests.PrivateProvider.Labels()),
		},
		{
			name:      "top level and public",
			instances: filterAction(instances(&onlyPubZones, self), deploy),
			expected:  util.MergeMaps(manifests.TopLevelLabels, manifests.PublicProvider.Labels()),
		},
		{
			name:      "all labels",
			instances: instances(&allZones, self),
			expected:  util.MergeMaps(manifests.TopLevelLabels, manifests.PublicProvider.Labels(), manifests.PrivateProvider.Labels()),
		},
	}

	for _, test := range tests {
		got := getLabels(test.instances...)
		require.Equal(t, test.expected, got, "expected labels do not match got")
	}
}

func TestCleanObjs(t *testing.T) {
	tests := []struct {
		name      string
		instances []instance
		expected  []cleanObj
	}{
		{
			name:      "private dns clean",
			instances: instances(&onlyPubZones, self),
			expected: []cleanObj{{
				resources: instances(&onlyPubZones, self)[1].resources,
				labels:    util.MergeMaps(manifests.TopLevelLabels, manifests.PrivateProvider.Labels()),
			}},
		},
		{
			name:      "public dns clean",
			instances: instances(&onlyPrivZones, self),
			expected: []cleanObj{{
				resources: instances(&onlyPrivZones, self)[0].resources,
				labels:    util.MergeMaps(manifests.TopLevelLabels, manifests.PublicProvider.Labels()),
			}},
		},
		{
			name:      "all dns clean",
			instances: instances(&noZones, self),
			expected: []cleanObj{
				{
					resources: instances(&noZones, self)[0].resources,
					labels:    util.MergeMaps(manifests.TopLevelLabels, manifests.PublicProvider.Labels()),
				},
				{
					resources: instances(&noZones, self)[1].resources,
					labels:    util.MergeMaps(manifests.TopLevelLabels, manifests.PrivateProvider.Labels()),
				}},
		},
		{
			name:      "no dns clean",
			instances: instances(&allZones, self),
			expected:  []cleanObj(nil),
		},
	}

	for _, test := range tests {
		got := cleanObjs(test.instances)
		require.Equal(t, test.expected, got)
	}
}

func TestActionFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		conf     *manifests.ExternalDnsConfig
		expected action
	}{
		{
			name: "empty dns",
			conf: &manifests.ExternalDnsConfig{
				DnsZoneResourceIDs: []string{},
			},
			expected: clean,
		},
		{
			name: "one dns",
			conf: &manifests.ExternalDnsConfig{
				DnsZoneResourceIDs: []string{"one"},
			},
			expected: deploy,
		},
		{
			name: "multiple dns",
			conf: &manifests.ExternalDnsConfig{
				DnsZoneResourceIDs: []string{"one", "two"},
			},
			expected: deploy,
		},
	}

	for _, test := range tests {
		got := actionFromConfig(test.conf)
		require.Equal(t, test.expected, got)
	}
}

func TestAddExternalDnsReconciler(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{MetricsBindAddress: "0"})
	require.NoError(t, err)
	err = addExternalDnsReconciler(m, []client.Object{obj(gvk1, nil)})
	require.NoError(t, err)
}

func TestAddExternalDnsCleaner(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{MetricsBindAddress: "0"})
	require.NoError(t, err)

	err = addExternalDnsCleaner(m, []cleanObj{
		{
			resources: instances(&noZones, self)[0].resources,
			labels:    util.MergeMaps(manifests.TopLevelLabels, manifests.PublicProvider.Labels()),
		}})
	require.NoError(t, err)
}

func TestNewExternalDns(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{MetricsBindAddress: "0"})
	require.NoError(t, err)

	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}
	err = NewExternalDns(m, conf, self)
	require.NoError(t, err)
}

func obj(gvk schema.GroupVersionKind, labels map[string]string) client.Object {
	o := &unstructured.Unstructured{}
	o.SetLabels(labels)
	o.SetGroupVersionKind(gvk)
	return o
}
