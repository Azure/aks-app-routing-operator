package suites

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	netv1 "k8s.io/api/networking/v1"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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

func nicTests(in infra.Provisioned) []test {
	return []test{
		{
			name: "nic validations",
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

				// validate that crd rejected with invalid fields
				testNIC := manifests.NewNginxIngressController("nginx-ingress-controller", "Invalid+Characters")
				lgr.Info("creating NginxIngressController with invalid ingressClassName")
				if err := c.Create(ctx, testNIC); err == nil {
					return fmt.Errorf("able to create NginxIngressController with invalid ingressClassName '%s'", testNIC.Spec.IngressClassName)
				}

				testNIC = manifests.NewNginxIngressController("nginx-ingress-controller", "nginxingressclass")
				testNIC.Spec.ControllerNamePrefix = "Invalid+Characters"
				lgr.Info("creating NginxIngressController with invalid controllerNamePrefix")
				if err := c.Create(ctx, testNIC); err == nil {
					return fmt.Errorf("able to create NginxIngressController with invalid controllerNamePrefix '%s'", testNIC.Spec.ControllerNamePrefix)
				}

				testNIC = manifests.NewNginxIngressController("nginx-ingress-controller", "nginxingressclass")
				testNIC.Spec.DefaultSSLCertificate = &v1alpha1.DefaultSSLCertificate{
					Secret: &v1alpha1.Secret{
						Name:      "Invalid+@Name",
						Namespace: "validnamespace",
					},
				}
				lgr.Info("creating NginxIngressController with invalid Secret field")
				if err := c.Create(ctx, testNIC); err == nil {
					return fmt.Errorf("able to create NginxIngressController despite invalid Secret Name'%s'", testNIC.Spec.ControllerNamePrefix)
				}

				testNIC = manifests.NewNginxIngressController("nginx-ingress-controller", "nginxingressclass")
				testNIC.Spec.DefaultSSLCertificate = &v1alpha1.DefaultSSLCertificate{
					Secret: &v1alpha1.Secret{
						Name:      "validname",
						Namespace: "Invalid+@Namespace",
					},
				}
				lgr.Info("creating NginxIngressController with invalid Secret field")
				if err := c.Create(ctx, testNIC); err == nil {
					return fmt.Errorf("able to create NginxIngressController despite invalid Secret Namespace'%s'", testNIC.Spec.ControllerNamePrefix)
				}

				testNIC = manifests.NewNginxIngressController("nginx-ingress-controller", "nginxingressclass")
				testNIC.Spec.DefaultSSLCertificate = &v1alpha1.DefaultSSLCertificate{
					Secret: &v1alpha1.Secret{
						Name:      "validname",
						Namespace: "",
					},
				}
				lgr.Info("creating NginxIngressController with empty Secret field")
				if err := c.Create(ctx, testNIC); err == nil {
					return fmt.Errorf("able to create NginxIngressController despite missing Secret field'%s'", testNIC.Spec.ControllerNamePrefix)
				}

				lgr.Info("finished testing")
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

				privateNic := manifests.NewNginxIngressController("private", "private.ingress.class")
				privateNic.Spec.LoadBalancerAnnotations = map[string]string{
					"service.beta.kubernetes.io/azure-load-balancer-internal": "true",
				}
				if err := upsert(ctx, c, privateNic); err != nil {
					return fmt.Errorf("ensuring private NIC: %w", err)
				}

				var service v1alpha1.ManagedObjectReference
				lgr.Info("waiting for service associated with private NIC to be ready")
				if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
					lgr.Info("checking if private NIC service is ready")
					var nic v1alpha1.NginxIngressController
					if err := c.Get(ctx, client.ObjectKeyFromObject(privateNic), &nic); err != nil {
						return false, fmt.Errorf("get private nic: %w", err)
					}

					if nic.Status.ManagedResourceRefs == nil {
						return false, nil
					}

					for _, ref := range nic.Status.ManagedResourceRefs {
						if ref.Kind == "Service" {
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

				lgr.Info("validating service contains private annotations")
				var serviceCopy corev1.Service
				if err := c.Get(ctx, client.ObjectKey{Namespace: service.Namespace, Name: service.Name}, &serviceCopy); err != nil {
					return fmt.Errorf("getting service: %w", err)
				}

				if _, ok := serviceCopy.ObjectMeta.Annotations["service.beta.kubernetes.io/azure-load-balancer-internal"]; !ok {
					lgr.Error("private nginx annotations not found")
					return errors.New("private nginx annotations not found")
				}

				if err := clientServerTest(ctx, config, operator, nil, in, func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error {
					ingress.Spec.IngressClassName = to.Ptr(privateNic.Spec.IngressClassName)
					return nil
				}, to.Ptr(service.Name)); err != nil {
					return err
				}

				lgr.Info("finished testing")
				return nil
			},
		},
	}
}
