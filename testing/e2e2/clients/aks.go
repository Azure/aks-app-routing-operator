package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
)

type aks struct {
	factory                             *armcontainerservice.ClientFactory
	name, subscriptionId, resourceGroup string
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
			AgentPoolProfiles: []*armcontainerservice.ManagedClusterAgentPoolProfile{
				{
					Name:   to.Ptr("default"),
					VMSize: to.Ptr("Standard_DS4_v2"),
					Count:  to.Ptr(int32(2)),
				},
			},
			AddonProfiles: map[string]*armcontainerservice.ManagedClusterAddonProfile{
				"azureKeyvaultSecretsProvider": {
					Enabled: to.Ptr(true),
					Config: map[string]*string{
						"enableSecretRotation": to.Ptr("true"),
					},
				},
			},
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

	lgr.Info(fmt.Sprintf("waiting for aks %s to be created", name))
	result, err := pollWithLog(ctx, poll, "still creating aks "+name)
	if err != nil {
		return nil, fmt.Errorf("creating cluster: %w", err)
	}

	return &aks{
		factory:        factory,
		name:           *result.ManagedCluster.Name,
		subscriptionId: subscriptionId,
		resourceGroup:  resourceGroup,
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
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to get aks " + a.name)
	defer lgr.Info("finished getting aks " + a.name)

	result, err := a.factory.NewManagedClustersClient().Get(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, fmt.Errorf("getting cluster: %w", err)
	}

	return &result.ManagedCluster, nil
}

func (a *aks) GetVnetId(ctx context.Context) (string, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to get vnet id for aks " + a.name)
	defer lgr.Info("finished getting vnet id for aks " + a.name)

	cred, err := GetAzCred()
	if err != nil {
		return "", fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armnetwork.NewVirtualNetworksClient(a.subscriptionId, cred, nil)
	if err != nil {
		return "", fmt.Errorf("creating network client: %w", err)
	}

	cluster, err := a.GetCluster(ctx)
	if err != nil {
		return "", fmt.Errorf("getting cluster: %w", err)
	}

	pager := client.NewListPager(*cluster.Properties.NodeResourceGroup, nil)
	page, err := pager.NextPage(ctx)
	if err != nil {
		return "", fmt.Errorf("listing vnet : %w", err)
	}

	vnets := page.Value
	if len(vnets) == 0 {
		return "", fmt.Errorf("no vnets found")
	}

	return *vnets[0].ID, nil
}
