package suites

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginxingress"
	appManifests "github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	osmNs = make(map[string]*corev1.Namespace)
)

func restartNginxDeployment(config *rest.Config) error {
	c, err := client.New(config, client.Options{})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	defaulticc, err := nginxingress.GetDefaultIngressClassControllerClass(c)
	if err != nil {
		return fmt.Errorf("getting default ingress class controller: %w", err)
	}

	nic := nginxingress.GetDefaultNginxIngressController()
	nginxIngressCfg := nginxingress.ToNginxIngressConfig(&nic, defaulticc)

	deploymentLabels := appManifests.GetTopLevelLabels()
	deploymentLabels["app.kubernetes.io/component"] = "ingress-controller"

	var nginxDep appsv1.Deployment

	err = c.Get(context.Background(), client.ObjectKey{Namespace: manifests.ManagedResourceNs, Name: nginxIngressCfg.ResourceName}, &nginxDep)
	if err != nil {
		return fmt.Errorf("getting nginx deployment: %w", err)
	}

	if nginxDep.Spec.Template.ObjectMeta.Annotations == nil {
		nginxDep.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	nginxDep.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	err = c.Update(context.Background(), &nginxDep)
	if err != nil {
		return fmt.Errorf("updating nginx deployment: %w", err)
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
				if operator.DisableOsm {
					return fmt.Errorf("running osm suite without osm enabled")
				}
				osmAnnotationsModifier := func(ingress *netv1.Ingress, service *corev1.Service, _ zoner) error {
					annotations := ingress.GetAnnotations()
					annotations["kubernetes.azure.com/use-osm-mtls"] = "true"
					ingress.SetAnnotations(annotations)

					return nil
				}

				if err := clientServerTest(ctx, config, operator, osmNs, in, osmAnnotationsModifier, nil); err != nil {
					return err
				}

				if err := restartNginxDeployment(config); err != nil {
					return err
				}

				if err := clientServerTest(ctx, config, operator, osmNs, in, osmAnnotationsModifier, nil); err != nil {
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
					annotations["kubernetes.azure.com/tls-cert-keyvault-uri"] = in.Cert.GetId()
					service.SetAnnotations(annotations)

					return nil
				}
				if err := clientServerTest(ctx, config, operator, osmNs, in, applyOsmSvcAnnotations, nil); err != nil {
					return err
				}

				if err := restartNginxDeployment(config); err != nil {
					return err
				}

				if err := clientServerTest(ctx, config, operator, osmNs, in, applyOsmSvcAnnotations, nil); err != nil {
					return err
				}

				return nil
			},
		},
	}
}
