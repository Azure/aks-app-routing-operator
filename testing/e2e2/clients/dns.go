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
	if z.Properties == nil {
		z.Properties = &armdns.ZoneProperties{}
	}

	z.Properties.ZoneType = to.Ptr(armdns.ZoneTypePrivate)
	return nil
}

func NewZone(ctx context.Context, subscriptionId, resourceGroup, name string, zoneOpts ...ZoneOpt) (*zone, error) {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")
	name = name + ".com"

	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create zone " + name)
	defer lgr.Info("finished creating zone" + name)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armdns.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}

	new := &armdns.Zone{
		Location: to.Ptr("global"), // https://github.com/Azure/azure-cli/issues/6052 this must be global because DNS zones are global resources
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

func (z *zone) GetDns(ctx context.Context) (*armdns.Zone, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to get dns")
	defer lgr.Info("finished getting dns")

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armdns.NewZonesClient(z.subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	resp, err := client.Get(ctx, z.resourceGroup, z.name, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dns: %w", err)
	}

	return &resp.Zone, nil
}

func (z *zone) GetName() string {
	return z.name
}

func NewPrivateZone(ctx context.Context, subscriptionId, resourceGroup, name string) (*privateZone, error) {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")
	name = name + ".com"

	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create private zone " + name)
	defer lgr.Info("finished creating private zone " + name)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armprivatedns.NewPrivateZonesClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	new := armprivatedns.PrivateZone{
		Location: to.Ptr("global"),
		Name:     to.Ptr(name),
	}

	poller, err := client.BeginCreateOrUpdate(ctx, resourceGroup, name, new, nil)
	if err != nil {
		return nil, fmt.Errorf("creating private zone: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("creating private zone: %w", err)
	}

	return &privateZone{zone{
		name:           *result.PrivateZone.Name,
		subscriptionId: subscriptionId,
		resourceGroup:  resourceGroup,
	}}, nil
}

func (p *privateZone) LinkVnet(ctx context.Context, linkName, vnetId string) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to link vnet" + linkName)
	defer lgr.Info("finished linking vnet" + linkName)

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
