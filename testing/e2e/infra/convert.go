package infra

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
)

func (p Provisioned) Loadable() (LoadableProvisioned, error) {
	cluster, err := arm.ParseResourceID(p.Cluster.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing cluster resource id: %w", err)
	}

	containerRegistry, err := arm.ParseResourceID(p.ContainerRegistry.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing container registry resource id: %w", err)
	}

	zones := make([]LoadableZone, len(p.Zones))
	for i, zone := range p.Zones {
		z, err := arm.ParseResourceID(zone.GetId())
		if err != nil {
			return LoadableProvisioned{}, fmt.Errorf("parsing zone resource id: %w", err)
		}
		zones[i] = LoadableZone{
			ResourceId:  *z,
			Nameservers: zone.GetNameservers(),
		}
	}

	privateZones := make([]arm.ResourceID, len(p.PrivateZones))
	for i, privateZone := range p.PrivateZones {
		z, err := arm.ParseResourceID(privateZone.GetId())
		if err != nil {
			return LoadableProvisioned{}, fmt.Errorf("parsing private zone resource id: %w", err)
		}
		privateZones[i] = *z
	}

	keyVault, err := arm.ParseResourceID(p.KeyVault.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing key vault resource id: %w", err)
	}

	resourceGroup, err := arm.ParseResourceID(p.ResourceGroup.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing resource group resource id: %w", err)
	}

	return LoadableProvisioned{
		Name:                p.Name,
		Cluster:             *cluster,
		ClusterLocation:     p.Cluster.GetLocation(),
		ClusterDnsServiceIp: p.Cluster.GetDnsServiceIp(),
		ClusterPrincipalId:  p.Cluster.GetPrincipalId(),
		ClusterClientId:     p.Cluster.GetClientId(),
		ContainerRegistry:   *containerRegistry,
		Zones:               zones,
		PrivateZones:        privateZones,
		KeyVault:            *keyVault,
		CertName:            p.Cert.GetName(),
		CertId:              p.Cert.GetId(),
		ResourceGroup:       *resourceGroup,
		SubscriptionId:      p.SubscriptionId,
		TenantId:            p.TenantId,
		E2eImage:            p.E2eImage,
		OperatorImage:       p.OperatorImage,
	}, nil
}

func ToLoadable(p []Provisioned) ([]LoadableProvisioned, error) {
	ret := make([]LoadableProvisioned, len(p))
	for i, provisioned := range p {
		loadable, err := provisioned.Loadable()
		if err != nil {
			return nil, fmt.Errorf("loading provisioned %s: %w", provisioned.Name, err)
		}
		ret[i] = loadable
	}
	return ret, nil
}

func ToProvisioned(l []LoadableProvisioned) ([]Provisioned, error) {
	ret := make([]Provisioned, len(l))
	for i, loadable := range l {
		provisioned, err := loadable.Provisioned()
		if err != nil {
			return nil, fmt.Errorf("parsing loadable %s: %w", loadable.Name, err)
		}
		ret[i] = provisioned
	}
	return ret, nil
}

func (l LoadableProvisioned) Provisioned() (Provisioned, error) {
	zs := make([]zone, len(l.Zones))
	for i, z := range l.Zones {
		zs[i] = clients.LoadZone(z.ResourceId, z.Nameservers)
	}
	pzs := make([]privateZone, len(l.PrivateZones))
	for i, pz := range l.PrivateZones {
		pzs[i] = clients.LoadPrivateZone(pz)
	}

	return Provisioned{
		Name:              l.Name,
		Cluster:           clients.LoadAks(l.Cluster, l.ClusterDnsServiceIp, l.ClusterLocation, l.ClusterPrincipalId, l.ClusterClientId),
		ContainerRegistry: clients.LoadAcr(l.ContainerRegistry),
		Zones:             zs,
		PrivateZones:      pzs,
		KeyVault:          clients.LoadAkv(l.KeyVault),
		Cert:              clients.LoadCert(l.CertName, l.CertId),
		ResourceGroup:     clients.LoadRg(l.ResourceGroup),
		SubscriptionId:    l.SubscriptionId,
		TenantId:          l.TenantId,
		E2eImage:          l.E2eImage,
		OperatorImage:     l.OperatorImage,
	}, nil
}
