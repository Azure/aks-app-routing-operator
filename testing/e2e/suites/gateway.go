package suites

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/dns"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
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
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}). // TODO - variable dns zone counts for MI zones too
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
						ResourceName:       "gw-cluster",
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

				if err := cleanupResources(ctx, config, resources, in, clusterExternalDns); err != nil {
					return fmt.Errorf("cleaning up gateway resources: %w", err)
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

				if err := cleanupResources(ctx, config, resources2, in, externalDns); err != nil {
					return fmt.Errorf("cleaning up gateway resources (ns-scoped): %w", err)
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

// waitForDNSRecordDeletion waits for the external-dns pod to log that it deleted the DNS A record
// deploymentName is the name of the external-dns deployment (e.g., "gw-cluster-external-dns")
// namespace is the namespace where the deployment is located
func waitForDNSRecordDeletion(ctx context.Context, config *rest.Config, deploymentName, namespace, zoneName, recordName string) error {
	lgr := logger.FromContext(ctx).With("zone", zoneName, "record", recordName, "deployment", deploymentName, "namespace", namespace)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	// The expected log message from external-dns when deleting an A record
	expectedLogMessage := fmt.Sprintf("Deleting A record named '%s' for Azure DNS zone '%s'", recordName, zoneName)
	lgr.Info("waiting for external-dns to log deletion", "expectedMessage", expectedLogMessage)

	err = wait.PollImmediate(5*time.Second, 3*time.Minute, func() (bool, error) {
		// Find the external-dns pods by the deployment's label selector (app=deploymentName)
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", deploymentName),
		})
		if err != nil {
			return false, fmt.Errorf("listing external-dns pods: %w", err)
		}

		if len(pods.Items) == 0 {
			lgr.Info("no external-dns pods found, retrying")
			return false, nil
		}

		// Check logs from each pod
		for _, pod := range pods.Items {
			req := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
			logs, err := req.Stream(ctx)
			if err != nil {
				lgr.Info("failed to get pod logs", "pod", pod.Name, "error", err)
				continue
			}

			scanner := bufio.NewScanner(logs)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, expectedLogMessage) {
					logs.Close()
					lgr.Info("found DNS deletion log entry", "pod", pod.Name)
					return true, nil
				}
			}
			logs.Close()
		}

		lgr.Info("DNS deletion log entry not found yet, retrying")
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for DNS A record '%s' deletion log in zone '%s': %w", recordName, zoneName, err)
	}
	return nil
}

func cleanupResources(ctx context.Context, config *rest.Config, resources *manifests.GatewayClientServerResources, in infra.Provisioned, dnsResource dns.ExternalDNSCRDConfiguration, otherObjects ...client.Object) error {
	lgr := logger.FromContext(ctx)
	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Delete Gateway and HTTPRoute first (triggers DNS record cleanup by external-dns)
	if err := cl.Delete(ctx, resources.Gateway); err != nil {
		return fmt.Errorf("deleting gateway: %w", err)
	}
	if err := cl.Delete(ctx, resources.HTTPRoute); err != nil {
		return fmt.Errorf("deleting httproute: %w", err)
	}

	// Wait for external-dns to log that it deleted the DNS A record
	// The record name is the subdomain part of the host (e.g., "wi-ns" for "wi-ns.zone.com")
	zoneName := in.ManagedIdentityZone.Zone.GetName()
	recordName := strings.ToLower(gatewayTestNamespace)
	// The deployment name is {CRD.Spec.ResourceName}-external-dns
	externalDnsDeploymentName := dnsResource.GetInputResourceName() + "-external-dns"
	lgr.Info("waiting for DNS record deletion", "zone", zoneName, "record", recordName, "deployment", externalDnsDeploymentName)
	if err := waitForDNSRecordDeletion(ctx, config, externalDnsDeploymentName, gatewayTestNamespace, zoneName, recordName); err != nil {
		return fmt.Errorf("waiting for DNS record deletion: %w", err)
	}

	if err := cl.Delete(ctx, dnsResource); err != nil {
		return fmt.Errorf("cleaning up external dns CRD: %w", err)
	}

	// Now delete other objects
	for _, obj := range otherObjects {
		lgr.Info("deleting resource", "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind)
		if err := cl.Delete(ctx, obj); err != nil {
			return fmt.Errorf("deleting resource %s: %w", obj.GetName(), err)
		}
	}

	return nil
}
