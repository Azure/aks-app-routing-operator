package suites

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
)

var (
	spNs = make(map[string]*corev1.Namespace)
)

func servicePrincipalSuite(in infra.Provisioned) []test {
	return []test{
		{
			name: "service principal ingress",
			cfgs: builderFromInfra(in).
				withOsm(in, false).
				withVersions(manifests.AllOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				withServicePrincipal().
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				if err := clientServerTest(ctx, config, operator, spNs, in,
					func(ingress *netv1.Ingress, service *corev1.Service, _ zoner) error {
						annotations := ingress.GetAnnotations()
						ingress.SetAnnotations(annotations)

						return nil
					}); err != nil {
					return err
				}

				return nil
			},
		},
	}
}
