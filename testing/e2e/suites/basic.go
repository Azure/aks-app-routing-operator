package suites

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func basicSuite(infra infra.Provisioned) []test {
	return []test{
		{
			name: "public basic ingress",
			cfgs: operatorCfgs{
				{
					Msi:      infra.Cluster.GetClientId(),
					TenantId: infra.TenantId,
					Location: infra.Cluster.GetLocation(),
				},
			}.
				WithAllOsm().
				withPublicZones(manifests.DnsZoneCountOne).
				withVersions(manifests.OperatorVersionLatest, manifests.OperatorVersion0_0_3),
			run: func(ctx context.Context, config *rest.Config) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("running basic service")

				c, err := client.New(config, client.Options{})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				lgr.Info("creating namespace")
				ns := manifests.UncollisionedNs()
				if err := c.Create(ctx, ns); err != nil {
					return fmt.Errorf("creating ns: %w", err)
				}
				lgr = lgr.With("namespace", ns.Name)
				ctx = logger.WithContext(ctx, lgr)

				zone := infra.Zones[0]
				testingResources := manifests.ClientAndServer(ns.Name, "basic-service-test", zone.GetName(), zone.GetNameservers()[0], infra.Cert.GetId())
				for _, object := range testingResources.Objects() {
					if err := c.Create(ctx, object); err != nil {
						return fmt.Errorf("creating resource: %w", err)
					}
				}

				ctx = logger.WithContext(ctx, lgr.With("client", testingResources.Client.GetName(), "clientNamespace", testingResources.Client.GetNamespace()))
				if err := waitForAvailable(ctx, c, *testingResources.Client); err != nil {
					return fmt.Errorf("waiting for client deployment to be available: %w", err)
				}

				for _, object := range testingResources.Objects() {
					if err := c.Delete(ctx, object); err != nil {
						return fmt.Errorf("deleting resource: %w", err)
					}
				}

				// wait for testingResources.Client to be ready
				lgr.Info("finished running basic service")
				return nil
			},
		},
	}
}
