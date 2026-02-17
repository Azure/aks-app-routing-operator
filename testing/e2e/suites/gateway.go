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
	"github.com/Azure/aks-app-routing-operator/testing/e2e/utils"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type zoneType int

const (
	zoneTypePublic zoneType = iota
	zoneTypePrivate
)

func (z zoneType) String() string {
	switch z {
	case zoneTypePrivate:
		return "Azure Private DNS zone"
	default:
		return "Azure DNS zone"
	}
}

func (z zoneType) Prefix() string {
	switch z {
	case zoneTypePrivate:
		return "private-"
	default:
		return ""
	}
}

// multiZoneGatewayTestConfig contains configuration for gateway tests with multiple zones
type multiZoneGatewayTestConfig struct {
	// clientId is the managed identity client ID for workload identity
	clientId string
	// zoneConfigs contains configuration for each zone to test
	zoneConfigs []gatewayZoneConfig
	// zoneType indicates whether these are public or private zones
	zoneType zoneType
}

// gatewayZoneConfig contains zone-specific configuration for gateway tests.
// This abstraction allows the same test logic to work with both public and private DNS zones.
type gatewayZoneConfig struct {
	// ZoneIndex is the index of this zone in the list (used for namespace naming)
	ZoneIndex int
	// ZoneID is the Azure resource ID of the DNS zone
	ZoneID string
	// ZoneName is the DNS zone domain name (e.g., "mi-zone-0-abc123.com")
	ZoneName string
	// Nameserver is the DNS server for resolution
	// - Public zones: zone's authoritative nameserver
	// - Private zones: cluster's DNS service IP (CoreDNS)
	Nameserver string
	// KeyvaultCertURI is the Azure Key Vault certificate URI for TLS
	KeyvaultCertURI string
}

// newPublicZoneConfig creates a gatewayZoneConfig for a public DNS zone
func newPublicZoneConfig(zone infra.WithCert[infra.Zone], index int) gatewayZoneConfig {
	return gatewayZoneConfig{
		ZoneIndex:       index,
		ZoneID:          zone.Zone.GetId(),
		ZoneName:        zone.Zone.GetName(),
		Nameserver:      zone.Zone.GetNameservers()[0],
		KeyvaultCertURI: zone.Cert.GetId(),
	}
}

// newPrivateZoneConfig creates a gatewayZoneConfig for a private DNS zone
func newPrivateZoneConfig(zone infra.WithCert[infra.PrivateZone], dnsServiceIP string, index int) gatewayZoneConfig {
	return gatewayZoneConfig{
		ZoneIndex:       index,
		ZoneID:          zone.Zone.GetId(),
		ZoneName:        zone.Zone.GetName(),
		Nameserver:      dnsServiceIP,
		KeyvaultCertURI: zone.Cert.GetId(),
	}
}

// buildPublicZoneConfigs creates gatewayZoneConfig entries for all public managed identity zones
func buildPublicZoneConfigs(in infra.Provisioned) []gatewayZoneConfig {
	configs := make([]gatewayZoneConfig, len(in.ManagedIdentityZones))
	for i, zone := range in.ManagedIdentityZones {
		configs[i] = newPublicZoneConfig(zone, i)
	}
	return configs
}

// buildPrivateZoneConfigs creates gatewayZoneConfig entries for all private managed identity zones
func buildPrivateZoneConfigs(in infra.Provisioned) []gatewayZoneConfig {
	configs := make([]gatewayZoneConfig, len(in.ManagedIdentityPrivateZones))
	for i, zone := range in.ManagedIdentityPrivateZones {
		configs[i] = newPrivateZoneConfig(zone, in.Cluster.GetDnsServiceIp(), i)
	}
	return configs
}

// getZoneIDs extracts all zone IDs from a list of zone configs
func getZoneIDs(configs []gatewayZoneConfig) []string {
	ids := make([]string, len(configs))
	for i, cfg := range configs {
		ids[i] = cfg.ZoneID
	}
	return ids
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
			name: "gateway with externaldns for public zones",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				withGatewayTLS(true).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				testConfig := multiZoneGatewayTestConfig{
					clientId:    in.ManagedIdentity.GetClientID(),
					zoneConfigs: buildPublicZoneConfigs(in),
					zoneType:    zoneTypePublic,
				}
				if err := runMultiZoneGatewayTests(ctx, config, testConfig); err != nil {
					return err
				}
				return nil
			},
		},
		{
			name: "gateway with externaldns for private zones",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				withGatewayTLS(true).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				testConfig := multiZoneGatewayTestConfig{
					clientId:    in.ManagedIdentity.GetClientID(),
					zoneConfigs: buildPrivateZoneConfigs(in),
					zoneType:    zoneTypePrivate,
				}
				if err := runMultiZoneGatewayTests(ctx, config, testConfig); err != nil {
					return err
				}
				return nil
			},
		},
	}
}

// runMultiZoneGatewayTests runs gateway tests with multiple DNS zones
// It tests both cluster-scoped and namespace-scoped ExternalDNS configurations
func runMultiZoneGatewayTests(ctx context.Context, config *rest.Config, testConfig multiZoneGatewayTestConfig) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting multi-zone gateway with externaldns test", "numZones", len(testConfig.zoneConfigs), "zoneType", testConfig.zoneType.String())

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// ========================================
	// Test 1: Cluster-scoped ExternalDNS (one namespace per zone)
	// ========================================
	lgr.Info("testing cluster-scoped externaldns with multiple zones")

	// Create namespaces and service accounts for each zone (cluster-scoped test)
	clusterTestNamespaces := make([]*corev1.Namespace, len(testConfig.zoneConfigs))
	clusterTestServiceAccounts := make([]*corev1.ServiceAccount, len(testConfig.zoneConfigs))

	for i, zoneCfg := range testConfig.zoneConfigs {
		nsName := utils.GatewayClusterNsName(zoneCfg.ZoneIndex)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
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
			return fmt.Errorf("upserting namespace %s: %w", nsName, err)
		}
		clusterTestNamespaces[i] = ns

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.GatewayClusterSaName,
				Namespace: nsName,
				Annotations: map[string]string{
					"azure.workload.identity/client-id": testConfig.clientId,
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
			return fmt.Errorf("creating service account in namespace %s: %w", nsName, err)
		}
		clusterTestServiceAccounts[i] = sa
	}

	// Use the first namespace as the resource namespace for ClusterExternalDNS
	resourceNamespace := clusterTestNamespaces[0].Name

	// Create single ClusterExternalDNS with all zone IDs
	clusterExternalDns := &v1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testConfig.zoneType.Prefix() + "gw-cluster-dns",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceName:       testConfig.zoneType.Prefix() + "gw-cluster",
			DNSZoneResourceIDs: getZoneIDs(testConfig.zoneConfigs),
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: clusterTestServiceAccounts[0].Name,
			},
			ResourceNamespace: resourceNamespace,
		},
	}
	if err := upsert(ctx, cl, clusterExternalDns); err != nil {
		return fmt.Errorf("upserting cluster external dns: %w", err)
	}

	// Deploy gateway resources for each zone in its respective namespace
	clusterResources := make([]manifests.ObjectsContainer, len(testConfig.zoneConfigs))
	clusterHostPrefixes := make([]string, len(testConfig.zoneConfigs)) // for DNS record verification during cleanup
	for i, zoneCfg := range testConfig.zoneConfigs {
		recordName := fmt.Sprintf("zone%d", zoneCfg.ZoneIndex)
		clusterHostPrefixes[i] = recordName
		tlsHost := fmt.Sprintf("%s.%s", recordName, strings.TrimSuffix(zoneCfg.ZoneName, "."))
		resources, err := deployGatewayResourcesForZone(ctx, cl, zoneCfg, clusterTestNamespaces[i].Name, clusterTestServiceAccounts[i].Name, testConfig.zoneType.Prefix(), tlsHost)
		if err != nil {
			return fmt.Errorf("deploying gateway resources for zone %d: %w", zoneCfg.ZoneIndex, err)
		}
		clusterResources[i] = resources
	}

	// Wait for all client deployments to be available in parallel
	eg, egCtx := errgroup.WithContext(ctx)
	for i, resources := range clusterResources {
		i, resources := i, resources // capture loop variables
		eg.Go(func() error {
			castedResources := resources.(*manifests.GatewayClientServerResources)
			lgr.Info("waiting for client deployment to be available", "client", castedResources.Client.Name, "zoneIndex", i)
			if err := waitForAvailable(egCtx, cl, *castedResources.Client); err != nil {
				return fmt.Errorf("waiting for client deployment (zone %d): %w", i, err)
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	lgr.Info("cluster-scoped externaldns test passed, cleaning up gateway resources")

	// Cleanup cluster-scoped test resources
	if err := cleanupMultiZoneResources(ctx, config, clusterResources, clusterExternalDns, testConfig, clusterHostPrefixes, clusterTestNamespaces[0].Name); err != nil {
		return fmt.Errorf("cleaning up cluster-scoped gateway resources: %w", err)
	}

	// ========================================
	// Test 2: Namespace-scoped ExternalDNS (all zones in single namespace)
	// ========================================
	lgr.Info("testing namespace-scoped externaldns with multiple zones")

	// Determine namespace based on zone type
	var nsName string
	if testConfig.zoneType == zoneTypePublic {
		nsName = utils.GatewayNsPublic
	} else {
		nsName = utils.GatewayNsPrivate
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
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
		return fmt.Errorf("upserting namespace %s: %w", nsName, err)
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.GatewayNsSaName,
			Namespace: nsName,
			Annotations: map[string]string{
				"azure.workload.identity/client-id": testConfig.clientId,
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
		return fmt.Errorf("creating service account in namespace %s: %w", nsName, err)
	}

	// Create single namespace-scoped ExternalDNS with all zone IDs
	externalDns := &v1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfig.zoneType.Prefix() + "gw-ns-dns",
			Namespace: nsName,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ExternalDNSSpec{
			ResourceName:       testConfig.zoneType.Prefix() + "gw-ns",
			DNSZoneResourceIDs: getZoneIDs(testConfig.zoneConfigs),
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: utils.GatewayNsSaName,
			},
		},
	}
	if err := upsert(ctx, cl, externalDns); err != nil {
		return fmt.Errorf("upserting namespace-scoped external dns: %w", err)
	}

	// Deploy gateway resources for each zone in the same namespace
	nsResources := make([]manifests.ObjectsContainer, len(testConfig.zoneConfigs))
	namespaceHostPrefixes := make([]string, len(testConfig.zoneConfigs)) // for DNS record verification during cleanup
	for i, zoneCfg := range testConfig.zoneConfigs {
		recordName := fmt.Sprintf("zone%d", zoneCfg.ZoneIndex)
		namespaceHostPrefixes[i] = recordName
		tlsHost := fmt.Sprintf("%s.%s", recordName, strings.TrimSuffix(zoneCfg.ZoneName, "."))
		resources, err := deployGatewayResourcesForZone(ctx, cl, zoneCfg, nsName, utils.GatewayNsSaName, testConfig.zoneType.Prefix(), tlsHost)
		if err != nil {
			return fmt.Errorf("deploying gateway resources for zone %d (ns-scoped): %w", zoneCfg.ZoneIndex, err)
		}
		nsResources[i] = resources
	}

	// Wait for all client deployments to be available in parallel
	eg2, egCtx2 := errgroup.WithContext(ctx)
	for i, resources := range nsResources {
		eg2.Go(func() error {
			castedResources := resources.(*manifests.GatewayClientServerResources)
			lgr.Info("waiting for client deployment to be available", "client", castedResources.Client.Name, "zoneIndex", i)
			if err := waitForAvailable(egCtx2, cl, *castedResources.Client); err != nil {
				return fmt.Errorf("waiting for client deployment (ns-scoped, zone %d): %w", i, err)
			}
			return nil
		})
	}
	if err := eg2.Wait(); err != nil {
		return err
	}

	lgr.Info("namespace-scoped externaldns test passed, cleaning up gateway resources")

	// Cleanup namespace-scoped test resources
	if err := cleanupMultiZoneResources(ctx, config, nsResources, externalDns, testConfig, namespaceHostPrefixes, nsName); err != nil {
		return fmt.Errorf("cleaning up namespace-scoped gateway resources: %w", err)
	}

	lgr.Info("finished multi-zone gateway with externaldns test")

	// Run filter tests
	if err := runAllFilterTests(ctx, config, testConfig); err != nil {
		return fmt.Errorf("running filter tests: %w", err)
	}

	return nil
}

// deployGatewayResourcesForZone creates Gateway API resources for a specific zone
func deployGatewayResourcesForZone(
	ctx context.Context,
	cl client.Client,
	zoneCfg gatewayZoneConfig,
	namespace string,
	serviceAccountName string,
	zoneTypePrefix string,
	tlsHost string,
) (*manifests.GatewayClientServerResources, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("deploying gateway resources", "host", tlsHost, "zone", zoneCfg.ZoneName, "namespace", namespace)
	// Create Gateway API resources
	resources := manifests.GatewayClientAndServer(
		namespace,
		fmt.Sprintf("%szone%d", zoneTypePrefix, zoneCfg.ZoneIndex), // unique name per zone
		zoneCfg.Nameserver,
		zoneCfg.KeyvaultCertURI,
		tlsHost,
		serviceAccountName,
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

// cleanupMultiZoneResources cleans up gateway resources for multiple zones
func cleanupMultiZoneResources(
	ctx context.Context,
	config *rest.Config,
	allResources []manifests.ObjectsContainer,
	dnsResource dns.ExternalDNSCRDConfiguration,
	testConfig multiZoneGatewayTestConfig,
	recordNames []string,
	externalDnsNamespace string,
) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Get the external-dns deployment name from the DNS resource
	externalDnsDeploymentName := dnsResource.GetInputResourceName() + "-external-dns"

	// Delete all resources for all zones
	for i, resources := range allResources {
		lgr.Info("deleting filter resources for zone", "zoneIndex", i)

		for _, obj := range resources.Objects() {
			if err := client.IgnoreNotFound(cl.Delete(ctx, obj)); err != nil {
				lgr.Info("failed to delete object", "error", err, "name", obj.GetName())
			}
		}
	}

	// Wait for DNS record deletion for each zone
	for i, zoneCfg := range testConfig.zoneConfigs {
		lgr.Info("waiting for DNS record deletion", "zone", zoneCfg.ZoneName, "record", recordNames[i])
		if err := waitForDNSRecordDeletion(ctx, config, externalDnsDeploymentName, externalDnsNamespace, zoneCfg.ZoneName, recordNames[i], testConfig.zoneType); err != nil {
			return fmt.Errorf("waiting for DNS record deletion (zone %s, record %s): %w", zoneCfg.ZoneName, recordNames[i], err)
		}
	}

	// Delete DNS CRD resource
	lgr.Info("deleting DNS resource", "name", dnsResource.GetName())
	if err := client.IgnoreNotFound(cl.Delete(ctx, dnsResource)); err != nil {
		lgr.Info("failed to delete DNS resource", "error", err, "name", dnsResource.GetName())
		return fmt.Errorf("failed to delete DNS resource: %w", err)

	}

	// Wait a bit for resources to be deleted
	time.Sleep(10 * time.Second)

	return nil
}

// waitForDNSRecordDeletion waits for the external-dns pod to log that it deleted both the DNS A record
// and the corresponding TXT ownership record.
// deploymentName is the name of the external-dns deployment (e.g., "gw-cluster-external-dns")
// namespace is the namespace where the deployment is located
func waitForDNSRecordDeletion(ctx context.Context, config *rest.Config, deploymentName, namespace, zoneName, recordName string, zoneType zoneType) error {
	lgr := logger.FromContext(ctx).With("zone", zoneName, "record", recordName, "deployment", deploymentName, "namespace", namespace)

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	// The expected log messages from external-dns when deleting records
	// A record: "Deleting A record named 'gateway-wi-ns' for Azure DNS zone 'example.com'"
	// TXT record: "Deleting TXT record named 'a-gateway-wi-ns' for Azure DNS zone 'example.com'"
	// The TXT record name is prefixed with "a-" to indicate it's the ownership record for an A record
	expectedARecordLog := fmt.Sprintf("Deleting A record named '%s' for %s '%s'", recordName, zoneType.String(), zoneName)
	txtRecordName := "a-" + recordName // external-dns prefixes TXT ownership records with the record type
	expectedTXTRecordLog := fmt.Sprintf("Deleting TXT record named '%s' for %s '%s'", txtRecordName, zoneType.String(), zoneName)

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
