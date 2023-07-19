package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
)

type acr struct {
	name          string
	resourceGroup string
}

func NewAcr(ctx context.Context, subscriptionId, resourceGroup, name, location string) (*acr, error) {
	name = nonAlphanumericRegex.ReplaceAllString(name, "")

	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create acr " + name)
	defer lgr.Info("finished creating acr " + name)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armcontainerregistry.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}

	new := &armcontainerregistry.Registry{
		Location: util.StringPtr(location),
		SKU: &armcontainerregistry.SKU{
			Name: to.Ptr(armcontainerregistry.SKUNameBasic),
		},
	}
	poller, err := factory.NewRegistriesClient().BeginCreate(ctx, resourceGroup, name, *new, nil)
	if err != nil {
		return nil, fmt.Errorf("starting to create registry: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("waiting for registry creation to complete: %w", err)
	}

	return &acr{
		name:          *result.Name,
		resourceGroup: resourceGroup,
	}, nil
}

func (a *acr) GetName() string {
	return a.name
}

func (a *acr) CreateCertificate() error {
}
