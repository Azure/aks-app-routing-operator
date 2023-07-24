package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
)

type aks struct {
	factory             *armcontainerservice.ClientFactory
	name, resourceGroup string
}

// McOpt specifies what kind of managed cluster to create
type McOpt func(mc *armcontainerservice.ManagedCluster) error

func NewAks(ctx context.Context, subscriptionId, resourceGroup, name, location string, mcOpts ...McOpt) (*aks, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create aks " + name)
	defer lgr.Info("finished creating aks " + name)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armcontainerservice.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating aks client factory: %w", err)
	}

	new := armcontainerservice.ManagedCluster{
		Location: util.StringPtr(location),
		Identity: &armcontainerservice.ManagedClusterIdentity{
			Type: to.Ptr(armcontainerservice.ResourceIdentityTypeSystemAssigned),
		},
		Properties: &armcontainerservice.ManagedClusterProperties{
			DNSPrefix: to.Ptr("approutinge2e"),
		},
	}
	for _, opt := range mcOpts {
		if err := opt(&new); err != nil {
			return nil, fmt.Errorf("applying cluster option: %w", err)
		}
	}

	poll, err := factory.NewManagedClustersClient().BeginCreateOrUpdate(ctx, resourceGroup, name, new, nil)
	if err != nil {
		return nil, fmt.Errorf("starting create cluster: %w", err)
	}

	result, err := poll.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("polling create operation: %w", err)
	}

	return &aks{
		factory:       factory,
		name:          *result.ManagedCluster.Name,
		resourceGroup: resourceGroup,
	}, nil
}

// we probably need to run inside clusters to support private clusters
// todo: in test build and push test image to cluster, then run test inside cluster
// todo: what if this ran in the same vnet as the cluster?
// https://learn.microsoft.com/en-us/azure/aks/private-clusters#options-for-connecting-to-the-private-cluster
// what if we use a self hosted runner on github?
func (a *aks) GetKubeconfig(ctx context.Context) ([]byte, error) {
	resp, err := a.factory.NewManagedClustersClient().ListClusterUserCredentials(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, fmt.Errorf("listing user credentials: %w", err)
	}

	kubeconfigs := resp.Kubeconfigs
	if kubeconfigs == nil || len(kubeconfigs) == 0 {
		return nil, fmt.Errorf("no kubeconfig returned")
	}

	return kubeconfigs[0].Value, nil
}

func (a *aks) GetCluster(ctx context.Context) (*armcontainerservice.ManagedCluster, error) {
	result, err := a.factory.NewManagedClustersClient().Get(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, fmt.Errorf("getting cluster: %w", err)
	}

	return &result.ManagedCluster, nil
}
