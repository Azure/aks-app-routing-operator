package suites

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
)

type cfgBuilder struct {
	msi      string
	tenantId string
	location string
	isDalec  bool
}

func builderFromInfra(infra infra.Provisioned) cfgBuilder {
	_, isDalec := infra.Cluster.GetOptions()[clients.DalecClusterOpt.Name]
	return cfgBuilder{
		msi:      infra.Cluster.GetClientId(),
		tenantId: infra.TenantId,
		location: infra.Cluster.GetLocation(),
		isDalec:  isDalec,
	}
}

type cfgBuilderWithOsm struct {
	cfgBuilder
	osmEnabled []bool
}

func (c cfgBuilder) withOsm(in infra.Provisioned, enabled ...bool) cfgBuilderWithOsm {
	osmCluster := false
	if _, ok := in.Cluster.GetOptions()[clients.OsmClusterOpt.Name]; ok {
		osmCluster = true
	}

	if len(enabled) == 0 {
		enabled = []bool{osmCluster}
	}

	osms := make([]bool, 0, len(enabled))
	for _, e := range enabled {
		// osm tests can only work if the cluster has osm installed.
		// filter out any enabled on clusters without osm
		if !osmCluster && e {
			continue
		}

		if osmCluster && !e {
			continue
		}

		osms = append(osms, e)
	}

	return cfgBuilderWithOsm{
		cfgBuilder: c,
		osmEnabled: osms,
	}
}

type cfgBuilderWithVersions struct {
	cfgBuilderWithOsm
	versions []manifests.OperatorVersion
}

func (c cfgBuilderWithOsm) withVersions(versions ...manifests.OperatorVersion) cfgBuilderWithVersions {
	if len(versions) == 0 {
		versions = manifests.AllUsedOperatorVersions
	}

	return cfgBuilderWithVersions{
		cfgBuilderWithOsm: c,
		versions:          versions,
	}
}

type cfgBuilderWithZones struct {
	cfgBuilderWithVersions
	zones            []manifests.DnsZones
	enableGatewayTLS bool
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

func (c cfgBuilderWithZones) withGatewayTLS(enabled bool) cfgBuilderWithZones {
	c.enableGatewayTLS = enabled
	return c
}

type operatorCfgs []manifests.OperatorConfig

func (c cfgBuilderWithZones) build() operatorCfgs {
	ret := operatorCfgs{}

	for _, osmEnabled := range c.osmEnabled {
		for _, version := range c.versions {
			// Dalec clusters only produce configs for versions that support dalec.
			// SupportsDalec() is the single source of truth — when a new released
			// version ships with dalec, update it there.
			if c.isDalec && !version.SupportsDalec() {
				continue
			}

			for _, zones := range c.zones {
				ret = append(ret, manifests.OperatorConfig{
					Version:          version,
					Location:         c.location,
					TenantId:         c.tenantId,
					Msi:              c.msi,
					Zones:            zones,
					DisableOsm:       !osmEnabled,
					EnableGatewayTLS: c.enableGatewayTLS,
					EnableDalecNginx: c.isDalec,
				})
			}
		}
	}

	return ret
}
