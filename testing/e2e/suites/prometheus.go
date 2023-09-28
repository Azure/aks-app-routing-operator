package suites

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
)

var (
	promNs = manifests.UncollisionedNs()
)

func promSuite(in infra.Provisioned) []test {
	return []test{
		{
			name: "ingress prometheus metrics",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.AllOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				withServicePrincipal().
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				c, err := client.New(config, client.Options{})
				if err != nil {
					return fmt.Errorf("creating client: %w")
				}

				lgr.Info("creating namespace")
				if err := upsert(ctx, c, promNs); err != nil {
					return fmt.Errorf("creating namespace: %w", err)
				}
				lgr = lgr.With("namespace", promNs.Name)
				ctx = logger.WithContext(ctx, lgr)

				resources := manifests.PrometheusClientAndServer(promNs.Name, "prometheus")
				for _, object := range resources.Objects() {
					if err := upsert(ctx, c, object); err != nil {
						return fmt.Errorf("upserting resource: %w", err)
					}
				}

				lgr = lgr.With("client", resources.Client.Name)
				ctx = logger.WithContext(ctx, lgr)
				lgr.Info("waiting for prometheus client to be ready")
				if err := waitForAvailable(ctx, c, *resources.Client); err != nil {
					return fmt.Errorf("waiting for prometheus client to be ready: %w", err)
				}

				lgr.Info("finished testing prometheus")
				return nil
			},
		},
	}
}
