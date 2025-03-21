package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

type rg struct {
	name string
	id   string
}

type RgOpt func(rg *armresources.ResourceGroup) error

func DeleteAfterOpt(d time.Duration) RgOpt {
	return func(rg *armresources.ResourceGroup) error {
		if rg.Tags == nil {
			rg.Tags = map[string]*string{}
		}

		rg.Tags["deletion_marked_by"] = to.Ptr("gc")
		rg.Tags["deletion_due_time"] = to.Ptr(fmt.Sprint(time.Now().Add(d).Unix()))

		return nil
	}
}

func LoadRg(id arm.ResourceID) *rg {
	return &rg{
		id:   id.String(),
		name: id.Name,
	}
}

func NewResourceGroup(ctx context.Context, subscriptionId, name, location string, rgOpts ...RgOpt) (*rg, error) {
	lgr := logger.FromContext(ctx).With("name", name, "location", location, "subscriptionId", subscriptionId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create resource group")
	defer lgr.Info("finished creating resource group")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating resource group client: %w", err)
	}

	r := armresources.ResourceGroup{
		Location: to.Ptr(location),
	}
	for _, opt := range rgOpts {
		if err := opt(&r); err != nil {
			return nil, fmt.Errorf("applying resource group option: %w", err)
		}
	}

	resp, err := client.CreateOrUpdate(ctx, name, r, nil)
	if err != nil {
		return nil, fmt.Errorf("creating resource group: %w", err)
	}

	// guard against things that should be impossible
	if resp.ID == nil {
		return nil, fmt.Errorf("resource group ID is nil")
	}
	if resp.Name == nil {
		return nil, fmt.Errorf("resource group name is nil")
	}

	return &rg{
		name: *resp.Name,
		id:   *resp.ID,
	}, nil
}

func (r *rg) GetName() string {
	return r.name
}

func (r *rg) GetId() string {
	return r.id
}

func (r *rg) Cleanup(ctx context.Context) error {
	lgr := logger.FromContext(ctx).With("name", r.name)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to clean up resource group")
	defer lgr.Info("finished cleaning up resource group")

	cred, err := getAzCred()
	if err != nil {
		return fmt.Errorf("getting az credentials: %w", err)
	}

	id, err := arm.ParseResourceID(r.id)
	if err != nil {
		return fmt.Errorf("parsing resource ID: %w", err)
	}

	client, err := armresources.NewResourceGroupsClient(id.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("creating resource group client: %w", err)
	}

	// we don't care about poller, we just want to start the delete.
	// don't need to hang at the end of the test before marking as success
	if _, err = client.BeginDelete(ctx, r.name, nil); err != nil {
		return fmt.Errorf("deleting resource group: %w", err)
	}

	return nil
}
