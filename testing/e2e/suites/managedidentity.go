package suites

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func managedIdentityTests(in infra.Provisioned) []test {
	// Skip managed identity tests on gateway clusters - these tests create ExternalDNS
	// resources targeting ManagedIdentityZone and ManagedIdentityPrivateZone. On Istio/gateway
	// clusters, the gateway tests also manage those same zones, so two different controllers
	// would compete to manage DNS records for the same zone, causing flaky behavior.
	opts := in.Cluster.GetOptions()
	if _, hasIstio := opts[clients.IstioServiceMeshOpt.Name]; hasIstio {
		return []test{}
	}

	return []test{
		{
			name: "ExternalDNS MSI Ingress",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				cl, err := client.New(config, client.Options{
					Scheme: scheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				msiNic := manifests.NewNginxIngressController("msi", "msi.class")
				if err := upsert(ctx, cl, msiNic); err != nil {
					return fmt.Errorf("upserting nginx ingress controller: %w", err)
				}

				var service v1alpha1.ManagedObjectReference
				lgr.Info("waiting for service associated with NIC to be ready")
				if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
					lgr.Info("checking if NIC service is ready")
					var nic v1alpha1.NginxIngressController
					if err := cl.Get(ctx, client.ObjectKeyFromObject(msiNic), &nic); err != nil {
						return false, fmt.Errorf("get msi nic: %w", err)
					}

					if nic.Status.ManagedResourceRefs == nil {
						return false, nil
					}

					for _, ref := range nic.Status.ManagedResourceRefs {
						if ref.Kind == "Service" && !strings.HasSuffix(ref.Name, "-metrics") {
							lgr.Info("found service")
							service = ref
							return true, nil
						}
					}

					lgr.Info("service not found")
					return false, nil
				}); err != nil {
					return fmt.Errorf("waiting for MSI NIC to be ready: %w", err)
				}

				if err := clientServerTest(ctx, config, operator, singleNamespacer{namespace: "msi-ns"}, in, func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error {
					ns := ingress.GetNamespace()

					externalDns := &v1alpha1.ExternalDNS{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "msi",
							Namespace: ns,
						},
						TypeMeta: metav1.TypeMeta{
							Kind:       "ExternalDNS",
							APIVersion: v1alpha1.GroupVersion.String(),
						},
						Spec: v1alpha1.ExternalDNSSpec{
							ResourceName:       "msi-external-dns",
							DNSZoneResourceIDs: []string{in.ManagedIdentityZone.Zone.GetId()},
							ResourceTypes:      []string{"ingress"},
							Identity: v1alpha1.ExternalDNSIdentity{
								Type:     v1alpha1.IdentityTypeManagedIdentity,
								ClientID: in.ManagedIdentity.GetClientID(),
							},
						},
					}
					if err := upsert(ctx, cl, externalDns); err != nil {
						return fmt.Errorf("upserting external dns: %w", err)
					}

					privateExternalDns := &v1alpha1.ExternalDNS{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "private-msi",
							Namespace: ns,
						},
						TypeMeta: metav1.TypeMeta{
							Kind:       "ExternalDNS",
							APIVersion: v1alpha1.GroupVersion.String(),
						},
						Spec: v1alpha1.ExternalDNSSpec{
							ResourceName:       "msi-private-external-dns",
							DNSZoneResourceIDs: []string{in.ManagedIdentityPrivateZone.Zone.GetId()},
							ResourceTypes:      []string{"ingress"},
							Identity: v1alpha1.ExternalDNSIdentity{
								Type:     v1alpha1.IdentityTypeManagedIdentity,
								ClientID: in.ManagedIdentity.GetClientID(),
							},
						},
					}
					if err := upsert(ctx, cl, privateExternalDns); err != nil {
						return fmt.Errorf("upserting private external dns: %w", err)
					}

					ingress.Spec.IngressClassName = util.ToPtr(msiNic.Spec.IngressClassName)
					return nil
				}, util.ToPtr(service.Name), func(ctx context.Context, c client.Client, namespacer namespacer, operator manifests.OperatorConfig, infra infra.Provisioned, serviceName *string) ([]zoner, error) {
					zs, err := toZoners(ctx, cl, namespacer, infra.ManagedIdentityZone)
					if err != nil {
						return nil, fmt.Errorf("getting zoners: %w", err)
					}

					pzs, err := toPrivateZoners(ctx, cl, namespacer, infra.ManagedIdentityPrivateZone, in.Cluster.GetDnsServiceIp())
					if err != nil {
						return nil, fmt.Errorf("getting private zoners: %w", err)
					}

					return append(zs, pzs...), nil
				}); err != nil {
					return err
				}

				lgr.Info("finished testing")
				return nil
			},
		},
	}
}
