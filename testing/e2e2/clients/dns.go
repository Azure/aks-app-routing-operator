package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
)

type zone struct {
	name           string
	subscriptionId string
	resourceGroup  string
}

type privateZone struct {
	zone
}

// ZoneOpt specifies what kind of zone to create
type ZoneOpt func(z *armdns.Zone) error

func privateZoneOpt(z *armdns.Zone) error {
	z.Properties.ZoneType = to.Ptr(armdns.ZoneTypePrivate)
	return nil
}

func NewZone(ctx context.Context, subscriptionId, resourceGroup, name, location string, zoneOpts ...ZoneOpt) (*zone, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create zone")
	defer lgr.Info("finished creating zone")

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armdns.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}

	new := &armdns.Zone{
		Location: to.Ptr(location),
		Name:     to.Ptr(name),
	}
	for _, opt := range zoneOpts {
		if err := opt(new); err != nil {
			return nil, fmt.Errorf("applying zone option: %w", err)
		}
	}
	resp, err := factory.NewZonesClient().CreateOrUpdate(ctx, resourceGroup, name, *new, nil)
	if err != nil {
		return nil, fmt.Errorf("creating zone: %w", err)
	}

	return &zone{
		name:           *resp.Name,
		resourceGroup:  resourceGroup,
		subscriptionId: subscriptionId,
	}, nil
}

func NewPrivateZone(ctx context.Context, subscriptionId, resourceGroup, name, location string) (*privateZone, error) {
	z, err := NewZone(ctx, subscriptionId, resourceGroup, name, location, privateZoneOpt)
	if err != nil {
		return nil, fmt.Errorf("creating private zone: %w", err)
	}

	return &privateZone{zone: *z}, nil
}

func (p *privateZone) LinkVnet(ctx context.Context, linkName, vnetId string) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to link vnet")
	defer lgr.Info("finished linking vnet")

	cred, err := GetAzCred()
	if err != nil {
		return fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armprivatedns.NewClientFactory(p.zone.subscriptionId, cred, nil)
	if err != nil {
		return fmt.Errorf("creating client factory: %w", err)
	}

	new := armprivatedns.VirtualNetworkLink{
		Properties: &armprivatedns.VirtualNetworkLinkProperties{
			VirtualNetwork: &armprivatedns.SubResource{
				ID: to.Ptr(vnetId),
			},
		},
		Name: to.Ptr(linkName),
	}
	_, err = factory.NewVirtualNetworkLinksClient().BeginCreateOrUpdate(ctx, p.zone.resourceGroup, p.zone.name, linkName, new, nil)
	if err != nil {
		return fmt.Errorf("creating virtual network link: %w", err)
	}

	return nil
}
