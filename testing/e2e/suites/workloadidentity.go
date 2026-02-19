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

func workloadIdentityTests(in infra.Provisioned) []test {
	// Skip workload identity tests on gateway clusters - these tests are for Ingress
	// and would conflict with gateway tests over ManagedIdentityZone ownership
	opts := in.Cluster.GetOptions()
	if _, hasIstio := opts[clients.IstioServiceMeshOpt.Name]; hasIstio {
		return []test{}
	}

	return []test{
		{
			name: "Workload Identity Ingress",
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

				wiNic := manifests.NewNginxIngressController("wi", "wi.class")
				if err := upsert(ctx, cl, wiNic); err != nil {
					return fmt.Errorf("upserting nginx ingress controller: %w", err)
				}

				var service v1alpha1.ManagedObjectReference
				lgr.Info("waiting for service associated with NIC to be ready")
				if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
					lgr.Info("checking if NIC service is ready")
					var nic v1alpha1.NginxIngressController
					if err := cl.Get(ctx, client.ObjectKeyFromObject(wiNic), &nic); err != nil {
						return false, fmt.Errorf("get private nic: %w", err)
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
					return fmt.Errorf("waiting for private NIC to be ready: %w", err)
				}

				if err := clientServerTest(ctx, config, operator, singleNamespacer{namespace: "wi-ns"}, in, func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error {
					ns := ingress.GetNamespace()
					sa := &corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "wi-sa",
							Namespace: ns,
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

					clusterExternalDns := &v1alpha1.ClusterExternalDNS{
						ObjectMeta: metav1.ObjectMeta{
							Name: "wi",
						},
						TypeMeta: metav1.TypeMeta{
							Kind:       "ClusterExternalDNS",
							APIVersion: v1alpha1.GroupVersion.String(),
						},
						Spec: v1alpha1.ClusterExternalDNSSpec{
							ResourceName:       "wi-cluster-external-dns",
							DNSZoneResourceIDs: []string{in.ManagedIdentityZones[0].Zone.GetId()},
							ResourceTypes:      []string{"ingress"},
							Identity: v1alpha1.ExternalDNSIdentity{
								Type:           v1alpha1.IdentityTypeWorkloadIdentity,
								ServiceAccount: sa.Name,
							},
							ResourceNamespace: ns,
						},
					}
					if err := upsert(ctx, cl, clusterExternalDns); err != nil {
						return fmt.Errorf("upserting cluster external dns: %w", err)
					}

					privateClusterExternalDns := &v1alpha1.ClusterExternalDNS{
						ObjectMeta: metav1.ObjectMeta{
							Name: "private-wi",
						},
						TypeMeta: metav1.TypeMeta{
							Kind:       "ClusterExternalDNS",
							APIVersion: v1alpha1.GroupVersion.String(),
						},
						Spec: v1alpha1.ClusterExternalDNSSpec{
							ResourceName:       "wi-private-cluster-external-dns",
							DNSZoneResourceIDs: []string{in.ManagedIdentityPrivateZones[0].Zone.GetId()},
							ResourceTypes:      []string{"ingress"},
							Identity: v1alpha1.ExternalDNSIdentity{
								Type:           v1alpha1.IdentityTypeWorkloadIdentity,
								ServiceAccount: sa.Name,
							},
							ResourceNamespace: ns,
						},
					}
					if err := upsert(ctx, cl, privateClusterExternalDns); err != nil {
						return fmt.Errorf("upserting private cluster external dns: %w", err)
					}

					ingress.Spec.IngressClassName = util.ToPtr(wiNic.Spec.IngressClassName)
					ingress.Annotations["kubernetes.azure.com/tls-cert-service-account"] = sa.GetName()
					return nil
				}, util.ToPtr(service.Name), func(ctx context.Context, c client.Client, namespacer namespacer, operator manifests.OperatorConfig, infra infra.Provisioned, serviceName *string) ([]zoner, error) {
					zs, err := toZoners(ctx, cl, namespacer, infra.ManagedIdentityZones[0])
					if err != nil {
						return nil, fmt.Errorf("getting zoners: %w", err)
					}

					pzs, err := toPrivateZoners(ctx, cl, namespacer, infra.ManagedIdentityPrivateZones[0], in.Cluster.GetDnsServiceIp())
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

type singleNamespacer struct {
	namespace string
}

func (s singleNamespacer) getNamespace(ctx context.Context, cl client.Client, key string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
	}

	if err := upsert(ctx, cl, ns); err != nil {
		return nil, fmt.Errorf("upserting namespace %s: %w", s.namespace, err)
	}

	return ns, nil
}
