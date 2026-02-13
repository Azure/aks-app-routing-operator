package suites

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/dns"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// filterTestNamespace is the namespace used for filter tests
	filterTestNamespace = "filter-ns"
	// filterTestServiceAccount is the service account used for filter tests
	filterTestServiceAccount = "filter-sa"
	// filterLabelKey is the label key used for filtering
	filterLabelKey = "externaldns"
	// filterLabelValue is the label value used for filtering
	filterLabelValue = "enabled"
)

// runAllFilterTests runs all 4 filter tests sequentially within a single test
func runAllFilterTests(ctx context.Context, config *rest.Config, gwTestConfig gatewayTestConfig) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting gateway and route label selector filter tests")

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Setup namespace and service account (shared by all tests)
	if err := setupFilterTestNamespace(ctx, cl, gwTestConfig); err != nil {
		return fmt.Errorf("setting up filter test namespace: %w", err)
	}

	// ========================================
	// Test 1: ClusterExternalDNS Gateway Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 1: ClusterExternalDNS Gateway Label Selector")
	lgr.Info("========================================")
	if err := runClusterExternalDNSGatewayLabelTest(ctx, config, gwTestConfig); err != nil {
		return fmt.Errorf("clusterexternaldns gateway label selector test failed: %w", err)
	}

	// ========================================
	// Test 2: ClusterExternalDNS Route Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 2: ClusterExternalDNS Route Label Selector")
	lgr.Info("========================================")
	if err := runClusterExternalDNSRouteLabelTest(ctx, config, gwTestConfig); err != nil {
		return fmt.Errorf("clusterexternaldns route label selector test failed: %w", err)
	}

	// ========================================
	// Test 3: ExternalDNS Gateway Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 3: ExternalDNS Gateway Label Selector")
	lgr.Info("========================================")
	if err := runExternalDNSGatewayLabelTest(ctx, config, gwTestConfig); err != nil {
		return fmt.Errorf("externaldns gateway label selector test failed: %w", err)
	}

	// ========================================
	// Test 4: ExternalDNS Route Label Selector
	// ========================================
	lgr.Info("========================================")
	lgr.Info("Test 4: ExternalDNS Route Label Selector")
	lgr.Info("========================================")
	if err := runExternalDNSRouteLabelTest(ctx, config, gwTestConfig); err != nil {
		return fmt.Errorf("externaldns route label selector test failed: %w", err)
	}

	lgr.Info("all gateway and route label selector filter tests passed")
	return nil
}

// runClusterExternalDNSGatewayLabelTest tests ClusterExternalDNS with gateway label selector
func runClusterExternalDNSGatewayLabelTest(ctx context.Context, config *rest.Config, gwTestConfig gatewayTestConfig) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	zoneName := gwTestConfig.zoneConfig.ZoneName
	nameserver := gwTestConfig.zoneConfig.Nameserver
	keyvaultURI := gwTestConfig.zoneConfig.KeyvaultCertURI

	// Host prefixes used for DNS records
	const labeledHostPrefix = "gw-labeled"
	const unlabeledHostPrefix = "gw-unlabeled"

	// Build hostnames
	labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneName, ".")
	unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneName, ".")

	lgr.Info("deploying gateway filter resources",
		"labeledHost", labeledHost,
		"unlabeledHost", unlabeledHost,
		"zone", zoneName)

	// Create ClusterExternalDNS with gateway label selector
	clusterExternalDns := &v1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: gwTestConfig.zoneConfig.NamePrefix + "gw-label-filter",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceName:       gwTestConfig.zoneConfig.NamePrefix + "gw-label-filter",
			DNSZoneResourceIDs: []string{gwTestConfig.zoneConfig.ZoneID},
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				Type:           v1alpha1.IdentityTypeWorkloadIdentity,
				ServiceAccount: filterTestServiceAccount,
			},
			ResourceNamespace: filterTestNamespace,
			Filters: &v1alpha1.ExternalDNSFilters{
				GatewayLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, clusterExternalDns); err != nil {
		return fmt.Errorf("upserting cluster external dns: %w", err)
	}

	// Create gateway filter test resources
	resources := manifests.GatewayLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
		Namespace:          filterTestNamespace,
		Name:               zoneName,
		Nameserver:         nameserver,
		KeyvaultURI:        keyvaultURI,
		LabeledHost:        labeledHost,
		UnlabeledHost:      unlabeledHost,
		ServiceAccountName: filterTestServiceAccount,
		GatewayClassName:   manifests.IstioGatewayClassName,
		FilterLabelKey:     filterLabelKey,
		FilterLabelValue:   filterLabelValue,
	})

	// Deploy all resources
	for _, obj := range resources.Objects() {
		if err := upsert(ctx, cl, obj); err != nil {
			return fmt.Errorf("upserting resource %s: %w", obj.GetName(), err)
		}
	}

	// Wait for client deployment to be available (validates that labeled gateway is reachable
	// and unlabeled gateway is unreachable)
	lgr.Info("waiting for client deployment to be available", "client", resources.Client.Name)
	if err := waitForAvailable(ctx, cl, *resources.Client); err != nil {
		return fmt.Errorf("waiting for client deployment: %w", err)
	}

	lgr.Info("clusterexternaldns gateway label selector test passed")

	// Cleanup
	if err := cleanupFilterResources(ctx, config, &resources, clusterExternalDns, zoneName, labeledHostPrefix); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// runClusterExternalDNSRouteLabelTest tests ClusterExternalDNS with route label selector
func runClusterExternalDNSRouteLabelTest(ctx context.Context, config *rest.Config, gwTestConfig gatewayTestConfig) error {
	lgr := logger.FromContext(ctx)
	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	zoneName := gwTestConfig.zoneConfig.ZoneName
	nameserver := gwTestConfig.zoneConfig.Nameserver
	keyvaultURI := gwTestConfig.zoneConfig.KeyvaultCertURI

	// Host prefixes used for DNS records
	const labeledHostPrefix = "route-labeled"
	const unlabeledHostPrefix = "route-unlabeled"

	// Build hostnames
	labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneName, ".")
	unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneName, ".")

	lgr.Info("deploying route filter resources",
		"labeledHost", labeledHost,
		"unlabeledHost", unlabeledHost,
		"zone", zoneName)

	// Create ClusterExternalDNS with route label selector
	clusterExternalDns := &v1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: gwTestConfig.zoneConfig.NamePrefix + "route-label-filter",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceName:       "route-label-filter",
			DNSZoneResourceIDs: []string{gwTestConfig.zoneConfig.ZoneID},
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				Type:           v1alpha1.IdentityTypeWorkloadIdentity,
				ServiceAccount: filterTestServiceAccount,
			},
			ResourceNamespace: filterTestNamespace,
			Filters: &v1alpha1.ExternalDNSFilters{
				RouteAndIngressLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, clusterExternalDns); err != nil {
		return fmt.Errorf("upserting cluster external dns: %w", err)
	}

	// Create route filter test resources
	resources := manifests.RouteLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
		Namespace:          filterTestNamespace,
		Name:               zoneName,
		Nameserver:         nameserver,
		KeyvaultURI:        keyvaultURI,
		LabeledHost:        labeledHost,
		UnlabeledHost:      unlabeledHost,
		ServiceAccountName: filterTestServiceAccount,
		GatewayClassName:   manifests.IstioGatewayClassName,
		FilterLabelKey:     filterLabelKey,
		FilterLabelValue:   filterLabelValue,
	})

	// Deploy all resources
	for _, obj := range resources.Objects() {
		if err := upsert(ctx, cl, obj); err != nil {
			return fmt.Errorf("upserting resource %s: %w", obj.GetName(), err)
		}
	}

	// Wait for client deployment to be available
	lgr.Info("waiting for client deployment to be available", "client", resources.Client.Name)
	if err := waitForAvailable(ctx, cl, *resources.Client); err != nil {
		return fmt.Errorf("waiting for client deployment: %w", err)
	}

	lgr.Info("clusterexternaldns route label selector test passed")

	// Cleanup
	if err := cleanupFilterResources(ctx, config, &resources, clusterExternalDns, zoneName, labeledHostPrefix); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// runExternalDNSGatewayLabelTest tests ExternalDNS with gateway label selector
func runExternalDNSGatewayLabelTest(ctx context.Context, config *rest.Config, gwTestConfig gatewayTestConfig) error {
	lgr := logger.FromContext(ctx)
	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	zoneName := gwTestConfig.zoneConfig.ZoneName
	nameserver := gwTestConfig.zoneConfig.Nameserver
	keyvaultURI := gwTestConfig.zoneConfig.KeyvaultCertURI

	// Host prefixes used for DNS records
	const labeledHostPrefix = "ns-gw-labeled"
	const unlabeledHostPrefix = "ns-gw-unlabeled"

	// Build hostnames
	labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneName, ".")
	unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneName, ".")

	lgr.Info("deploying gateway filter resources",
		"labeledHost", labeledHost,
		"unlabeledHost", unlabeledHost,
		"zone", zoneName)

	// Create ExternalDNS with gateway label selector
	externalDns := &v1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwTestConfig.zoneConfig.NamePrefix + "ns-gw-label-filter",
			Namespace: filterTestNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ExternalDNSSpec{
			ResourceName:       "ns-gw-label-filter",
			DNSZoneResourceIDs: []string{gwTestConfig.zoneConfig.ZoneID},
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				Type:           v1alpha1.IdentityTypeWorkloadIdentity,
				ServiceAccount: filterTestServiceAccount,
			},
			Filters: &v1alpha1.ExternalDNSFilters{
				GatewayLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, externalDns); err != nil {
		return fmt.Errorf("upserting external dns: %w", err)
	}

	// Create gateway filter test resources
	resources := manifests.GatewayLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
		Namespace:          filterTestNamespace,
		Name:               "ns-" + zoneName,
		Nameserver:         nameserver,
		KeyvaultURI:        keyvaultURI,
		LabeledHost:        labeledHost,
		UnlabeledHost:      unlabeledHost,
		ServiceAccountName: filterTestServiceAccount,
		GatewayClassName:   manifests.IstioGatewayClassName,
		FilterLabelKey:     filterLabelKey,
		FilterLabelValue:   filterLabelValue,
	})

	// Deploy all resources
	for _, obj := range resources.Objects() {
		if err := upsert(ctx, cl, obj); err != nil {
			return fmt.Errorf("upserting resource %s: %w", obj.GetName(), err)
		}
	}

	// Wait for client deployment to be available
	lgr.Info("waiting for client deployment to be available", "client", resources.Client.Name)
	if err := waitForAvailable(ctx, cl, *resources.Client); err != nil {
		return fmt.Errorf("waiting for client deployment: %w", err)
	}

	lgr.Info("externaldns gateway label selector test passed")

	// Cleanup
	if err := cleanupFilterResources(ctx, config, &resources, externalDns, zoneName, labeledHostPrefix); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// runExternalDNSRouteLabelTest tests ExternalDNS with route label selector
func runExternalDNSRouteLabelTest(ctx context.Context, config *rest.Config, gwTestConfig gatewayTestConfig) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	zoneName := gwTestConfig.zoneConfig.ZoneName
	nameserver := gwTestConfig.zoneConfig.Nameserver
	keyvaultURI := gwTestConfig.zoneConfig.KeyvaultCertURI

	// Host prefixes used for DNS records
	const labeledHostPrefix = "ns-route-labeled"
	const unlabeledHostPrefix = "ns-route-unlabeled"

	// Build hostnames
	labeledHost := labeledHostPrefix + "." + strings.TrimRight(zoneName, ".")
	unlabeledHost := unlabeledHostPrefix + "." + strings.TrimRight(zoneName, ".")

	lgr.Info("deploying route filter resources",
		"labeledHost", labeledHost,
		"unlabeledHost", unlabeledHost,
		"zone", zoneName)

	// Create ExternalDNS with route label selector
	externalDns := &v1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwTestConfig.zoneConfig.NamePrefix + "route-label-filter",
			Namespace: filterTestNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExternalDNS",
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		Spec: v1alpha1.ExternalDNSSpec{
			ResourceName:       "ns-route-label-filter",
			DNSZoneResourceIDs: []string{gwTestConfig.zoneConfig.ZoneID},
			ResourceTypes:      []string{"gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				Type:           v1alpha1.IdentityTypeWorkloadIdentity,
				ServiceAccount: filterTestServiceAccount,
			},
			Filters: &v1alpha1.ExternalDNSFilters{
				RouteAndIngressLabelSelector: to.Ptr(filterLabelKey + "=" + filterLabelValue),
			},
		},
	}
	if err := upsert(ctx, cl, externalDns); err != nil {
		return fmt.Errorf("upserting external dns: %w", err)
	}

	// Create route filter test resources
	resources := manifests.RouteLabelFilterResources(manifests.GatewayLabelFilterTestConfig{
		Namespace:          filterTestNamespace,
		Name:               "ns-" + zoneName,
		Nameserver:         nameserver,
		KeyvaultURI:        keyvaultURI,
		LabeledHost:        labeledHost,
		UnlabeledHost:      unlabeledHost,
		ServiceAccountName: filterTestServiceAccount,
		GatewayClassName:   manifests.IstioGatewayClassName,
		FilterLabelKey:     filterLabelKey,
		FilterLabelValue:   filterLabelValue,
	})

	// Deploy all resources
	for _, obj := range resources.Objects() {
		if err := upsert(ctx, cl, obj); err != nil {
			return fmt.Errorf("upserting resource %s: %w", obj.GetName(), err)
		}
	}

	// Wait for client deployment to be available
	lgr.Info("waiting for client deployment to be available", "client", resources.Client.Name)
	if err := waitForAvailable(ctx, cl, *resources.Client); err != nil {
		return fmt.Errorf("waiting for client deployment: %w", err)
	}

	lgr.Info("externaldns route label selector test passed")

	// Cleanup
	if err := cleanupFilterResources(ctx, config, &resources, externalDns, zoneName, labeledHostPrefix); err != nil {
		return fmt.Errorf("cleaning up resources: %w", err)
	}

	return nil
}

// setupFilterTestNamespace creates the namespace and service account for filter tests
func setupFilterTestNamespace(ctx context.Context, cl client.Client, gwTestConfig gatewayTestConfig) error {
	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: filterTestNamespace,
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
			Name:      filterTestServiceAccount,
			Namespace: filterTestNamespace,
			Annotations: map[string]string{
				"azure.workload.identity/client-id": gwTestConfig.clientId,
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

// cleanupFilterResources cleans up resources created for filter tests
func cleanupFilterResources(ctx context.Context, config *rest.Config, resources *manifests.GatewayFilterTestResources, dnsResource dns.ExternalDNSCRDConfiguration, zoneName, recordName string) error {
	lgr := logger.FromContext(ctx)

	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	// Delete gateways and routes
	if resources.LabeledGateway != nil {
		if err := client.IgnoreNotFound(cl.Delete(ctx, resources.LabeledGateway)); err != nil {
			lgr.Info("failed to delete labeled gateway", "error", err)
		}
	}
	if resources.UnlabeledGateway != nil {
		if err := client.IgnoreNotFound(cl.Delete(ctx, resources.UnlabeledGateway)); err != nil {
			lgr.Info("failed to delete unlabeled gateway", "error", err)
		}
	}
	if resources.LabeledRoute != nil {
		if err := client.IgnoreNotFound(cl.Delete(ctx, resources.LabeledRoute)); err != nil {
			lgr.Info("failed to delete labeled route", "error", err)
		}
	}
	if resources.UnlabeledRoute != nil {
		if err := client.IgnoreNotFound(cl.Delete(ctx, resources.UnlabeledRoute)); err != nil {
			lgr.Info("failed to delete unlabeled route", "error", err)
		}
	}

	// Delete client and server
	if resources.Client != nil {
		if err := client.IgnoreNotFound(cl.Delete(ctx, resources.Client)); err != nil {
			lgr.Info("failed to delete client", "error", err)
		}
	}
	if resources.Server != nil {
		if err := client.IgnoreNotFound(cl.Delete(ctx, resources.Server)); err != nil {
			lgr.Info("failed to delete server", "error", err)
		}
	}
	if resources.Service != nil {
		if err := client.IgnoreNotFound(cl.Delete(ctx, resources.Service)); err != nil {
			lgr.Info("failed to delete service", "error", err)
		}
	}

	// wait for dns records to be deleted
	if err := waitForDNSRecordDeletion(ctx, config, dnsResource.GetInputResourceName()+"-external-dns", dnsResource.GetResourceNamespace(), zoneName, recordName); err != nil {
		lgr.Error("failed to wait for dns record deletion", "error", err)
		return fmt.Errorf("error waiting for dns record deletion: %w", err)
	}

	// Delete DNS resource
	if err := client.IgnoreNotFound(cl.Delete(ctx, dnsResource)); err != nil {
		lgr.Info("failed to delete dns resource", "error", err)
	}

	// Wait a bit for resources to be deleted
	time.Sleep(10 * time.Second)

	return nil
}
