package suites

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"
)

var (
	osmNs = make(map[string]*corev1.Namespace)
)

func osmSuite(in infra.Provisioned) []test {
	if in.Name != infra.OsmInfraName {
		return []test{}
	}

	return []test{
		{
			name: "osm ingress",
			cfgs: builderFromInfra(in).
				withOsm(true).
				withVersions(manifests.AllOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				if err := clientServerTest(ctx, config, operator, osmNs, in,
					func(ingress *netv1.Ingress, service *corev1.Service, _ zoner) error {
						annotations := ingress.GetAnnotations()
						annotations["kubernetes.azure.com/use-osm-mtls"] = "true"
						ingress.SetAnnotations(annotations)

						return nil
					}); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "osm service",
			cfgs: builderFromInfra(in).
				withOsm(true).
				withVersions(manifests.AllOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				if err := clientServerTest(ctx, config, operator, osmNs, in,
					func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error {
						ingress = nil
						annotations := service.GetAnnotations()
						annotations["kubernetes.azure.com/ingress-host"] = z.GetNameserver()
						annotations["kubernetes.azure.com/tls-cert-keyvault-uri"] = in.Cert.GetId()
						service.SetAnnotations(annotations)

						return nil
					}); err != nil {
					return err
				}

				return nil
			},
		},
	}
}
