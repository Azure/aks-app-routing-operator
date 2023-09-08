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

var (
	publicBasicIngressNs = manifests.UncollisionedNs() // using a shared ns allows us to appropriately test upgrade scenarios
)

func basicSuite(infra infra.Provisioned) []test {
	return []test{
		{
			name: "public basic ingress",
			cfgs: builderFromInfra(infra).
				withOsm(false).
				withVersions(manifests.OperatorVersionLatest, manifests.OperatorVersion0_0_3).
				withZones(
					manifests.DnsZones{
						Public:  manifests.DnsZoneCountOne,
						Private: manifests.DnsZoneCountNone,
					},
					manifests.DnsZones{
						Public:  manifests.DnsZoneCountMultiple,
						Private: manifests.DnsZoneCountNone,
					},
				).
				build(),
			run: func(ctx context.Context, config *rest.Config) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("running basic service")

				c, err := client.New(config, client.Options{})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				lgr.Info("creating namespace")
				if err := upsert(ctx, c, publicBasicIngressNs); err != nil {
					return fmt.Errorf("upserting ns: %w", err)
				}
				lgr = lgr.With("namespace", publicBasicIngressNs.Name)
				ctx = logger.WithContext(ctx, lgr)

				zone := infra.Zones[0]
				testingResources := manifests.ClientAndServer(publicBasicIngressNs.Name, "basic-service-test", zone.GetName(), zone.GetNameservers()[0], infra.Cert.GetId())
				for _, object := range testingResources.Objects() {
					if err := upsert(ctx, c, object); err != nil {
						return fmt.Errorf("upserting resource: %w", err)
					}
				}

				ctx = logger.WithContext(ctx, lgr.With("client", testingResources.Client.GetName(), "clientNamespace", testingResources.Client.GetNamespace()))
				if err := waitForAvailable(ctx, c, *testingResources.Client); err != nil {
					return fmt.Errorf("waiting for client deployment to be available: %w", err)
				}

				lgr.Info("finished running basic service")
				return nil
			},
		},
	}
}
