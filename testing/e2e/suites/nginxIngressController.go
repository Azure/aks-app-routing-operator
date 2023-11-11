package suites

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	existingOperatorIngressClass = "webapprouting.kubernetes.azure.com"
	testNICIngressClass          = "nginxingressclass"

	scheme = runtime.NewScheme()
)

func init() {
	v1alpha1.AddToScheme(scheme)
	batchv1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	metav1.AddMetaToScheme(scheme)
	appsv1.AddToScheme(scheme)
	policyv1.AddToScheme(scheme)
	rbacv1.AddToScheme(scheme)
}

func nicWebhookTests(in infra.Provisioned) []test {
	return []test{
		{
			name: "nic webhook",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				c, err := client.New(config, client.Options{
					Scheme: scheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w")
				}

				testNIC := manifests.NewNginxIngressController("nginx-ingress-controller", "testNICIngressClass")
				lgr.Info("creating basic NginxIngressController")
				if err := upsert(ctx, c, testNIC); err != nil {
					return fmt.Errorf("creating NginxIngressController: %w", err)
				}

				oldNICName := testNIC.Spec.IngressClassName
				testNIC.Spec.IngressClassName = existingOperatorIngressClass
				lgr.Info("testing existing ingressclass")
				if err := upsert(ctx, c, testNIC); err == nil {
					return fmt.Errorf("created NginxIngressController with existing IngressClass")
				}

				testNIC.Spec.IngressClassName = oldNICName
				if err = c.Delete(ctx, testNIC); err != nil {
					return fmt.Errorf("deleting NginxIngressController: %w", err)
				}

				lgr.Info("finished testing nic webhook")
				return nil
			},
		},
		{
			name: "private ingress",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				c, err := client.New(config, client.Options{
					Scheme: scheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w")
				}

				testNIC := manifests.NewNginxIngressController("default", existingOperatorIngressClass)
				var copyNIC v1alpha1.NginxIngressController
				err = c.Get(ctx, client.ObjectKeyFromObject(testNIC), &copyNIC)
				if err != nil {
					return fmt.Errorf("get default nic: %w", err)
				}

				copyNIC.Spec.LoadBalancerAnnotations = map[string]string{
					"service.beta.kubernetes.io/azure-load-balancer-internal": "true",
				}

				lgr.Info("updating NIC to internal")
				if err := c.Update(ctx, &copyNIC); err != nil {
					return fmt.Errorf("updating NIC to internal: %w", err)
				}

				if err := wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
					lgr.Info("validating service is updated with new annotations")

					var serviceCopy corev1.Service
					if err := c.Get(ctx, client.ObjectKey{Namespace: "app-routing-system", Name: "nginx"}, &serviceCopy); err != nil {
						if apierrors.IsNotFound(err) {
							lgr.Info("nginx service not found")
							return false, nil
						}

						return false, fmt.Errorf("getting nginx service: %w", err)
					}

					if _, ok := serviceCopy.ObjectMeta.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"]; !ok {
						lgr.Info("nginx annotations not found")
						return false, nil
					}

					return true, nil
				}); err != nil {
					return fmt.Errorf("waiting for updated nginx service: %w", err)
				}

				if err := clientServerTest(ctx, config, operator, nil, in, nil); err != nil {
					return err
				}

				err = c.Get(ctx, client.ObjectKeyFromObject(testNIC), &copyNIC)
				if err != nil {
					return fmt.Errorf("get default nic: %w", err)
				}

				copyNIC.Spec.LoadBalancerAnnotations = map[string]string{}
				lgr.Info("reverting NIC to external")
				if err := c.Update(ctx, &copyNIC); err != nil {
					return fmt.Errorf("updating NIC to external: %w", err)
				}

				lgr.Info("finished testing nic webhook")
				return nil
			},
		},
	}
}
