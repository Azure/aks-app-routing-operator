package infra

import (
	"fmt"

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

	zones := make([]arm.ResourceID, len(p.Zones))
	for i, zone := range p.Zones {
		z, err := arm.ParseResourceID(zone.GetId())
		if err != nil {
			return LoadableProvisioned{}, fmt.Errorf("parsing zone resource id: %w", err)
		}
		zones[i] = *z
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
		Name:              p.Name,
		Cluster:           *cluster,
		ContainerRegistry: *containerRegistry,
		Zones:             zones,
		PrivateZones:      privateZones,
		KeyVault:          *keyVault,
		CertName:          p.Cert.GetName(),
		ResourceGroup:     *resourceGroup,
	}, nil
}

func Loadable(p []Provisioned) ([]LoadableProvisioned, error) {
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
