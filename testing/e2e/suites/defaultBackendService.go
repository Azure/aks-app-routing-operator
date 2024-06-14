package suites

import (
	"context"
	"fmt"
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	dbeScheme  = runtime.NewScheme()
	dbeBasicNS = make(map[string]*corev1.Namespace)
)

func init() {
	v1alpha1.AddToScheme(dbeScheme)
	batchv1.AddToScheme(dbeScheme)
	corev1.AddToScheme(dbeScheme)
	metav1.AddMetaToScheme(dbeScheme)
	appsv1.AddToScheme(dbeScheme)
	policyv1.AddToScheme(dbeScheme)
	rbacv1.AddToScheme(dbeScheme)
	secv1.AddToScheme(dbeScheme)
}

func defaultBackendTests(in infra.Provisioned) []test {
	return []test{{
		name: "testing default backend validity",
		cfgs: builderFromInfra(in).
			withOsm(in, false, true).
			withVersions(manifests.OperatorVersionLatest).
			withZones([]manifests.DnsZoneCount{manifests.DnsZoneCountNone}, []manifests.DnsZoneCount{manifests.DnsZoneCountNone}).
			build(),
		run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
			lgr := logger.FromContext(ctx)
			lgr.Info("starting test")

			if err := defaultBackendClientServerTest(ctx, config, operator, dbeBasicNS, in, func(ingress *v1alpha1.NginxIngressController, service *corev1.Service, z zoner) error {
				return nil
			}, nil); err != nil {
				return err
			}

			lgr.Info("finished testing")
			return nil
		},
	},
	}
}

// modifier is a function that can be used to modify the ingress and service
type nicModifier func(ingress *v1alpha1.NginxIngressController, service *corev1.Service, z zoner) error

var defaultBackendClientServerTest = func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig, namespaces map[string]*corev1.Namespace, infra infra.Provisioned, mod nicModifier, serviceName *string) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting test")

	if namespaces == nil {
		namespaces = make(map[string]*corev1.Namespace)
	}
	if serviceName == nil {
		serviceName = to.Ptr("nginx")
	}

	c, err := client.New(config, client.Options{
		Scheme: dbeScheme,
	})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	var zoners []zoner
	switch operator.Zones.Public {
	case manifests.DnsZoneCountNone:
	case manifests.DnsZoneCountOne:
		zoners, err := toZoners(ctx, c, namespaces, infra.Zones[0])
		if err != nil {
			return fmt.Errorf("converting to zoners: %w", err)
		}
		zoners = append(zoners, zoners...)
	case manifests.DnsZoneCountMultiple:
		for _, z := range infra.Zones {
			zs, err := toZoners(ctx, c, namespaces, z)
			if err != nil {
				return fmt.Errorf("converting to zoners: %w", err)
			}
			zoners = append(zoners, zs...)
		}
	}
	switch operator.Zones.Private {
	case manifests.DnsZoneCountNone:
	case manifests.DnsZoneCountOne:
		zoners, err := toPrivateZoners(ctx, c, namespaces, infra.PrivateZones[0], infra.Cluster.GetDnsServiceIp())
		if err != nil {
			return fmt.Errorf("converting to zoners: %w", err)
		}
		zoners = append(zoners, zoners...)
	case manifests.DnsZoneCountMultiple:
		for _, z := range infra.PrivateZones {
			zs, err := toPrivateZoners(ctx, c, namespaces, z, infra.Cluster.GetDnsServiceIp())
			if err != nil {
				return fmt.Errorf("converting to zoners: %w", err)
			}
			zoners = append(zoners, zs...)
		}
	}

	if operator.Zones.Public == manifests.DnsZoneCountNone && operator.Zones.Private == manifests.DnsZoneCountNone {
		zoners = append(zoners, zone{
			name:       fmt.Sprintf("%s.app-routing-system.svc.cluster.local:80", *serviceName),
			nameserver: infra.Cluster.GetDnsServiceIp(),
			host:       fmt.Sprintf("%s.app-routing-system.svc.cluster.local", *serviceName),
		})
	}

	var eg errgroup.Group
	for _, zone := range zoners {
		zone := zone
		eg.Go(func() error {
			lgr := logger.FromContext(ctx).With("zone", zone.GetName())
			ctx := logger.WithContext(ctx, lgr)

			ns, err := getNamespace(ctx, c, namespaces, zone.GetName())
			if err != nil {
				return fmt.Errorf("getting namespace: %w", err)
			}

			lgr = lgr.With("namespace", ns.Name)
			ctx = logger.WithContext(ctx, lgr)

			lgr.Info(fmt.Sprintf("Keyvault URI Value for defaultBackendService: %s\n", zone.GetCertId()))
			testingResources := manifests.DefaultBackendClientAndServer(ns.Name, zone.GetName()[:40], zone.GetNameserver(), zone.GetHost(), zone.GetTlsHost())
			if mod != nil {
				if err := mod(testingResources.NginxIngressController, testingResources.Service, zone); err != nil {
					return fmt.Errorf("modifying ingress and service: %w", err)
				}
			}
			for _, object := range testingResources.Objects() {
				if err := upsert(ctx, c, object); err != nil {
					return fmt.Errorf("upserting resource: %w", err)
				}
			}

			ctx = logger.WithContext(ctx, lgr.With("client", testingResources.Client.GetName(), "clientNamespace", testingResources.Client.GetNamespace()))
			if err := waitForAvailable(ctx, c, *testingResources.Client); err != nil {
				return fmt.Errorf("waiting for client deployment to be available: %w", err)
			}

			lgr.Info("finished testing zone")
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("testing all zones: %w", err)
	}

	lgr.Info("finished successfully")
	return nil
}
