package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// when creating a resource group mark it for deletion after 2 hours
// https://github.com/Azure/aks-app-routing-operator/blob/731d6232b8f1485fe085becbe36948719ddf735f/devenv/main.tf#L31

type rg struct {
	name string
}

type RgOpt func(rg *armresources.ResourceGroup) error

func DeleteAfterOpt(d time.Duration) RgOpt {
	return func(rg *armresources.ResourceGroup) error {
		if rg.Tags == nil {
			rg.Tags = map[string]*string{}
		}

		rg.Tags["deletion_marked_by"] = util.StringPtr("gc")
		rg.Tags["deletion_due_time"] = util.StringPtr(fmt.Sprint(time.Now().Add(d).Unix()))

		return nil
	}
}

func NewResourceGroup(ctx context.Context, subscriptionId, name, location string, rgOpts ...RgOpt) (*rg, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create resource group " + name)
	defer lgr.Info("finished creating resource group " + name)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armresources.NewResourceGroupsClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating resource group client: %w", err)
	}

	new := armresources.ResourceGroup{
		Location: util.StringPtr(location),
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
