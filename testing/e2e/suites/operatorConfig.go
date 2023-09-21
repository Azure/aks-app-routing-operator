package suites

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
)

type cfgBuilder struct {
	msi      string
	tenantId string
	location string
}

func builderFromInfra(infra infra.Provisioned) cfgBuilder {
	return cfgBuilder{
		msi:      infra.Cluster.GetClientId(),
		tenantId: infra.TenantId,
		location: infra.Cluster.GetLocation(),
	}
}

type cfgBuilderWithOsm struct {
	cfgBuilder
	osmEnabled []bool
}

func (c cfgBuilder) withOsm(enabled ...bool) cfgBuilderWithOsm {
	if len(enabled) == 0 {
		enabled = []bool{false}
	}

	return cfgBuilderWithOsm{
		cfgBuilder: c,
		osmEnabled: enabled,
	}
}

type cfgBuilderWithVersions struct {
	cfgBuilderWithOsm
	versions []manifests.OperatorVersion
}

func (c cfgBuilderWithOsm) withVersions(versions ...manifests.OperatorVersion) cfgBuilderWithVersions {
	if len(versions) == 0 {
		versions = []manifests.OperatorVersion{manifests.OperatorVersionLatest}
	}

	return cfgBuilderWithVersions{
		cfgBuilderWithOsm: c,
		versions:          versions,
	}
}

type cfgBuilderWithZones struct {
	cfgBuilderWithVersions
	zones []manifests.DnsZones
}

func (c cfgBuilderWithVersions) withZones(public []manifests.DnsZoneCount, private []manifests.DnsZoneCount) cfgBuilderWithZones {
	if len(public) == 0 {
		public = manifests.AllDnsZoneCounts
	}
	if len(private) == 0 {
		private = manifests.AllDnsZoneCounts
	}

	zones := []manifests.DnsZones{}
	for _, pub := range public {
		for _, pri := range private {
			zones = append(zones, manifests.DnsZones{
				Public:  pub,
				Private: pri,
			})
		}
	}

	return cfgBuilderWithZones{
		cfgBuilderWithVersions: c,
		zones:                  zones,
	}
}

type operatorCfgs []manifests.OperatorConfig

func (c cfgBuilderWithZones) build() operatorCfgs {
	ret := operatorCfgs{}

	for _, osmEnabled := range c.osmEnabled {
		for _, version := range c.versions {
			for _, zones := range c.zones {
				ret = append(ret, manifests.OperatorConfig{
					Version:    version,
					Location:   c.location,
					TenantId:   c.tenantId,
					Msi:        c.msi,
					Zones:      zones,
					DisableOsm: !osmEnabled,
				})
			}
		}
	}

	return ret
}
