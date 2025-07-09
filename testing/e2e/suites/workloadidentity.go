package suites

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
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
							GenerateName: "wi-sa",
							Namespace:    ns,
							Annotations: map[string]string{
								"azure.workload.identity/client-id": in.ManagedIdentity.GetClientID(),
							},
						},
					}
					if err := upsert(ctx, cl, sa); err != nil {
						return fmt.Errorf("creating service account: %w", err)
					}

					ingress.Spec.IngressClassName = util.ToPtr(wiNic.Spec.IngressClassName)
					ingress.Annotations["kubernetes.azure.com/tls-cert-service-account"] = sa.GetName()
					return nil
				}, util.ToPtr(service.Name)); err != nil {
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
	}

	if err := upsert(ctx, cl, ns); err != nil {
		return nil, fmt.Errorf("upserting namespace %s: %w", s.namespace, err)
	}

	return ns, nil
}
