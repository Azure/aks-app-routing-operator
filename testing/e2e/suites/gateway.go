package suites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// gatewayTestNamespace is the namespace used for gateway tests
	gatewayTestNamespace = "wi-ns"
	// gatewayTestServiceAccount is the service account used for gateway tests
	gatewayTestServiceAccount = "wi-sa"
)

// TODO: Add e2e test for multi-tenant zone sharing scenario where multiple namespace-scoped
// ExternalDNS resources (across different namespaces) share the same DNS zone. This validates
// the expected behavior that:
// - Multiple ExternalDNS (namespace-scoped) CRDs can share a zone (multi-tenant use case)
// - ClusterExternalDNS claims exclusive ownership of a zone (no sharing allowed)

// isGatewayCluster checks if the provisioned infrastructure has Gateway API and Istio enabled
func isGatewayCluster(in infra.Provisioned) bool {
	opts := in.Cluster.GetOptions()
	_, hasIstio := opts[clients.IstioServiceMeshOpt.Name]
	_, hasGateway := opts[clients.ManagedGatewayOpt.Name]
	return hasIstio && hasGateway
}

func gatewayTests(in infra.Provisioned) []test {
	// Only run gateway tests on clusters with Gateway API and Istio enabled
	if !isGatewayCluster(in) {
		return []test{}
	}

	return []test{
		{
			name: "gateway with externaldns",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting gateway with externaldns test")

				cl, err := client.New(config, client.Options{
					Scheme: scheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				// Create namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: gatewayTestNamespace,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
				}
				if err := upsert(ctx, cl, ns); err != nil {
					return fmt.Errorf("upserting namespace: %w", err)
				}

				// Create ServiceAccount with workload identity
				sa := &corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      gatewayTestServiceAccount,
						Namespace: gatewayTestNamespace,
						Annotations: map[string]string{
							"azure.workload.identity/client-id": in.ManagedIdentity.GetClientID(),
						},
						Labels: map[string]string{
							"azure.workload.identity/use": "true",
						},
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "ServiceAccount",
						APIVersion: "v1",
					},
				}
				if err := upsert(ctx, cl, sa); err != nil {
					return fmt.Errorf("creating service account: %w", err)
				}

				// ========================================
				// Test 1: Cluster-scoped ExternalDNS
				// ========================================
				lgr.Info("testing cluster-scoped externaldns")

				// Create ClusterExternalDNS for public zone
				clusterExternalDns := &v1alpha1.ClusterExternalDNS{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gw-cluster-dns",
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterExternalDNS",
						APIVersion: v1alpha1.GroupVersion.String(),
					},
					Spec: v1alpha1.ClusterExternalDNSSpec{
						ResourceName:       "gw-cluster-external-dns",
						DNSZoneResourceIDs: []string{in.ManagedIdentityZone.Zone.GetId()},
						ResourceTypes:      []string{"gateway"},
						Identity: v1alpha1.ExternalDNSIdentity{
							ServiceAccount: sa.Name,
						},
						ResourceNamespace: gatewayTestNamespace,
					},
				}
				if err := upsert(ctx, cl, clusterExternalDns); err != nil {
					return fmt.Errorf("upserting cluster external dns: %w", err)
				}

				// Deploy Gateway resources and run test
				resources, err := deployGatewayResources(ctx, cl, in, in.ManagedIdentityZone)
				if err != nil {
					return fmt.Errorf("deploying gateway resources: %w", err)
				}

				// Wait for client deployment to be available (validates end-to-end connectivity)
				lgr.Info("waiting for client deployment to be available", "client", resources.Client.Name)
				if err := waitForAvailable(ctx, cl, *resources.Client); err != nil {
					return fmt.Errorf("waiting for client deployment: %w", err)
				}

				lgr.Info("cluster-scoped externaldns test passed, cleaning up gateway resources")

				// Delete Gateway and HTTPRoute first (triggers DNS record cleanup by external-dns)
				if err := cl.Delete(ctx, resources.Gateway); err != nil {
					return fmt.Errorf("deleting gateway: %w", err)
				}
				if err := cl.Delete(ctx, resources.HTTPRoute); err != nil {
					return fmt.Errorf("deleting httproute: %w", err)
				}

				// Wait for DNS A record to be deleted from Azure DNS zone
				// The record name is the subdomain part of the host (e.g., "wi-ns" for "wi-ns.zone.com")
				zoneName := in.ManagedIdentityZone.Zone.GetName()
				recordName := strings.ToLower(gatewayTestNamespace)
				lgr.Info("waiting for DNS record deletion", "zone", zoneName, "record", recordName)
				if err := waitForDNSRecordDeletion(ctx, in.SubscriptionId, in.ResourceGroup.GetName(), zoneName, recordName); err != nil {
					return fmt.Errorf("waiting for DNS record deletion: %w", err)
				}

				// Now delete the ClusterExternalDNS CRD
				lgr.Info("deleting cluster-scoped externaldns")
				if err := cl.Delete(ctx, clusterExternalDns); err != nil {
					return fmt.Errorf("deleting cluster external dns: %w", err)
				}

				// ========================================
				// Test 2: Namespace-scoped ExternalDNS
				// ========================================
				lgr.Info("testing namespace-scoped externaldns")

				// Create namespace-scoped ExternalDNS for public zone
				externalDns := &v1alpha1.ExternalDNS{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-ns-dns",
						Namespace: gatewayTestNamespace,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "ExternalDNS",
						APIVersion: v1alpha1.GroupVersion.String(),
					},
					Spec: v1alpha1.ExternalDNSSpec{
						ResourceName:       "gw-ns-external-dns",
						DNSZoneResourceIDs: []string{in.ManagedIdentityZone.Zone.GetId()},
						ResourceTypes:      []string{"gateway"},
						Identity: v1alpha1.ExternalDNSIdentity{
							ServiceAccount: sa.Name,
						},
					},
				}
				if err := upsert(ctx, cl, externalDns); err != nil {
					return fmt.Errorf("upserting namespace-scoped external dns: %w", err)
				}

				// Deploy new Gateway resources for namespace-scoped test
				resources2, err := deployGatewayResources(ctx, cl, in, in.ManagedIdentityZone)
				if err != nil {
					return fmt.Errorf("deploying gateway resources for ns-scoped test: %w", err)
				}

				// Wait for client deployment to be available
				lgr.Info("waiting for client deployment to be available", "client", resources2.Client.Name)
				if err := waitForAvailable(ctx, cl, *resources2.Client); err != nil {
					return fmt.Errorf("waiting for client deployment (ns-scoped): %w", err)
				}

				lgr.Info("finished gateway with externaldns test")
				return nil
			},
		},
	}
}

// deployGatewayResources creates Gateway API resources and returns them for later cleanup
func deployGatewayResources(
	ctx context.Context,
	cl client.Client,
	in infra.Provisioned,
	zoneWithCert infra.WithCert[infra.Zone],
) (*manifests.GatewayClientServerResources, error) {
	lgr := logger.FromContext(ctx)

	zone := zoneWithCert.Zone
	zoneName := zone.GetName()
	nameserver := zone.GetNameservers()[0]

	// Build hostname from namespace and zone
	host := strings.ToLower(gatewayTestNamespace) + "." + strings.TrimRight(zoneName, ".")
	tlsHost := host
	keyvaultURI := zoneWithCert.Cert.GetId()

	lgr.Info("deploying gateway resources", "host", host, "zone", zoneName)

	// Create Gateway API resources
	resources := manifests.GatewayClientAndServer(
		gatewayTestNamespace,
		zoneName,
		nameserver,
		keyvaultURI,
		host,
		tlsHost,
		gatewayTestServiceAccount,
		manifests.IstioGatewayClassName,
	)

	// Deploy all resources
	for _, obj := range resources.Objects() {
		if err := upsert(ctx, cl, obj); err != nil {
			return nil, fmt.Errorf("upserting resource %s: %w", obj.GetName(), err)
		}
	}

	return &resources, nil
}

// waitForDNSRecordDeletion waits for the DNS A record to be deleted from the Azure DNS zone
func waitForDNSRecordDeletion(ctx context.Context, subscriptionId, resourceGroup, zoneName, recordName string) error {
	lgr := logger.FromContext(ctx).With("zone", zoneName, "record", recordName)

	recordSetsClient, err := clients.NewRecordSetsClient(subscriptionId, resourceGroup, zoneName)
	if err != nil {
		return fmt.Errorf("creating record sets client: %w", err)
	}

	err = wait.PollImmediate(5*time.Second, 3*time.Minute, func() (bool, error) {
		_, err := recordSetsClient.GetARecord(ctx, recordName)
		if err != nil {
			// Check if it's a not found error (404)
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound {
				lgr.Info("DNS A record deleted")
				return true, nil
			}
			return false, fmt.Errorf("getting DNS record: %w", err)
		}
		lgr.Info("waiting for DNS A record to be deleted")
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for DNS A record %q to be deleted from zone %q: %w", recordName, zoneName, err)
	}
	return nil
}
