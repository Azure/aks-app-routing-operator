package clients

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v8"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/go-autorest/autorest/azure"
)

var (
	// https://kubernetes.io/docs/concepts/workloads/
	// more specifically, these are compatible with kubectl rollout status
	workloadKinds = []string{"Deployment", "StatefulSet", "DaemonSet"}

	nonZeroExitCode = errors.New("non-zero exit code")
)

type aks struct {
	name, subscriptionId, resourceGroup string
	id                                  string
	dnsServiceIp                        string
	location                            string
	principalId                         string
	clientId                            string
	oidcUrl                             string
	options                             map[string]struct{}
}

// ServicePrincipal represents all the information needed to use a service principal including
// a fresh set of credentials and the associated application and service principal object ids.
// This representation is intended as read-only as in most cases only one ID is needed to retrieve
// the rest of the information for testing purposes.
type ServicePrincipal struct {
	// ApplicationObjectID is Object ID of the application associated with the service principal
	ApplicationObjectID string
	// ApplicationClientID is the Client ID of the application and service principal (also called AppID of the service principal)
	ApplicationClientID string
	// ServicePrincipalObjectID is Object ID of the service principal
	ServicePrincipalObjectID string
	// ServicePrincipalCredPassword is a generated password credential for the application associated with the service principal
	ServicePrincipalCredPassword string
}

// McOpt specifies what kind of managed cluster to create
type McOpt struct {
	Name string
	fn   func(mc *armcontainerservice.ManagedCluster) error
}

// PrivateClusterOpt specifies that the cluster should be private
var PrivateClusterOpt = McOpt{
	Name: "private cluster",
	fn: func(mc *armcontainerservice.ManagedCluster) error {
		if mc.Properties == nil {
			mc.Properties = &armcontainerservice.ManagedClusterProperties{}
		}

		if mc.Properties.APIServerAccessProfile == nil {
			mc.Properties.APIServerAccessProfile = &armcontainerservice.ManagedClusterAPIServerAccessProfile{}
		}

		mc.Properties.APIServerAccessProfile.EnablePrivateCluster = to.Ptr(true)
		return nil
	},
}

var OsmClusterOpt = McOpt{
	Name: "osm cluster",
	fn: func(mc *armcontainerservice.ManagedCluster) error {
		if mc.Properties.AddonProfiles == nil {
			mc.Properties.AddonProfiles = map[string]*armcontainerservice.ManagedClusterAddonProfile{}
		}

		mc.Properties.AddonProfiles["openServiceMesh"] = &armcontainerservice.ManagedClusterAddonProfile{
			Enabled: to.Ptr(true),
		}

		return nil
	},
}

func VmCountOpt(count int32) McOpt {
	return McOpt{
		Name: fmt.Sprintf("%d virtual machines", count),
		fn: func(mc *armcontainerservice.ManagedCluster) error {
			mc.Properties.AgentPoolProfiles[0].Count = to.Ptr(count)
			return nil
		},
	}
}

// IstioServiceMeshOpt enables the Istio service mesh addon on the cluster
var IstioServiceMeshOpt = McOpt{
	Name: "istio service mesh",
	fn: func(mc *armcontainerservice.ManagedCluster) error {
		if mc.Properties == nil {
			mc.Properties = &armcontainerservice.ManagedClusterProperties{}
		}

		mc.Properties.ServiceMeshProfile = &armcontainerservice.ServiceMeshProfile{
			Mode:  to.Ptr(armcontainerservice.ServiceMeshModeIstio),
			Istio: &armcontainerservice.IstioServiceMesh{},
		}

		return nil
	},
}

// ManagedGatewayOpt enables the managed Gateway API on the cluster
var ManagedGatewayOpt = McOpt{
	Name: "managed gateway api",
	fn: func(mc *armcontainerservice.ManagedCluster) error {
		if mc.Properties == nil {
			mc.Properties = &armcontainerservice.ManagedClusterProperties{}
		}

		if mc.Properties.IngressProfile == nil {
			mc.Properties.IngressProfile = &armcontainerservice.ManagedClusterIngressProfile{}
		}

		mc.Properties.IngressProfile.GatewayAPI = &armcontainerservice.ManagedClusterIngressProfileGatewayConfiguration{
			Installation: to.Ptr(armcontainerservice.ManagedGatewayTypeStandard),
		}

		return nil
	},
}

// AppRoutingIstioOpt marks the cluster as using the approuting-istio GatewayClass.
// The actual enablement is done post-creation via EnableAppRoutingIstio since the
// SDK does not yet have the types for the 2026-01-02-preview API.
// This is mutually exclusive with IstioServiceMeshOpt.
var AppRoutingIstioOpt = McOpt{
	Name: "approuting-istio",
	fn: func(mc *armcontainerservice.ManagedCluster) error {
		// No SDK struct mutation — the feature is enabled post-creation via raw REST API.
		return nil
	},
}

// armHTTPClient is an HTTP client with a longer timeout for ARM REST API calls.
// ARM operations (especially PUT for cluster updates) can take a long time.
var armHTTPClient = &http.Client{
	Timeout: 10 * time.Minute,
}

// doWithRetry performs an HTTP request with retries for transient errors (timeouts, connection resets, 429s, 5xx).
// It uses exponential backoff starting at initialBackoff and doubling each attempt up to maxRetries.
func doWithRetry(ctx context.Context, lgr *slog.Logger, buildReq func() (*http.Request, error), maxRetries int, initialBackoff time.Duration) (*http.Response, error) {
	backoff := initialBackoff
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			lgr.Info(fmt.Sprintf("retrying request (attempt %d/%d) after %s", attempt, maxRetries, backoff))
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := buildReq()
		if err != nil {
			return nil, fmt.Errorf("building request: %w", err)
		}

		resp, err := armHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			lgr.Info(fmt.Sprintf("request failed (attempt %d/%d): %v", attempt, maxRetries, err))
			continue
		}

		// Retry on 429 (Too Many Requests) and 5xx server errors
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("retryable status %d: %s", resp.StatusCode, string(body))
			lgr.Info(fmt.Sprintf("retryable status %d (attempt %d/%d)", resp.StatusCode, attempt, maxRetries))
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("all %d retries exhausted: %w", maxRetries, lastErr)
}

// EnableAppRoutingIstio enables the approuting-istio GatewayClass on an existing AKS cluster
// by doing a GET-modify-PUT via the raw ARM REST API with api-version=2026-01-02-preview.
// A PUT (CreateOrUpdate) is required because the PATCH endpoint only accepts TagsObject.
// This is necessary because the published SDK does not yet include the GatewayAPIImplementations types.
func EnableAppRoutingIstio(ctx context.Context, subscriptionId, resourceGroup, clusterName string) error {
	lgr := logger.FromContext(ctx).With("name", clusterName, "resourceGroup", resourceGroup)
	lgr.Info("enabling approuting-istio on cluster")
	defer lgr.Info("finished enabling approuting-istio on cluster")

	cred, err := getAzCred()
	if err != nil {
		return fmt.Errorf("getting az credentials: %w", err)
	}

	// Get a token for ARM
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return fmt.Errorf("getting ARM token: %w", err)
	}

	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s?api-version=2026-01-02-preview",
		subscriptionId, resourceGroup, clusterName,
	)

	// Step 1: GET the full cluster object
	lgr.Info("fetching current cluster state")
	getResp, err := doWithRetry(ctx, lgr, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token.Token)
		return req, nil
	}, 3, 10*time.Second)
	if err != nil {
		return fmt.Errorf("sending GET request: %w", err)
	}

	var cluster map[string]any
	if err := json.NewDecoder(getResp.Body).Decode(&cluster); err != nil {
		getResp.Body.Close()
		return fmt.Errorf("decoding GET response: %w", err)
	}
	getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d getting cluster", getResp.StatusCode)
	}

	// Step 2: Merge in the webAppRouting configuration
	props, _ := cluster["properties"].(map[string]any)
	if props == nil {
		return fmt.Errorf("cluster properties is nil")
	}

	ingressProfile, _ := props["ingressProfile"].(map[string]any)
	if ingressProfile == nil {
		ingressProfile = map[string]any{}
		props["ingressProfile"] = ingressProfile
	}

	ingressProfile["webAppRouting"] = map[string]any{
		"gatewayAPIImplementations": map[string]any{
			"appRoutingIstio": map[string]any{
				"mode": "Enabled",
			},
		},
	}

	// Step 3: PUT the modified cluster back
	bodyBytes, err := json.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("marshaling request body: %w", err)
	}

	lgr.Info(fmt.Sprintf("PUT URL: %s", url))

	putResp, err := doWithRetry(ctx, lgr, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token.Token)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, 3, 30*time.Second)
	if err != nil {
		return fmt.Errorf("sending PUT request: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("unexpected status %d enabling approuting-istio: %s", putResp.StatusCode, string(respBody))
	}

	lgr.Info(fmt.Sprintf("PUT response status: %d", putResp.StatusCode))

	// Step 4: Poll for completion by waiting for provisioning state
	lgr.Info("waiting for approuting-istio enablement to complete")
	for {
		time.Sleep(30 * time.Second)

		pollResp, err := doWithRetry(ctx, lgr, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token.Token)
			return req, nil
		}, 3, 10*time.Second)
		if err != nil {
			return fmt.Errorf("polling cluster state: %w", err)
		}

		var result map[string]any
		if err := json.NewDecoder(pollResp.Body).Decode(&result); err != nil {
			pollResp.Body.Close()
			return fmt.Errorf("decoding poll response: %w", err)
		}
		pollResp.Body.Close()

		pollProps, _ := result["properties"].(map[string]any)
		state, _ := pollProps["provisioningState"].(string)
		lgr.Info(fmt.Sprintf("cluster provisioning state: %s", state))

		if state == "Succeeded" {
			// Verify the feature was actually enabled
			ip, _ := pollProps["ingressProfile"].(map[string]any)
			war, _ := ip["webAppRouting"].(map[string]any)
			gwImpl, _ := war["gatewayAPIImplementations"].(map[string]any)
			ariSection, _ := gwImpl["appRoutingIstio"].(map[string]any)
			mode, _ := ariSection["mode"].(string)

			ingressProfileRaw, _ := json.MarshalIndent(ip, "", "  ")
			lgr.Info(fmt.Sprintf("ingressProfile from ARM response: %s", string(ingressProfileRaw)))

			if mode != "Enabled" {
				return fmt.Errorf("approuting-istio was not enabled correctly: expected mode \"Enabled\", got %q (ingressProfile: %s)", mode, string(ingressProfileRaw))
			}
			lgr.Info("verified approuting-istio is enabled")
			break
		}
		if state == "Failed" || state == "Canceled" {
			return fmt.Errorf("cluster provisioning failed with state: %s", state)
		}
	}

	return nil
}

func LoadAks(id azure.Resource, dnsServiceIp, location, principalId, clientId, oidcUrl string, options map[string]struct{}) *aks {
	return &aks{
		name:           id.ResourceName,
		subscriptionId: id.SubscriptionID,
		resourceGroup:  id.ResourceGroup,
		id:             id.String(),
		clientId:       clientId,
		dnsServiceIp:   dnsServiceIp,
		location:       location,
		principalId:    principalId,
		oidcUrl:        oidcUrl,
		options:        options,
	}
}

// NewAks creates a new AKS cluster
// spOpts is optional, if nil then the cluster will use MSI
func NewAks(ctx context.Context, subscriptionId, resourceGroup, name, location string, spOpts *ServicePrincipal, mcOpts ...McOpt) (*aks, error) {
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
		Properties: &armcontainerservice.ManagedClusterProperties{
			DNSPrefix:         to.Ptr("approutinge2e"),
			NodeResourceGroup: to.Ptr(truncate("MC_"+name, 80)),
			AgentPoolProfiles: []*armcontainerservice.ManagedClusterAgentPoolProfile{
				{
					Name:   to.Ptr("default"),
					VMSize: to.Ptr("Standard_DS3_v2"),
					Count:  to.Ptr(int32(5)),
					Mode:   to.Ptr(armcontainerservice.AgentPoolModeSystem),
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
			OidcIssuerProfile: &armcontainerservice.ManagedClusterOIDCIssuerProfile{
				Enabled: to.Ptr(true),
			},
			SecurityProfile: &armcontainerservice.ManagedClusterSecurityProfile{
				WorkloadIdentity: &armcontainerservice.ManagedClusterSecurityProfileWorkloadIdentity{
					Enabled: to.Ptr(true),
				},
			},
		},
	}

	// apply service principal
	if spOpts != nil {
		mc.Properties.ServicePrincipalProfile = &armcontainerservice.ManagedClusterServicePrincipalProfile{
			ClientID: to.Ptr(spOpts.ApplicationClientID),
			Secret:   to.Ptr(spOpts.ServicePrincipalCredPassword),
		}
	} else {
		mc.Identity = &armcontainerservice.ManagedClusterIdentity{
			Type: to.Ptr(armcontainerservice.ResourceIdentityTypeSystemAssigned),
		}
	}

	options := make(map[string]struct{})
	for _, opt := range mcOpts {
		if err := opt.fn(&mc); err != nil {
			return nil, fmt.Errorf("applying cluster option: %w", err)
		}

		options[opt.Name] = struct{}{}
	}
	if mc.Properties.IdentityProfile != nil && mc.Properties.ServicePrincipalProfile != nil {
		return nil, fmt.Errorf("cluster has both identity profile and service principal profile, must only have one identity type")
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

	// guard against things that should be impossible
	if result.ManagedCluster.Properties == nil {
		return nil, fmt.Errorf("managed cluster properties is nil")
	}
	// cluster must use either MSI or Service Principal
	if result.ManagedCluster.Properties.IdentityProfile == nil && result.ManagedCluster.Properties.ServicePrincipalProfile == nil {
		return nil, fmt.Errorf("cluster has no identity type since identity profile and service principal profile are nil")
	}
	if result.ManagedCluster.Name == nil {
		return nil, fmt.Errorf("managed cluster name is nil")
	}
	if result.Properties.NetworkProfile.DNSServiceIP == nil {
		return nil, fmt.Errorf("dns service ip is nil")
	}

	if result.Properties.OidcIssuerProfile == nil {
		return nil, fmt.Errorf("oidc issuer profile is nil")
	}

	if result.Properties.OidcIssuerProfile.IssuerURL == nil {
		return nil, fmt.Errorf("oidc issuer url is nil")
	}

	// validate MSI when not using Service Principal
	var identity *armcontainerservice.UserAssignedIdentity
	var principalID, clientID string
	isMSICluster := spOpts == nil
	if isMSICluster {
		ok := false // avoid shadowing
		identity, ok = result.Properties.IdentityProfile["kubeletidentity"]
		if !ok {
			return nil, fmt.Errorf("kubelet identity not found")
		}
		if identity.ObjectID == nil {
			return nil, fmt.Errorf("kubelet identity object id is nil")
		}
		if identity.ClientID == nil {
			return nil, fmt.Errorf("kubelet identity client id is nil")
		}
		principalID = *identity.ObjectID
		clientID = *identity.ClientID
	} else {
		principalID = spOpts.ServicePrincipalObjectID
	}

	// final principal id validation to be safe
	if principalID == "" {
		return nil, fmt.Errorf("principal id is empty")
	}

	cluster := &aks{
		name:           *result.ManagedCluster.Name,
		subscriptionId: subscriptionId,
		resourceGroup:  resourceGroup,
		id:             *result.ManagedCluster.ID,
		dnsServiceIp:   *result.Properties.NetworkProfile.DNSServiceIP,
		location:       location,
		principalId:    principalID,
		clientId:       clientID,
		oidcUrl:        *result.Properties.OidcIssuerProfile.IssuerURL,
		options:        options,
	}

	return cluster, nil
}

func (a *aks) Deploy(ctx context.Context, objs []client.Object) error {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to deploy resources")
	defer lgr.Info("finished deploying resources")

	zip, err := zipManifests(objs)
	if err != nil {
		return fmt.Errorf("zipping manifests: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(zip)

	if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
		Command: to.Ptr("kubectl apply -f manifests/"),
		Context: &encoded,
	}, runCommandOpts{}); err != nil {
		return fmt.Errorf("running kubectl apply: %w", err)
	}

	if err := a.waitStable(ctx, objs); err != nil {
		return fmt.Errorf("waiting for resources to be stable: %w", err)
	}

	return nil
}

// zipFile reads a file from the local filesystem and wraps it into a zip file.
// The file is placed at the root of the zip with a fixed name for kubectl to reference.
func zipFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", path, err)
	}

	b := &bytes.Buffer{}
	zipWriter := zip.NewWriter(b)
	f, err := zipWriter.Create("gateway-crd.yaml")
	if err != nil {
		return nil, fmt.Errorf("creating zip entry: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		return nil, fmt.Errorf("writing zip entry: %w", err)
	}
	zipWriter.Close()
	return b.Bytes(), nil
}

// zipManifests wraps manifests into base64 zip file.
// this is specified by the AKS ARM API.
// https://github.com/FumingZhang/azure-cli/blob/aefcf3948ed4207bfcf5d53064e5dac8ea8f19ca/src/azure-cli/azure/cli/command_modules/acs/custom.py#L2750
func zipManifests(objs []client.Object) ([]byte, error) {
	b := &bytes.Buffer{}
	zipWriter := zip.NewWriter(b)
	for i, obj := range objs {
		json, err := manifests.MarshalJson(obj)
		if err != nil {
			return nil, fmt.Errorf("marshaling json for object: %w", err)
		}

		f, err := zipWriter.Create(fmt.Sprintf("manifests/%d.json", i))
		if err != nil {
			return nil, fmt.Errorf("creating zip entry: %w", err)
		}

		if _, err := f.Write(json); err != nil {
			return nil, fmt.Errorf("writing zip entry: %w", err)
		}
	}
	zipWriter.Close()
	return b.Bytes(), nil
}

func (a *aks) Clean(ctx context.Context, objs []client.Object) error {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to clean resources")
	defer lgr.Info("finished cleaning resources")

	zip, err := zipManifests(objs)
	if err != nil {
		return fmt.Errorf("zipping manifests: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(zip)

	if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
		Command: to.Ptr("kubectl delete -f manifests/ --ignore-not-found=true"),
		Context: &encoded,
	}, runCommandOpts{}); err != nil {
		return fmt.Errorf("running kubectl delete: %w", err)
	}

	return nil
}

func (a *aks) waitStable(ctx context.Context, objs []client.Object) error {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to wait for resources to be stable")
	defer lgr.Info("finished waiting for resources to be stable")

	var eg errgroup.Group
	for _, obj := range objs {
		func(obj client.Object) {
			eg.Go(func() error {
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
					}, runCommandOpts{}); err != nil {
						return fmt.Errorf("waiting for %s/%s to be stable: %w", kind, obj.GetName(), err)
					}
				case kind == "Pod":
					lgr.Info("waiting for pod to be ready")
					if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
						Command: to.Ptr(fmt.Sprintf("kubectl wait --for=condition=Ready pod/%s -n %s", obj.GetName(), ns)),
					}, runCommandOpts{}); err != nil {
						return fmt.Errorf("waiting for pod/%s to be stable: %w", obj.GetName(), err)
					}
				case kind == "Job":
					lgr.Info("waiting for job complete")

					outputFile := fmt.Sprintf("job-%s.log", obj.GetName()) // output to a file for jobs because jobs are naturally different from other deployment resources in that waiting for "stability" is waiting for them to complete
					if err := os.RemoveAll(outputFile); err != nil {       // clean out previous log file, if doesn't exist returns nil
						return fmt.Errorf("removing previous job log file: %w", err)
					}

					getLogsFn := func() error { // right now this just dumps all logs on the pod, if we eventually have more logs
						// than can be stored we will need to "stream" this by using the --since-time flag
						if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
							Command: to.Ptr(fmt.Sprintf("kubectl logs job/%s -n %s", obj.GetName(), ns)),
						}, runCommandOpts{
							outputFile: outputFile,
						}); err != nil {
							return fmt.Errorf("getting logs for for job/%s: %w", obj.GetName(), err)
						}

						return nil
					}

					// invoke command jobs are supposed to be short-lived, so we have to constantly poll for completion
					for {
						// check if job is complete
						if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
							Command: to.Ptr(fmt.Sprintf("kubectl wait --for=condition=complete --timeout=5s job/%s -n %s", obj.GetName(), ns)),
						}, runCommandOpts{}); err == nil {
							break // job is complete
						} else {
							if !errors.Is(err, nonZeroExitCode) { // if the job is not complete, we will get a non-zero exit code
								getLogsFn()
								return fmt.Errorf("waiting for job/%s to complete: %w", obj.GetName(), err)
							}
						}

						// check if job is failed
						if err := a.runCommand(ctx, armcontainerservice.RunCommandRequest{
							Command: to.Ptr(fmt.Sprintf("kubectl wait --for=condition=failed --timeout=5s job/%s -n %s", obj.GetName(), ns)),
						}, runCommandOpts{}); err == nil {
							getLogsFn()
							return fmt.Errorf("job/%s failed", obj.GetName())
						}
					}

					if err := getLogsFn(); err != nil {
						return fmt.Errorf("getting logs for job/%s: %w", obj.GetName(), err)
					}
				}

				return nil
			})
		}(obj)
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("waiting for resources to be stable: %w", err)
	}

	return nil
}

type runCommandOpts struct {
	// outputFile is the file to write the output of the command to. Useful for saving logs from a job or something similar
	// where there's lots of logs that are extremely important and shouldn't be muddled up in the rest of the logs.
	outputFile string
}

func (a *aks) runCommand(ctx context.Context, request armcontainerservice.RunCommandRequest, opt runCommandOpts) error {
	lgr := logger.FromContext(ctx).With("name", a.name, "resourceGroup", a.resourceGroup, "command", *request.Command)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to run command")
	defer lgr.Info("finished running command")

	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt*5) * time.Second
			lgr.Info(fmt.Sprintf("retrying run command after transient error (attempt %d/%d, waiting %s)", attempt+1, maxRetries, delay))
			time.Sleep(delay)
		}

		err := a.runCommandOnce(ctx, lgr, request, opt)
		if err == nil {
			return nil
		}

		// non-zero exit code errors are not transient, return immediately
		if errors.Is(err, nonZeroExitCode) {
			return err
		}

		lastErr = err
		lgr.Info(fmt.Sprintf("run command attempt %d/%d failed with transient error: %s", attempt+1, maxRetries, err))
	}

	return lastErr
}

func (a *aks) runCommandOnce(ctx context.Context, lgr *slog.Logger, request armcontainerservice.RunCommandRequest, opt runCommandOpts) error {
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

	logs := ""
	if result.Properties != nil && result.Properties.Logs != nil {
		logs = *result.Properties.Logs
	}
	if opt.outputFile != "" {
		outputFile, err := os.OpenFile(opt.outputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("creating output file %s: %w", opt.outputFile, err)
		}
		defer outputFile.Close()

		_, err = outputFile.WriteString(logs)
		if err != nil {
			return fmt.Errorf("writing output file %s: %w", opt.outputFile, err)
		}
	} else {
		lgr.Info("command output: " + logs)
	}

	if *result.Properties.ExitCode != 0 {
		lgr.Info(fmt.Sprintf("command failed with exit code %d", *result.Properties.ExitCode))
		lgr.Info("command logs: " + logs)
		return fmt.Errorf("%w: %s", nonZeroExitCode, logs)
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

func (a *aks) GetPrincipalId() string {
	return a.principalId
}

func (a *aks) GetLocation() string {
	return a.location
}

func (a *aks) GetDnsServiceIp() string {
	return a.dnsServiceIp
}

func (a *aks) GetClientId() string {
	return a.clientId
}

func (a *aks) GetOidcUrl() string {
	return a.oidcUrl
}

func (a *aks) GetOptions() map[string]struct{} {
	return a.options
}
