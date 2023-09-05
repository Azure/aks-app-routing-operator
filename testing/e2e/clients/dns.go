package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
)

type zone struct {
	name           string
	subscriptionId string
	resourceGroup  string
	id             string
}

type privateZone struct {
	name           string
	subscriptionId string
	resourceGroup  string
	id             string
}

// ZoneOpt specifies what kind of zone to create
type ZoneOpt func(z *armdns.Zone) error

// PrivateZoneOpt specifies what kind of private zone to create
type PrivateZoneOpt func(z *armprivatedns.PrivateZone) error

func LoadZone(id arm.ResourceID) *zone {
	return &zone{
		id:             id.String(),
		name:           id.Name,
		subscriptionId: id.SubscriptionID,
		resourceGroup:  id.ResourceGroupName,
	}
}

func NewZone(ctx context.Context, subscriptionId, resourceGroup, name string, zoneOpts ...ZoneOpt) (*zone, error) {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")
	name = name + ".com"

	lgr := logger.FromContext(ctx).With("name", name, "subscriptionId", subscriptionId, "resourceGroup", resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create zone")
	defer lgr.Info("finished creating zone")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armdns.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}

	z := &armdns.Zone{
		Location: to.Ptr("global"), // https://github.com/Azure/azure-cli/issues/6052 this must be global because DNS zones are global resources
		Name:     to.Ptr(name),
	}
	for _, opt := range zoneOpts {
		if err := opt(z); err != nil {
			return nil, fmt.Errorf("applying zone option: %w", err)
		}
	}
	resp, err := factory.NewZonesClient().CreateOrUpdate(ctx, resourceGroup, name, *z, nil)
	if err != nil {
		return nil, fmt.Errorf("creating zone: %w", err)
	}

	return &zone{
		name:           *resp.Name,
		resourceGroup:  resourceGroup,
		subscriptionId: subscriptionId,
		id:             *resp.ID,
	}, nil
}

func (z *zone) GetDnsZone(ctx context.Context) (*armdns.Zone, error) {
	lgr := logger.FromContext(ctx).With("name", z.name, "subscriptionId", z.subscriptionId, "resourceGroup", z.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to get dns")
	defer lgr.Info("finished getting dns")

	cred, err := getAzCred()
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

func (z *zone) GetNameservers(ctx context.Context) ([]string, error) {
	zone, err := z.GetDnsZone(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting dns zone: %w", err)
	}

	nameservers := zone.Properties.NameServers
	ret := make([]string, len(nameservers))
	for i, ns := range nameservers {
		ret[i] = *ns
	}

	return ret, nil
}

func (z *zone) GetId() string {
	return z.id
}

func LoadPrivateZone(id arm.ResourceID) *privateZone {
	return &privateZone{
		id:             id.String(),
		name:           id.Name,
		subscriptionId: id.SubscriptionID,
		resourceGroup:  id.ResourceGroupName,
	}
}

func NewPrivateZone(ctx context.Context, subscriptionId, resourceGroup, name string, opts ...PrivateZoneOpt) (*privateZone, error) {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")
	name = name + ".com"

	lgr := logger.FromContext(ctx).With("name", name, "subscriptionId", subscriptionId, "resourceGroup", resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create private zone")
	defer lgr.Info("finished creating private zone")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armprivatedns.NewPrivateZonesClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	pz := armprivatedns.PrivateZone{
		Location: to.Ptr("global"),
		Name:     to.Ptr(name),
	}
	for _, opt := range opts {
		if err := opt(&pz); err != nil {
			return nil, fmt.Errorf("applying private zone option: %w", err)
		}
	}
	poller, err := client.BeginCreateOrUpdate(ctx, resourceGroup, name, pz, nil)
	if err != nil {
		return nil, fmt.Errorf("creating private zone: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("creating private zone: %w", err)
	}

	return &privateZone{
		name:           *result.PrivateZone.Name,
		subscriptionId: subscriptionId,
		resourceGroup:  resourceGroup,
		id:             *result.ID,
	}, nil
}

func (p *privateZone) GetName() string {
	return p.name
}

func (p *privateZone) GetDnsZone(ctx context.Context) (*armprivatedns.PrivateZone, error) {
	lgr := logger.FromContext(ctx).With("name", p.name, "subscriptionId", p.subscriptionId, "resourceGroup", p.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to get private dns")
	defer lgr.Info("finished getting private dns")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armprivatedns.NewPrivateZonesClient(p.subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	resp, err := client.Get(ctx, p.resourceGroup, p.name, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dns: %w", err)
	}

	return &resp.PrivateZone, nil
}

func (p *privateZone) LinkVnet(ctx context.Context, linkName, vnetId string) error {
	linkName = nonAlphanumericRegex.ReplaceAllString(linkName, "")
	linkName = truncate(linkName, 80)

	lgr := logger.FromContext(ctx).With("name", p.name, "subscriptionId", p.subscriptionId, "resourceGroup", p.resourceGroup, "linkName", linkName, "vnetId", vnetId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to link vnet")
	defer lgr.Info("finished linking vnet")

	cred, err := getAzCred()
	if err != nil {
		return fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armprivatedns.NewClientFactory(p.subscriptionId, cred, nil)
	if err != nil {
		return fmt.Errorf("creating client factory: %w", err)
	}

	l := armprivatedns.VirtualNetworkLink{
		Location: to.Ptr("global"),
		Properties: &armprivatedns.VirtualNetworkLinkProperties{
			RegistrationEnabled: to.Ptr(false),
			VirtualNetwork: &armprivatedns.SubResource{
				ID: to.Ptr(vnetId),
			},
		},
		Name: to.Ptr(linkName),
	}
	_, err = factory.NewVirtualNetworkLinksClient().BeginCreateOrUpdate(ctx, p.resourceGroup, p.name, linkName, l, nil)
	if err != nil {
		return fmt.Errorf("creating virtual network link: %w", err)
	}

	return nil
}

func (p *privateZone) GetId() string {
	return p.id
}
