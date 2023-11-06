package suites

import (
	"context"
	"fmt"
	
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
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
}

func nicWebhookTests(in infra.Provisioned) []test {
	return []test{
		{
			name: "nic webhook",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				c, err := client.New(config, client.Options{
					Scheme: nil,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w")
				}

				testNIC := manifests.NewNginxIngressController(testNICIngressClass)
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
	}
}
