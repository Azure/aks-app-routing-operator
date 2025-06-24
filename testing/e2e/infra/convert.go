package infra

import (
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/go-autorest/autorest/azure"
)

func (p Provisioned) Loadable() (LoadableProvisioned, error) {
	cluster, err := azure.ParseResourceID(p.Cluster.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing cluster resource id: %w", err)
	}

	containerRegistry, err := azure.ParseResourceID(p.ContainerRegistry.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing container registry resource id: %w", err)
	}

	zones := make([]withLoadableCert[LoadableZone], len(p.Zones))
	for i, zone := range p.Zones {
		z, err := azure.ParseResourceID(zone.Zone.GetId())
		if err != nil {
			return LoadableProvisioned{}, fmt.Errorf("parsing Zone resource id: %w", err)
		}
		zones[i] = withLoadableCert[LoadableZone]{
			Zone: LoadableZone{
				ResourceId:  z,
				Nameservers: zone.Zone.GetNameservers(),
			},
			CertName: zone.Cert.GetName(),
			CertId:   zone.Cert.GetId(),
		}
	}

	privateZones := make([]withLoadableCert[azure.Resource], len(p.PrivateZones))
	for i, privateZone := range p.PrivateZones {
		z, err := azure.ParseResourceID(privateZone.Zone.GetId())
		if err != nil {
			return LoadableProvisioned{}, fmt.Errorf("parsing private Zone resource id: %w", err)
		}
		privateZones[i] = withLoadableCert[azure.Resource]{
			Zone:     z,
			CertName: privateZone.Cert.GetName(),
			CertId:   privateZone.Cert.GetId(),
		}
	}

	keyVault, err := azure.ParseResourceID(p.KeyVault.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing key vault resource id: %w", err)
	}

	resourceGroup, err := arm.ParseResourceID(p.ResourceGroup.GetId())
	if err != nil {
		return LoadableProvisioned{}, fmt.Errorf("parsing resource group resource id: %w", err)
	}

	return LoadableProvisioned{
		Name:                p.Name,
		Cluster:             cluster,
		ClusterLocation:     p.Cluster.GetLocation(),
		ClusterDnsServiceIp: p.Cluster.GetDnsServiceIp(),
		ClusterPrincipalId:  p.Cluster.GetPrincipalId(),
		ClusterClientId:     p.Cluster.GetClientId(),
		ClusterOidcUrl:      p.Cluster.GetOidcUrl(),
		ClusterOptions:      p.Cluster.GetOptions(),
		ContainerRegistry:   containerRegistry,
		Zones:               zones,
		PrivateZones:        privateZones,
		KeyVault:            keyVault,
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
	zs := make([]WithCert[Zone], len(l.Zones))
	for i, z := range l.Zones {
		zs[i] = WithCert[Zone]{
			Zone: clients.LoadZone(z.Zone.ResourceId, z.Zone.Nameservers),
			Cert: clients.LoadCert(z.CertName, z.CertId),
		}
	}
	pzs := make([]WithCert[PrivateZone], len(l.PrivateZones))
	for i, pz := range l.PrivateZones {
		pzs[i] = WithCert[PrivateZone]{
			Zone: clients.LoadPrivateZone(pz.Zone),
			Cert: clients.LoadCert(pz.CertName, pz.CertId),
		}
	}

	return Provisioned{
		Name:              l.Name,
		Cluster:           clients.LoadAks(l.Cluster, l.ClusterDnsServiceIp, l.ClusterLocation, l.ClusterPrincipalId, l.ClusterClientId, l.ClusterOidcUrl, l.ClusterOptions),
		ContainerRegistry: clients.LoadAcr(l.ContainerRegistry),
		Zones:             zs,
		PrivateZones:      pzs,
		KeyVault:          clients.LoadAkv(l.KeyVault),
		ResourceGroup:     clients.LoadRg(l.ResourceGroup),
		SubscriptionId:    l.SubscriptionId,
		TenantId:          l.TenantId,
		E2eImage:          l.E2eImage,
		OperatorImage:     l.OperatorImage,
	}, nil
}
