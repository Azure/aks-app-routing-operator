package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

type rg struct {
	name string
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

func NewResourceGroup(ctx context.Context, subscriptionId, name, location string, rgOpts ...RgOpt) (*rg, error) {
	lgr := logger.FromContext(ctx).With("name", name, "location", location, "subscriptionId", subscriptionId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create resource group")
	defer lgr.Info("finished creating resource group")

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating resource group client: %w", err)
	}

	new := armresources.ResourceGroup{
		Location: to.Ptr(location),
	}
	for _, opt := range rgOpts {
		if err := opt(&new); err != nil {
			return nil, fmt.Errorf("applying resource group option: %w", err)
		}
	}

	resp, err := client.CreateOrUpdate(ctx, name, new, nil)
	if err != nil {
		return nil, fmt.Errorf("creating resource group: %w", err)
	}

	return &rg{
		name: *resp.Name,
	}, nil
}

func (r *rg) GetName() string {
	return r.name
}
