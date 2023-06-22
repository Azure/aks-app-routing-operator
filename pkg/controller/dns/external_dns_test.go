package dns

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	appsv1 "k8s.io/api/apps/v1"
)

var (
	fqdn, _ = url.Parse("fqdn.com")

	noZones = config.Config{
		ClusterFqdn:       fqdn,
		PrivateZoneConfig: config.DnsZoneConfig{},
		PublicZoneConfig:  config.DnsZoneConfig{},
	}
	onlyPubZones = config.Config{
		ClusterFqdn:       fqdn,
		PrivateZoneConfig: config.DnsZoneConfig{},
		PublicZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       []string{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/publicdnszones/test.com"},
		},
	}
	onlyPrivZones = config.Config{
		ClusterFqdn:      fqdn,
		PublicZoneConfig: config.DnsZoneConfig{},
		PrivateZoneConfig: config.DnsZoneConfig{
			Subscription:  "subscription",
			ResourceGroup: "resourcegroup",
			ZoneIds:       []string{"/subscriptions/subscription/resourceGroups/resourcegroup/providers/Microsoft.Network/privatednszones/test.com"},
		},
	}
	allZones = config.Config{
		ClusterFqdn: fqdn,
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
)

var (
	self *appsv1.Deployment = nil
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
