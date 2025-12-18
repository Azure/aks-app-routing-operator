package suites

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			name: "gateway with cluster-scoped externaldns",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting gateway with cluster-scoped externaldns test")

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

				// Create ClusterExternalDNS for private zone
				privateClusterExternalDns := &v1alpha1.ClusterExternalDNS{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gw-private-cluster-dns",
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterExternalDNS",
						APIVersion: v1alpha1.GroupVersion.String(),
					},
					Spec: v1alpha1.ClusterExternalDNSSpec{
						ResourceName:       "gw-private-cluster-external-dns",
						DNSZoneResourceIDs: []string{in.ManagedIdentityPrivateZone.Zone.GetId()},
						ResourceTypes:      []string{"gateway"},
						Identity: v1alpha1.ExternalDNSIdentity{
							ServiceAccount: sa.Name,
						},
						ResourceNamespace: gatewayTestNamespace,
					},
				}
				if err := upsert(ctx, cl, privateClusterExternalDns); err != nil {
					return fmt.Errorf("upserting private cluster external dns: %w", err)
				}

				// Test with public zone
				if err := gatewayClientServerTest(ctx, cl, in, operator, in.ManagedIdentityZone); err != nil {
					return fmt.Errorf("testing public zone: %w", err)
				}

				lgr.Info("finished gateway with cluster-scoped externaldns test")
				return nil
			},
		},
		{
			name: "gateway with namespace-scoped externaldns",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting gateway with namespace-scoped externaldns test")

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
					return fmt.Errorf("upserting external dns: %w", err)
				}

				// Create namespace-scoped ExternalDNS for private zone
				privateExternalDns := &v1alpha1.ExternalDNS{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-private-ns-dns",
						Namespace: gatewayTestNamespace,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "ExternalDNS",
						APIVersion: v1alpha1.GroupVersion.String(),
					},
					Spec: v1alpha1.ExternalDNSSpec{
						ResourceName:       "gw-private-ns-external-dns",
						DNSZoneResourceIDs: []string{in.ManagedIdentityPrivateZone.Zone.GetId()},
						ResourceTypes:      []string{"gateway"},
						Identity: v1alpha1.ExternalDNSIdentity{
							ServiceAccount: sa.Name,
						},
					},
				}
				if err := upsert(ctx, cl, privateExternalDns); err != nil {
					return fmt.Errorf("upserting private external dns: %w", err)
				}

				// Test with public zone
				if err := gatewayClientServerTest(ctx, cl, in, operator, in.ManagedIdentityZone); err != nil {
					return fmt.Errorf("testing public zone: %w", err)
				}

				lgr.Info("finished gateway with namespace-scoped externaldns test")
				return nil
			},
		},
	}
}

// gatewayClientServerTest deploys Gateway API resources and validates connectivity
func gatewayClientServerTest(
	ctx context.Context,
	cl client.Client,
	in infra.Provisioned,
	operator manifests.OperatorConfig,
	zoneWithCert infra.WithCert[infra.Zone],
) error {
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
		zoneName[:min(len(zoneName), 40)], // truncate name to avoid length issues
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
			return fmt.Errorf("upserting resource %s: %w", obj.GetName(), err)
		}
	}

	// Wait for client deployment to be available (validates end-to-end connectivity)
	lgr.Info("waiting for client deployment to be available", "client", resources.Client.Name)
	if err := waitForAvailable(ctx, cl, *resources.Client); err != nil {
		return fmt.Errorf("waiting for client deployment to be available: %w", err)
	}

	lgr.Info("gateway test passed", "zone", zoneName)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
