package clients

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"golang.org/x/exp/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// https://kubernetes.io/docs/concepts/workloads/
	// more specifically, these are compatible with kubectl rollout status
	workloadKinds = []string{"Deployment", "StatefulSet", "DaemonSet"}
)

type aks struct {
	name, subscriptionId, resourceGroup string
	id                                  string
}

// McOpt specifies what kind of managed cluster to create
type McOpt func(mc *armcontainerservice.ManagedCluster) error

// PrivateClusterOpt specifies that the cluster should be private
func PrivateClusterOpt(mc *armcontainerservice.ManagedCluster) error {
	if mc.Properties == nil {
		mc.Properties = &armcontainerservice.ManagedClusterProperties{}
	}

	if mc.Properties.APIServerAccessProfile == nil {
		mc.Properties.APIServerAccessProfile = &armcontainerservice.ManagedClusterAPIServerAccessProfile{}
	}

	mc.Properties.APIServerAccessProfile.EnablePrivateCluster = to.Ptr(true)
	return nil
}

func LoadAks(id arm.ResourceID) *aks {
	return &aks{
		id:             id.String(),
		name:           id.Name,
		resourceGroup:  id.ResourceGroupName,
		subscriptionId: id.SubscriptionID,
	}
}

func NewAks(ctx context.Context, subscriptionId, resourceGroup, name, location string, mcOpts ...McOpt) (*aks, error) {
	lgr := logger.FromContext(ctx).With("name", name, "resourceGroup", resourceGroup, "location", location)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create aks")
	defer lgr.Info("finished creating aks")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	factory, err := armcontainerservice.NewClientFactory(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating aks client factory: %w", err)
	}

	mc := armcontainerservice.ManagedCluster{
		Location: to.Ptr(location),
		Identity: &armcontainerservice.ManagedClusterIdentity{
			Type: to.Ptr(armcontainerservice.ResourceIdentityTypeSystemAssigned),
		},
		Properties: &armcontainerservice.ManagedClusterProperties{
			DNSPrefix:         to.Ptr("approutinge2e"),
			NodeResourceGroup: to.Ptr(truncate("MC_"+name, 80)),
			AgentPoolProfiles: []*armcontainerservice.ManagedClusterAgentPoolProfile{
				{
					Name:   to.Ptr("default"),
					VMSize: to.Ptr("Standard_DS3_v2"),
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
		if err := opt(&mc); err != nil {
			return nil, fmt.Errorf("applying cluster option: %w", err)
		}
	}

	poll, err := factory.NewManagedClustersClient().BeginCreateOrUpdate(ctx, resourceGroup, name, mc, nil)
	if err != nil {
		return nil, fmt.Errorf("starting create cluster: %w", err)
	}

	lgr.Info(fmt.Sprintf("waiting for aks %s to be created", name))
	result, err := pollWithLog(ctx, poll, "still creating aks "+name)
	if err != nil {
		return nil, fmt.Errorf("creating cluster: %w", err)
	}

	return &aks{
		id:             *result.ManagedCluster.ID,
		name:           *result.ManagedCluster.Name,
		subscriptionId: subscriptionId,
		resourceGroup:  resourceGroup,
	}, nil
}

func (a *aks) Deploy(ctx context.Context, objs []client.Object) error {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to deploy resources")
	defer lgr.Info("finished deploying resources")

	// wrap manifests into base64 zip file.
	// this is specified by the AKS ARM API.
	// https://github.com/FumingZhang/azure-cli/blob/aefcf3948ed4207bfcf5d53064e5dac8ea8f19ca/src/azure-cli/azure/cli/command_modules/acs/custom.py#L2750
	b := &bytes.Buffer{}
	zipWriter := zip.NewWriter(b)
	for i, obj := range objs {
		json, err := manifests.MarshalJson(obj)
		if err != nil {
			return fmt.Errorf("marshaling json for object: %w", err)
		}

		f, err := zipWriter.Create(fmt.Sprintf("manifests/%d.json", i))
		if err != nil {
			return fmt.Errorf("creating zip entry: %w", err)
		}

		if _, err := f.Write(json); err != nil {
			return fmt.Errorf("writing zip entry: %w", err)
		}
	}
	zipWriter.Close()
	encoded := base64.StdEncoding.EncodeToString(b.Bytes())

	if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
		Command: to.Ptr("kubectl apply -f manifests/"),
		Context: &encoded,
	}); err != nil {
		return fmt.Errorf("running kubectl apply: %w", err)
	}

	if err := a.waitStable(ctx, objs); err != nil {
		return fmt.Errorf("waiting for resources to be stable: %w", err)
	}

	return nil
}

func (a *aks) waitStable(ctx context.Context, objs []client.Object) error {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to wait for resources to be stable")
	defer lgr.Info("finished waiting for resources to be stable")

	for _, obj := range objs {
		kind := obj.GetObjectKind().GroupVersionKind().GroupKind().Kind
		ns := obj.GetNamespace()
		if ns == "" {
			ns = "default"
		}

		lgr := lgr.With("kind", kind, "name", obj.GetName(), "namespace", ns)
		lgr.Info("checking stability of " + kind + "/" + obj.GetName())

		switch {
		case slices.Contains(workloadKinds, kind):
			lgr.Info("checking rollout status")
			if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
				Command: to.Ptr(fmt.Sprintf("kubectl rollout status %s/%s -n %s", kind, obj.GetName(), ns)),
			}); err != nil {
				return fmt.Errorf("waiting for %s/%s to be stable: %w", kind, obj.GetName(), err)
			}
		case kind == "Pod":
			lgr.Info("waiting for pod to be ready")
			if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
				Command: to.Ptr(fmt.Sprintf("kubectl wait --for=condition=Ready pod/%s -n %s", obj.GetName(), ns)),
			}); err != nil {
				return fmt.Errorf("waiting for pod/%s to be stable: %w", obj.GetName(), err)
			}
		case kind == "Job":
			lgr.Info("waiting for job complete")
			if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
				Command: to.Ptr(fmt.Sprintf("kubectl logs --pod-running-timeout=20s --follow job/%s -n %s", obj.GetName(), ns)),
			}); err != nil {
				return fmt.Errorf("waiting for job/%s to complete: %w", obj.GetName(), err)
			}
		}
	}

	return nil
}

func (a *aks) runCommand(ctx context.Context, request armcontainerservice.RunCommandRequest) error {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup, "command", *request.Command)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to run command")
	defer lgr.Info("finished running command")

	cred, err := getAzCred()
	if err != nil {
		return fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armcontainerservice.NewManagedClustersClient(a.subscriptionId, cred, nil)
	if err != nil {
		return fmt.Errorf("creating aks client: %w", err)
	}

	poller, err := client.BeginRunCommand(ctx, a.resourceGroup, a.name, request, nil)
	if err != nil {
		return fmt.Errorf("starting run command: %w", err)
	}

	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("running command: %w", err)
	}

	lgr.Info("command output: " + *result.Properties.Logs)
	if *result.Properties.ExitCode != 0 {
		return fmt.Errorf("command failed with exit code %d", *result.Properties.ExitCode)
	}

	return nil
}

func (a *aks) GetCluster(ctx context.Context) (*armcontainerservice.ManagedCluster, error) {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to get aks")
	defer lgr.Info("finished getting aks")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armcontainerservice.NewManagedClustersClient(a.subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating aks client: %w", err)
	}

	result, err := client.Get(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, fmt.Errorf("getting cluster: %w", err)
	}

	return &result.ManagedCluster, nil
}

func (a *aks) GetVnetId(ctx context.Context) (string, error) {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to get vnet id for aks")
	defer lgr.Info("finished getting vnet id for aks")

	cred, err := getAzCred()
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

func (a *aks) GetId() string {
	return a.id
}
