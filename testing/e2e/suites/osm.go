package suites

import (
	"context"
	"fmt"

	appManifests "github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var osmNs = make(map[string]*corev1.Namespace)

func deleteNginxPods(config *rest.Config) error {
	c, err := client.New(config, client.Options{})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	podLabels := appManifests.AddComponentLabel(appManifests.GetTopLevelLabels(), appManifests.IngressControllerComponentName)
	if err := c.DeleteAllOf(context.Background(), &corev1.Pod{}, client.InNamespace(manifests.ManagedResourceNs), client.MatchingLabels(podLabels)); err != nil {
		return fmt.Errorf("deleting nginx pods: %w", err)
	}

	return nil
}

func osmSuite(in infra.Provisioned) []test {
	return []test{
		{
			name: "osm ingress",
			cfgs: builderFromInfra(in).
				withOsm(in, true).
				withVersions(manifests.AllUsedOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				osmAnnotationsModifier := func(ingress *netv1.Ingress, service *corev1.Service, _ zoner) error {
					annotations := ingress.GetAnnotations()
					annotations["kubernetes.azure.com/use-osm-mtls"] = "true"
					ingress.SetAnnotations(annotations)

					return nil
				}

				if err := clientServerTest(ctx, config, operator, uniqueNamespaceNamespacer{namespaces: osmNs}, in, osmAnnotationsModifier, nil, getZoners); err != nil {
					return err
				}

				if err := deleteNginxPods(config); err != nil {
					return err
				}

				if err := clientServerTest(ctx, config, operator, uniqueNamespaceNamespacer{namespaces: osmNs}, in, osmAnnotationsModifier, nil, getZoners); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "osm service",
			cfgs: builderFromInfra(in).
				withOsm(in, true).
				withVersions(manifests.AllUsedOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				applyOsmSvcAnnotations := func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error {
					ingress = nil
					annotations := service.GetAnnotations()
					annotations["kubernetes.azure.com/ingress-host"] = z.GetNameserver()
					annotations["kubernetes.azure.com/tls-cert-keyvault-uri"] = z.GetCertId()
					service.SetAnnotations(annotations)

					return nil
				}
				if err := clientServerTest(ctx, config, operator, uniqueNamespaceNamespacer{namespaces: osmNs}, in, applyOsmSvcAnnotations, nil, getZoners); err != nil {
					return err
				}

				if err := deleteNginxPods(config); err != nil {
					return err
				}

				if err := clientServerTest(ctx, config, operator, uniqueNamespaceNamespacer{namespaces: osmNs}, in, applyOsmSvcAnnotations, nil, getZoners); err != nil {
					return err
				}

				return nil
			},
		},
	}
}
