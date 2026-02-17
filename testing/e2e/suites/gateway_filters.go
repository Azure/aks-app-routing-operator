package suites

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/utils"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// filterLabelKey is the label key used for filtering
	filterLabelKey = "externaldns"
	// filterLabelValue is the label value used for filtering
	filterLabelValue = "enabled"
)

// runAllFilterTests runs all 4 filter tests sequentially within a single test
// Each filter test validates filtering behavior across ALL zones
func runAllFilterTests(ctx context.Context, config *rest.Config, testConfig multiZoneGatewayTestConfig) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting multi-zone gateway and route label selector filter tests", "numZones", len(testConfig.zoneConfigs))

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	if len(testConfig.zoneConfigs) == 0 {
		return fmt.Errorf("no zone configs provided for filter tests")
	}

	// Setup cluster-scoped filter namespaces (one per zone for ClusterExternalDNS tests)
	if err := setupClusterScopedFilterNamespaces(ctx, cl, testConfig); err != nil {
		return fmt.Errorf("setting up cluster-scoped filter namespaces: %w", err)
	}

	// Setup namespace-scoped filter namespace (shared by namespace-scoped ExternalDNS tests)
	if err := setupNamespaceScopedFilterNamespace(ctx, cl, testConfig.clientId); err != nil {
		return fmt.Errorf("setting up namespace-scoped filter namespace: %w", err)
	}

	// ========================================
	// Test 1: ClusterExternalDNS Gateway Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 1: ClusterExternalDNS Gateway Label Selector (multi-zone)")
	lgr.Info("========================================")
	if err := runClusterExternalDNSGatewayLabelTest(ctx, config, testConfig); err != nil {
		return fmt.Errorf("clusterexternaldns gateway label selector test failed: %w", err)
	}

	// ========================================
	// Test 2: ClusterExternalDNS Route Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 2: ClusterExternalDNS Route Label Selector (multi-zone)")
	lgr.Info("========================================")
	if err := runClusterExternalDNSRouteLabelTest(ctx, config, testConfig); err != nil {
		return fmt.Errorf("clusterexternaldns route label selector test failed: %w", err)
	}

	// ========================================
	// Test 3: ExternalDNS Gateway Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 3: ExternalDNS Gateway Label Selector (multi-zone)")
	lgr.Info("========================================")
	if err := runExternalDNSGatewayLabelTest(ctx, config, testConfig); err != nil {
		return fmt.Errorf("externaldns gateway label selector test failed: %w", err)
	}

	// ========================================
	// Test 4: ExternalDNS Route Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 4: ExternalDNS Route Label Selector (multi-zone)")
	lgr.Info("========================================")
	if err := runExternalDNSRouteLabelTest(ctx, config, testConfig); err != nil {
		return fmt.Errorf("externaldns route label selector test failed: %w", err)
	}

	lgr.Info("all multi-zone gateway and route label selector filter tests passed")
	return nil
}

// runClusterExternalDNSGatewayLabelTest tests ClusterExternalDNS with gateway label selector across all zones
// Creates ONE ClusterExternalDNS with ALL zone IDs, deploys resources in per-zone namespaces
func runClusterExternalDNSGatewayLabelTest(ctx context.Context, config *rest.Config, testConfig multiZoneGatewayTestConfig) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Use first zone's namespace for the ClusterExternalDNS resource namespace
	resourceNamespace := utils.FilterClusterNsName(0)

	// Create ONE ClusterExternalDNS with ALL zone IDs and gateway label selector
	clusterExternalDns := &v1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testConfig.zoneType.Prefix() + "gw-label-filter",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceName:       testConfig.zoneType.Prefix() + "gw-label-filter",
			DNSZoneResourceIDs: getZoneIDs(testConfig.zoneConfigs),
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: utils.FilterClusterSaName,
			},
			ResourceNamespace: resourceNamespace,
			Filters: &v1alpha1.ExternalDNSFilters{
				GatewayLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, clusterExternalDns); err != nil {
		return fmt.Errorf("upserting cluster external dns: %w", err)
	}

	// Deploy resources for each zone in its own namespace
	allResources := make([]manifests.ObjectsContainer, len(testConfig.zoneConfigs))
	labeledHostPrefixes := make([]string, len(testConfig.zoneConfigs))

	for i, zoneCfg := range testConfig.zoneConfigs {
		nsName := utils.FilterClusterNsName(zoneCfg.ZoneIndex)

		// Host prefixes include zone index to avoid collisions
		labeledHostPrefix := fmt.Sprintf("gw-labeled-z%d", zoneCfg.ZoneIndex)
		unlabeledHostPrefix := fmt.Sprintf("gw-unlabeled-z%d", zoneCfg.ZoneIndex)
		labeledHostPrefixes[i] = labeledHostPrefix

		// Build hostnames
		labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")
		unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")

		lgr.Info("deploying gateway filter resources",
			"namespace", nsName,
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zoneIndex", zoneCfg.ZoneIndex)

		// Create gateway filter test resources
		resources := manifests.GatewayLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
			Namespace:          nsName,
			Name:               fmt.Sprintf("%sz%d", testConfig.zoneType.Prefix(), zoneCfg.ZoneIndex),
			Nameserver:         zoneCfg.Nameserver,
			KeyvaultURI:        zoneCfg.KeyvaultCertURI,
			LabeledHost:        labeledHost,
			UnlabeledHost:      unlabeledHost,
			ServiceAccountName: utils.FilterClusterSaName,
			GatewayClassName:   manifests.IstioGatewayClassName,
			FilterLabelKey:     filterLabelKey,
			FilterLabelValue:   filterLabelValue,
		})

		// Deploy all resources
		for _, obj := range resources.Objects() {
			if err := upsert(ctx, cl, obj); err != nil {
				return fmt.Errorf("upserting resource %s in zone %d: %w", obj.GetName(), zoneCfg.ZoneIndex, err)
			}
		}
		allResources[i] = &resources
	}

	// Wait for all client deployments to be available in parallel
	eg, egCtx := errgroup.WithContext(ctx)
	for i, resources := range allResources {
		eg.Go(func() error {
			castedResources := resources.(*manifests.GatewayFilterTestResources)
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

	lgr.Info("clusterexternaldns gateway label selector test passed for all zones")

	// Cleanup
	if err := cleanupMultiZoneResources(ctx, config, allResources, clusterExternalDns, testConfig, labeledHostPrefixes); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// runClusterExternalDNSRouteLabelTest tests ClusterExternalDNS with route label selector across all zones
// Creates ONE ClusterExternalDNS with ALL zone IDs, deploys resources in per-zone namespaces
func runClusterExternalDNSRouteLabelTest(ctx context.Context, config *rest.Config, testConfig multiZoneGatewayTestConfig) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Use first zone's namespace for the ClusterExternalDNS resource namespace
	resourceNamespace := utils.FilterClusterNsName(0)

	// Create ONE ClusterExternalDNS with ALL zone IDs and route label selector
	clusterExternalDns := &v1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: testConfig.zoneType.Prefix() + "route-label-filter",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceName:       testConfig.zoneType.Prefix() + "route-label-filter",
			DNSZoneResourceIDs: getZoneIDs(testConfig.zoneConfigs),
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: utils.FilterClusterSaName,
			},
			ResourceNamespace: resourceNamespace,
			Filters: &v1alpha1.ExternalDNSFilters{
				RouteAndIngressLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, clusterExternalDns); err != nil {
		return fmt.Errorf("upserting cluster external dns: %w", err)
	}

	// Deploy resources for each zone in its own namespace
	allResources := make([]manifests.ObjectsContainer, len(testConfig.zoneConfigs))
	labeledHostPrefixes := make([]string, len(testConfig.zoneConfigs))

	for i, zoneCfg := range testConfig.zoneConfigs {
		nsName := utils.FilterClusterNsName(zoneCfg.ZoneIndex)

		// Host prefixes include zone index to avoid collisions
		labeledHostPrefix := fmt.Sprintf("route-labeled-z%d", zoneCfg.ZoneIndex)
		unlabeledHostPrefix := fmt.Sprintf("route-unlabeled-z%d", zoneCfg.ZoneIndex)
		labeledHostPrefixes[i] = labeledHostPrefix

		// Build hostnames
		labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")
		unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")

		lgr.Info("deploying route filter resources",
			"namespace", nsName,
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zoneIndex", zoneCfg.ZoneIndex)

		// Create route filter test resources
		resources := manifests.RouteLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
			Namespace:          nsName,
			Name:               fmt.Sprintf("%sz%d", testConfig.zoneType.Prefix(), zoneCfg.ZoneIndex),
			Nameserver:         zoneCfg.Nameserver,
			KeyvaultURI:        zoneCfg.KeyvaultCertURI,
			LabeledHost:        labeledHost,
			UnlabeledHost:      unlabeledHost,
			ServiceAccountName: utils.FilterClusterSaName,
			GatewayClassName:   manifests.IstioGatewayClassName,
			FilterLabelKey:     filterLabelKey,
			FilterLabelValue:   filterLabelValue,
		})

		// Deploy all resources
		for _, obj := range resources.Objects() {
			if err := upsert(ctx, cl, obj); err != nil {
				return fmt.Errorf("upserting resource %s in zone %d: %w", obj.GetName(), zoneCfg.ZoneIndex, err)
			}
		}
		allResources[i] = &resources
	}

	// Wait for all client deployments to be available in parallel
	eg, egCtx := errgroup.WithContext(ctx)
	for i, resources := range allResources {
		eg.Go(func() error {
			castedResources := resources.(*manifests.GatewayFilterTestResources)
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

	lgr.Info("clusterexternaldns route label selector test passed for all zones")

	// Cleanup
	if err := cleanupMultiZoneResources(ctx, config, allResources, clusterExternalDns, testConfig, labeledHostPrefixes); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// runExternalDNSGatewayLabelTest tests ExternalDNS with gateway label selector across all zones
// Creates ONE ExternalDNS with ALL zone IDs, deploys all resources in the single FilterNs namespace
func runExternalDNSGatewayLabelTest(ctx context.Context, config *rest.Config, testConfig multiZoneGatewayTestConfig) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Create ONE ExternalDNS with ALL zone IDs and gateway label selector
	externalDns := &v1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfig.zoneType.Prefix() + "ns-gw-label-filter",
			Namespace: utils.FilterNs,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ExternalDNSSpec{
			ResourceName:       testConfig.zoneType.Prefix() + "ns-gw-label-filter",
			DNSZoneResourceIDs: getZoneIDs(testConfig.zoneConfigs),
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: utils.FilterNsSaName,
			},
			Filters: &v1alpha1.ExternalDNSFilters{
				GatewayLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, externalDns); err != nil {
		return fmt.Errorf("upserting external dns: %w", err)
	}

	// Deploy resources for each zone in the single FilterNs namespace
	allResources := make([]manifests.ObjectsContainer, len(testConfig.zoneConfigs))
	labeledHostPrefixes := make([]string, len(testConfig.zoneConfigs))

	for i, zoneCfg := range testConfig.zoneConfigs {
		// Host prefixes include zone index to avoid collisions
		labeledHostPrefix := fmt.Sprintf("ns-gw-labeled-z%d", zoneCfg.ZoneIndex)
		unlabeledHostPrefix := fmt.Sprintf("ns-gw-unlabeled-z%d", zoneCfg.ZoneIndex)
		labeledHostPrefixes[i] = labeledHostPrefix

		// Build hostnames
		labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")
		unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")

		lgr.Info("deploying gateway filter resources",
			"namespace", utils.FilterNs,
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zoneIndex", zoneCfg.ZoneIndex)

		// Create gateway filter test resources
		resources := manifests.GatewayLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
			Namespace:          utils.FilterNs,
			Name:               fmt.Sprintf("%sns-z%d", testConfig.zoneType.Prefix(), zoneCfg.ZoneIndex),
			Nameserver:         zoneCfg.Nameserver,
			KeyvaultURI:        zoneCfg.KeyvaultCertURI,
			LabeledHost:        labeledHost,
			UnlabeledHost:      unlabeledHost,
			ServiceAccountName: utils.FilterNsSaName,
			GatewayClassName:   manifests.IstioGatewayClassName,
			FilterLabelKey:     filterLabelKey,
			FilterLabelValue:   filterLabelValue,
		})

		// Deploy all resources
		for _, obj := range resources.Objects() {
			if err := upsert(ctx, cl, obj); err != nil {
				return fmt.Errorf("upserting resource %s in zone %d: %w", obj.GetName(), zoneCfg.ZoneIndex, err)
			}
		}
		allResources[i] = &resources
	}

	// Wait for all client deployments to be available in parallel
	eg, egCtx := errgroup.WithContext(ctx)
	for i, resources := range allResources {
		eg.Go(func() error {
			castedResources := resources.(*manifests.GatewayFilterTestResources)
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

	lgr.Info("externaldns gateway label selector test passed for all zones")

	// Cleanup
	if err := cleanupMultiZoneResources(ctx, config, allResources, externalDns, testConfig, labeledHostPrefixes); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// runExternalDNSRouteLabelTest tests ExternalDNS with route label selector across all zones
// Creates ONE ExternalDNS with ALL zone IDs, deploys all resources in the single FilterNs namespace
func runExternalDNSRouteLabelTest(ctx context.Context, config *rest.Config, testConfig multiZoneGatewayTestConfig) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Create ONE ExternalDNS with ALL zone IDs and route label selector
	externalDns := &v1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfig.zoneType.Prefix() + "ns-route-label-filter",
			Namespace: utils.FilterNs,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ExternalDNSSpec{
			ResourceName:       testConfig.zoneType.Prefix() + "ns-route-label-filter",
			DNSZoneResourceIDs: getZoneIDs(testConfig.zoneConfigs),
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: utils.FilterNsSaName,
			},
			Filters: &v1alpha1.ExternalDNSFilters{
				RouteAndIngressLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, externalDns); err != nil {
		return fmt.Errorf("upserting external dns: %w", err)
	}

	// Deploy resources for each zone in the single FilterNs namespace
	allResources := make([]manifests.ObjectsContainer, len(testConfig.zoneConfigs))
	labeledHostPrefixes := make([]string, len(testConfig.zoneConfigs))

	for i, zoneCfg := range testConfig.zoneConfigs {
		// Host prefixes include zone index to avoid collisions
		labeledHostPrefix := fmt.Sprintf("ns-route-labeled-z%d", zoneCfg.ZoneIndex)
		unlabeledHostPrefix := fmt.Sprintf("ns-route-unlabeled-z%d", zoneCfg.ZoneIndex)
		labeledHostPrefixes[i] = labeledHostPrefix

		// Build hostnames
		labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")
		unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneCfg.ZoneName, ".")

		lgr.Info("deploying route filter resources",
			"namespace", utils.FilterNs,
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zoneIndex", zoneCfg.ZoneIndex)

		// Create route filter test resources
		resources := manifests.RouteLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
			Namespace:          utils.FilterNs,
			Name:               fmt.Sprintf("%sns-z%d", testConfig.zoneType.Prefix(), zoneCfg.ZoneIndex),
			Nameserver:         zoneCfg.Nameserver,
			KeyvaultURI:        zoneCfg.KeyvaultCertURI,
			LabeledHost:        labeledHost,
			UnlabeledHost:      unlabeledHost,
			ServiceAccountName: utils.FilterNsSaName,
			GatewayClassName:   manifests.IstioGatewayClassName,
			FilterLabelKey:     filterLabelKey,
			FilterLabelValue:   filterLabelValue,
		})

		// Deploy all resources
		for _, obj := range resources.Objects() {
			if err := upsert(ctx, cl, obj); err != nil {
				return fmt.Errorf("upserting resource %s in zone %d: %w", obj.GetName(), zoneCfg.ZoneIndex, err)
			}
		}
		allResources[i] = &resources
	}

	// Wait for all client deployments to be available in parallel
	eg, egCtx := errgroup.WithContext(ctx)
	for i, resources := range allResources {
		eg.Go(func() error {
			castedResources := resources.(*manifests.GatewayFilterTestResources)
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

	lgr.Info("externaldns route label selector test passed for all zones")

	// Cleanup
	if err := cleanupMultiZoneResources(ctx, config, allResources, externalDns, testConfig, labeledHostPrefixes); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// setupClusterScopedFilterNamespaces creates namespaces and service accounts for cluster-scoped filter tests (one per zone)
func setupClusterScopedFilterNamespaces(ctx context.Context, cl client.Client, testConfig multiZoneGatewayTestConfig) error {
	lgr := logger.FromContext(ctx)

	for _, zoneCfg := range testConfig.zoneConfigs {
		nsName := utils.FilterClusterNsName(zoneCfg.ZoneIndex)
		lgr.Info("setting up cluster-scoped filter namespace", "namespace", nsName, "zoneIndex", zoneCfg.ZoneIndex)

		// Create namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
		}
		if err := upsert(ctx, cl, ns); err != nil {
			return fmt.Errorf("upserting namespace %s: %w", nsName, err)
		}

		// Create ServiceAccount with workload identity
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.FilterClusterSaName,
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
	}

	return nil
}

// setupNamespaceScopedFilterNamespace creates the namespace and service account for namespace-scoped filter tests
func setupNamespaceScopedFilterNamespace(ctx context.Context, cl client.Client, clientId string) error {
	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: utils.FilterNs,
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
			Name:      utils.FilterNsSaName,
			Namespace: utils.FilterNs,
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

	return nil
}
