package suites

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
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

func gatewayFilterTests(in infra.Provisioned) []test {
	// Only run gateway filter tests on clusters with Gateway API and Istio enabled
	if !isGatewayCluster(in) {
		return []test{}
	}

	return []test{
		{
			name: "clusterexternaldns gateway label selector",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				build(),
			run: clusterExternalDNSGatewayLabelSelectorTest(in),
		},
		{
			name: "clusterexternaldns route label selector",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				build(),
			run: clusterExternalDNSRouteLabelSelectorTest(in),
		},
		{
			name: "externaldns gateway label selector",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				build(),
			run: externalDNSGatewayLabelSelectorTest(in),
		},
		{
			name: "externaldns route label selector",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				build(),
			run: externalDNSRouteLabelSelectorTest(in),
		},
	}
}

func clusterExternalDNSGatewayLabelSelectorTest(in infra.Provisioned) func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
	return func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
		lgr := logger.FromContext(ctx)
		lgr.Info("starting clusterexternaldns gateway label selector test")

		cl, err := client.New(config, client.Options{
			Scheme: scheme,
		})
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		// Setup namespace and service account
		if err := setupFilterTestNamespace(ctx, cl, in); err != nil {
			return fmt.Errorf("setting up filter test namespace: %w", err)
		}

		zone := in.ManagedIdentityZone.Zone
		zoneName := zone.GetName()
		nameserver := zone.GetNameservers()[0]
		keyvaultURI := in.ManagedIdentityZone.Cert.GetId()

		// Build hostnames
		labeledHost := "gw-labeled." + strings.TrimRight(zoneName, ".")
		unlabeledHost := "gw-unlabeled." + strings.TrimRight(zoneName, ".")

		lgr.Info("deploying gateway filter resources",
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zone", zoneName)

		// Create ClusterExternalDNS with gateway label selector
		clusterExternalDns := &v1alpha1.ClusterExternalDNS{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gw-label-filter",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterExternalDNS",
				APIVersion: v1alpha1.GroupVersion.String(),
			},
			Spec: v1alpha1.ClusterExternalDNSSpec{
				ResourceName:       "gw-label-filter",
				DNSZoneResourceIDs: []string{in.ManagedIdentityZone.Zone.GetId()},
				ResourceTypes:      []string{"gateway"},
				Identity: v1alpha1.ExternalDNSIdentity{
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
		if err := cleanupFilterResources(ctx, cl, &resources, clusterExternalDns); err != nil {
			return fmt.Errorf("cleaning up resources: %w", err)
		}

		return nil
	}
}

func clusterExternalDNSRouteLabelSelectorTest(in infra.Provisioned) func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
	return func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
		lgr := logger.FromContext(ctx)
		lgr.Info("starting clusterexternaldns route label selector test")

		cl, err := client.New(config, client.Options{
			Scheme: scheme,
		})
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		// Setup namespace and service account
		if err := setupFilterTestNamespace(ctx, cl, in); err != nil {
			return fmt.Errorf("setting up filter test namespace: %w", err)
		}

		zone := in.ManagedIdentityZone.Zone
		zoneName := zone.GetName()
		nameserver := zone.GetNameservers()[0]
		keyvaultURI := in.ManagedIdentityZone.Cert.GetId()

		// Build hostnames
		labeledHost := "route-labeled." + strings.TrimRight(zoneName, ".")
		unlabeledHost := "route-unlabeled." + strings.TrimRight(zoneName, ".")

		lgr.Info("deploying route filter resources",
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zone", zoneName)

		// Create ClusterExternalDNS with route label selector
		clusterExternalDns := &v1alpha1.ClusterExternalDNS{
			ObjectMeta: metav1.ObjectMeta{
				Name: "route-label-filter",
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterExternalDNS",
				APIVersion: v1alpha1.GroupVersion.String(),
			},
			Spec: v1alpha1.ClusterExternalDNSSpec{
				ResourceName:       "route-label-filter",
				DNSZoneResourceIDs: []string{in.ManagedIdentityZone.Zone.GetId()},
				ResourceTypes:      []string{"gateway"},
				Identity: v1alpha1.ExternalDNSIdentity{
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
		if err := cleanupFilterResources(ctx, cl, &resources, clusterExternalDns); err != nil {
			return fmt.Errorf("cleaning up resources: %w", err)
		}

		return nil
	}
}

func externalDNSGatewayLabelSelectorTest(in infra.Provisioned) func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
	return func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
		lgr := logger.FromContext(ctx)
		lgr.Info("starting externaldns gateway label selector test")

		cl, err := client.New(config, client.Options{
			Scheme: scheme,
		})
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		// Setup namespace and service account
		if err := setupFilterTestNamespace(ctx, cl, in); err != nil {
			return fmt.Errorf("setting up filter test namespace: %w", err)
		}

		zone := in.ManagedIdentityZone.Zone
		zoneName := zone.GetName()
		nameserver := zone.GetNameservers()[0]
		keyvaultURI := in.ManagedIdentityZone.Cert.GetId()

		// Build hostnames
		labeledHost := "ns-gw-labeled." + strings.TrimRight(zoneName, ".")
		unlabeledHost := "ns-gw-unlabeled." + strings.TrimRight(zoneName, ".")

		lgr.Info("deploying gateway filter resources",
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zone", zoneName)

		// Create ExternalDNS with gateway label selector
		externalDns := &v1alpha1.ExternalDNS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ns-gw-label-filter",
				Namespace: filterTestNamespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "ExternalDNS",
				APIVersion: v1alpha1.GroupVersion.String(),
			},
			Spec: v1alpha1.ExternalDNSSpec{
				ResourceName:       "ns-gw-label-filter",
				DNSZoneResourceIDs: []string{in.ManagedIdentityZone.Zone.GetId()},
				ResourceTypes:      []string{"gateway"},
				Identity: v1alpha1.ExternalDNSIdentity{
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
		if err := cleanupFilterResources(ctx, cl, &resources, externalDns); err != nil {
			return fmt.Errorf("cleaning up resources: %w", err)
		}

		return nil
	}
}

func externalDNSRouteLabelSelectorTest(in infra.Provisioned) func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
	return func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
		lgr := logger.FromContext(ctx)
		lgr.Info("starting externaldns route label selector test")

		cl, err := client.New(config, client.Options{
			Scheme: scheme,
		})
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		// Setup namespace and service account
		if err := setupFilterTestNamespace(ctx, cl, in); err != nil {
			return fmt.Errorf("setting up filter test namespace: %w", err)
		}

		zone := in.ManagedIdentityZone.Zone
		zoneName := zone.GetName()
		nameserver := zone.GetNameservers()[0]
		keyvaultURI := in.ManagedIdentityZone.Cert.GetId()

		// Build hostnames
		labeledHost := "ns-route-labeled." + strings.TrimRight(zoneName, ".")
		unlabeledHost := "ns-route-unlabeled." + strings.TrimRight(zoneName, ".")

		lgr.Info("deploying route filter resources",
			"labeledHost", labeledHost,
			"unlabeledHost", unlabeledHost,
			"zone", zoneName)

		// Create ExternalDNS with route label selector
		externalDns := &v1alpha1.ExternalDNS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ns-route-label-filter",
				Namespace: filterTestNamespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "ExternalDNS",
				APIVersion: v1alpha1.GroupVersion.String(),
			},
			Spec: v1alpha1.ExternalDNSSpec{
				ResourceName:       "ns-route-label-filter",
				DNSZoneResourceIDs: []string{in.ManagedIdentityZone.Zone.GetId()},
				ResourceTypes:      []string{"gateway"},
				Identity: v1alpha1.ExternalDNSIdentity{
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
		if err := cleanupFilterResources(ctx, cl, &resources, externalDns); err != nil {
			return fmt.Errorf("cleaning up resources: %w", err)
		}

		return nil
	}
}

// setupFilterTestNamespace creates the namespace and service account for filter tests
func setupFilterTestNamespace(ctx context.Context, cl client.Client, in infra.Provisioned) error {
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

	return nil
}

// cleanupFilterResources cleans up resources created for filter tests
func cleanupFilterResources(ctx context.Context, cl client.Client, resources *manifests.GatewayFilterTestResources, dnsResource client.Object) error {
	lgr := logger.FromContext(ctx)

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

	// Delete DNS resource
	if err := client.IgnoreNotFound(cl.Delete(ctx, dnsResource)); err != nil {
		lgr.Info("failed to delete dns resource", "error", err)
	}

	return nil
}
