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
	cluster, err := infra.Cluster.GetCluster(context.Background())
	if err != nil {
		panic(fmt.Errorf("getting cluster: %w", err))
	}

	return []test{
		{
			name: "public basic ingress",
			cfgs: operatorCfgs{
				{
					Msi:      *cluster.Identity.PrincipalID,
					TenantId: infra.TenantId,
					Location: *cluster.Location,
				},
			}.
				WithAllOsm().
				withPublicZones(manifests.DnsZoneCountOne).
				withVersions(manifests.OperatorVersionLatest, manifests.OperatorVersion0_0_3),
			run: func(ctx context.Context, config *rest.Config) error {
				lgr := logger.FromContext(ctx).With("test", "publicBasicIngress")
				ctx = logger.WithContext(ctx, lgr)
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
				nameservers, err := zone.GetNameservers(ctx)
				if err != nil {
					return fmt.Errorf("getting nameservers: %w", err)
				}

				testingResources := manifests.ClientAndServer(ns.Name, "basic-service-test", zone.GetId(), nameservers[0], infra.Cert.GetId())
				for _, object := range testingResources.Objects() {
					if err := c.Create(ctx, object); err != nil {
						return fmt.Errorf("creating resource: %w", err)
					}
				}

				if err := waitForAvailable(ctx, c, *testingResources.Client); err != nil {
					return fmt.Errorf("waiting for client deployment to be available: %w", err)
				}

				// wait for testingResources.Client to be ready
				lgr.Info("finished running basic service")
				return nil
			},
		},
	}
}
