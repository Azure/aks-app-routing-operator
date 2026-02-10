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
	// gatewayPublicTestNamespace is the namespace used for gateway tests for public dns zones
	gatewayPublicTestNamespace = "gateway-wi-ns"
	// gatewayPrivateTestNamespace is the namespace used for gateway tests for private dns zones
	gatewayPrivateTestNamespace = "private-gateway-wi-ns"
	// gatewayTestServiceAccount is the service account used for gateway tests
	gatewayTestServiceAccount = "gateway-wi-sa"
)

type gatewayTestConfig struct {
	namespace  string
	clientId   string
	zoneConfig gatewayZoneConfig
}

// gatewayZoneConfig contains zone-specific configuration for gateway tests.
// This abstraction allows the same test logic to work with both public and private DNS zones.
type gatewayZoneConfig struct {
	// ZoneID is the Azure resource ID of the DNS zone
	ZoneID string
	// ZoneName is the DNS zone domain name (e.g., "mi-zone-abc123.com")
	ZoneName string
	// Nameserver is the DNS server for resolution
	// - Public zones: zone's authoritative nameserver
	// - Private zones: cluster's DNS service IP (CoreDNS)
	Nameserver string
	// KeyvaultCertURI is the Azure Key Vault certificate URI for TLS
	KeyvaultCertURI string
	// NamePrefix is used to create unique resource names (e.g., "public" or "private")
	// This ensures resources don't collide when tests run concurrently
	NamePrefix string
}

// newPublicZoneConfig creates a gatewayZoneConfig for a public DNS zone
func newPublicZoneConfig(zone infra.WithCert[infra.Zone]) gatewayZoneConfig {
	return gatewayZoneConfig{
		ZoneID:          zone.Zone.GetId(),
		ZoneName:        zone.Zone.GetName(),
		Nameserver:      zone.Zone.GetNameservers()[0],
		KeyvaultCertURI: zone.Cert.GetId(),
	}
}

// newPrivateZoneConfig creates a gatewayZoneConfig for a private DNS zone
func newPrivateZoneConfig(zone infra.WithCert[infra.PrivateZone], dnsServiceIP string) gatewayZoneConfig {
	return gatewayZoneConfig{
		ZoneID:          zone.Zone.GetId(),
		ZoneName:        zone.Zone.GetName(),
		Nameserver:      dnsServiceIP,
		KeyvaultCertURI: zone.Cert.GetId(),
		NamePrefix:      "private-",
	}
}

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
			name: "gateway with externaldns for public zone",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				withGatewayTLS(true).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				publicZoneConfig := newPublicZoneConfig(in.ManagedIdentityZone)
				publicGwTestConfig := gatewayTestConfig{
					namespace:  gatewayPublicTestNamespace,
					clientId:   in.ManagedIdentity.GetClientID(),
					zoneConfig: publicZoneConfig,
				}
				if err := runGatewayTests(ctx, config, in.ManagedIdentity.GetClientID(), publicGwTestConfig); err != nil {
					return err
				}
				return nil
			},
		},
		{
			name: "gateway with externaldns for private zone",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				privateZoneConfig := newPrivateZoneConfig(in.ManagedIdentityPrivateZone, in.Cluster.GetDnsServiceIp())
				privateGwTestConfig := gatewayTestConfig{
					namespace:  gatewayPrivateTestNamespace,
					clientId:   in.ManagedIdentity.GetClientID(),
					zoneConfig: privateZoneConfig,
				}
				if err := runGatewayTests(ctx, config, in.ManagedIdentity.GetClientID(), privateGwTestConfig); err != nil {
					return err
				}
				return nil
			},
		},
	}
}

func runGatewayTests(ctx context.Context, config *rest.Config, clientId string, gatewayTestConfig gatewayTestConfig) error {
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
			Name: gatewayTestConfig.namespace,
			Labels: map[string]string{
				manifests.ManagedByKey: manifests.ManagedByVal,
			},
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
			Namespace: gatewayTestConfig.namespace,
			Annotations: map[string]string{
				"azure.workload.identity/client-id": clientId,
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

	// Create ClusterExternalDNS for zone
	clusterExternalDns := &v1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: gatewayTestConfig.zoneConfig.NamePrefix + "gw-cluster-dns",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceName:       "gw-cluster",
			DNSZoneResourceIDs: []string{gatewayTestConfig.zoneConfig.ZoneID},
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: sa.Name,
			},
			ResourceNamespace: ns.Name,
		},
	}
	if err := upsert(ctx, cl, clusterExternalDns); err != nil {
		return fmt.Errorf("upserting cluster external dns: %w", err)
	}

	// Deploy Gateway resources and run test
	resources, err := deployGatewayResources(ctx, cl, gatewayTestConfig)
	if err != nil {
		return fmt.Errorf("deploying gateway resources: %w", err)
	}

	// Wait for client deployment to be available (validates end-to-end connectivity)
	lgr.Info("waiting for client deployment to be available", "client", resources.Client.Name)
	if err := waitForAvailable(ctx, cl, *resources.Client); err != nil {
		return fmt.Errorf("waiting for client deployment: %w", err)
	}

	lgr.Info("cluster-scoped externaldns test passed, cleaning up gateway resources")

	if err := cleanupResources(ctx, config, resources, gatewayTestConfig, clusterExternalDns); err != nil {
		return fmt.Errorf("cleaning up gateway resources: %w", err)
	}

	// ========================================
	// Test 2: Namespace-scoped ExternalDNS
	// ========================================
	lgr.Info("testing namespace-scoped externaldns")

	// Create namespace-scoped ExternalDNS for public zone
	externalDns := &v1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayTestConfig.zoneConfig.NamePrefix + "gw-ns-dns",
			Namespace: gatewayTestConfig.namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ExternalDNSSpec{
			ResourceName:       "gw-ns-external-dns",
			DNSZoneResourceIDs: []string{gatewayTestConfig.zoneConfig.ZoneID},
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
	resources2, err := deployGatewayResources(ctx, cl, gatewayTestConfig)
	if err != nil {
		return fmt.Errorf("deploying gateway resources for ns-scoped test: %w", err)
	}

	// Wait for client deployment to be available
	lgr.Info("waiting for client deployment to be available", "client", resources2.Client.Name)
	if err := waitForAvailable(ctx, cl, *resources2.Client); err != nil {
		return fmt.Errorf("waiting for client deployment (ns-scoped): %w", err)
	}

	if err := cleanupResources(ctx, config, resources2, gatewayTestConfig, externalDns); err != nil {
		return fmt.Errorf("cleaning up gateway resources (ns-scoped): %w", err)
	}

	lgr.Info("finished gateway with externaldns test")

	if err := runAllFilterTests(ctx, config, gatewayTestConfig); err != nil {
		return fmt.Errorf("running filter tests: %w", err)
	}

	return nil
}

// deployGatewayResources creates Gateway API resources and returns them for later cleanup
func deployGatewayResources(
	ctx context.Context,
	cl client.Client,
	gatewayTestConfig gatewayTestConfig,
) (*manifests.GatewayClientServerResources, error) {
	lgr := logger.FromContext(ctx)

	zoneName := gatewayTestConfig.zoneConfig.ZoneName
	nameserver := gatewayTestConfig.zoneConfig.Nameserver

	// Build hostname from namespace and zone
	host := strings.ToLower(gatewayTestConfig.namespace) + "." + strings.TrimSuffix(zoneName, ".")
	tlsHost := host
	keyvaultURI := gatewayTestConfig.zoneConfig.KeyvaultCertURI

	lgr.Info("deploying gateway resources", "host", host, "zone", zoneName)

	// Create Gateway API resources
	resources := manifests.GatewayClientAndServer(
		gatewayTestConfig.namespace,
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

// waitForDNSRecordDeletion waits for the external-dns pod to log that it deleted both the DNS A record
// and the corresponding TXT ownership record.
// deploymentName is the name of the external-dns deployment (e.g., "gw-cluster-external-dns")
// namespace is the namespace where the deployment is located
func waitForDNSRecordDeletion(ctx context.Context, config *rest.Config, deploymentName, namespace, zoneName, recordName string) error {
	lgr := logger.FromContext(ctx).With("zone", zoneName, "record", recordName, "deployment", deploymentName, "namespace", namespace)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	// The expected log messages from external-dns when deleting records
	// A record: "Deleting A record named 'gateway-wi-ns' for Azure DNS zone 'example.com'"
	// TXT record: "Deleting TXT record named 'a-gateway-wi-ns' for Azure DNS zone 'example.com'"
	// The TXT record name is prefixed with "a-" to indicate it's the ownership record for an A record
	expectedARecordLog := fmt.Sprintf("Deleting A record named '%s' for Azure DNS zone '%s'", recordName, zoneName)
	txtRecordName := "a-" + recordName // external-dns prefixes TXT ownership records with the record type
	expectedTXTRecordLog := fmt.Sprintf("Deleting TXT record named '%s' for Azure DNS zone '%s'", txtRecordName, zoneName)

	lgr.Info("waiting for external-dns to log deletion of A and TXT records",
		"expectedARecordLog", expectedARecordLog,
		"expectedTXTRecordLog", expectedTXTRecordLog)

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

		foundARecordDeletion := false
		foundTXTRecordDeletion := false

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
				if strings.Contains(line, expectedARecordLog) {
					foundARecordDeletion = true
				}
				if strings.Contains(line, expectedTXTRecordLog) {
					foundTXTRecordDeletion = true
				}
			}
			logs.Close()
		}

		if foundARecordDeletion && foundTXTRecordDeletion {
			lgr.Info("found DNS deletion log entries for both A and TXT records")
			return true, nil
		}

		// Log which specific deletion is missing
		if foundARecordDeletion && !foundTXTRecordDeletion {
			lgr.Info("found A record deletion but still waiting for TXT record deletion",
				"missingTXTRecord", txtRecordName)
		} else if !foundARecordDeletion && foundTXTRecordDeletion {
			lgr.Info("found TXT record deletion but still waiting for A record deletion",
				"missingARecord", recordName)
		} else {
			lgr.Info("waiting for both A and TXT record deletions")
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for DNS A record '%s' and TXT record '%s' deletion logs in zone '%s': %w", recordName, txtRecordName, zoneName, err)
	}
	return nil
}

func cleanupResources(ctx context.Context, config *rest.Config, resources *manifests.GatewayClientServerResources, gatewayTestConfig gatewayTestConfig, dnsResource dns.ExternalDNSCRDConfiguration, otherObjects ...client.Object) error {
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
	// The record name is the subdomain part of the host (e.g., "gateway-wi-ns" for "gateway-wi-ns.zone.com")
	zoneName := gatewayTestConfig.zoneConfig.ZoneName
	recordName := strings.ToLower(gatewayTestConfig.namespace)
	// The deployment name is {CRD.Spec.ResourceName}-external-dns
	externalDnsDeploymentName := dnsResource.GetInputResourceName() + "-external-dns"
	lgr.Info("waiting for DNS record deletion", "zone", zoneName, "record", recordName, "deployment", externalDnsDeploymentName)
	if err := waitForDNSRecordDeletion(ctx, config, externalDnsDeploymentName, gatewayTestConfig.namespace, zoneName, recordName); err != nil {
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
