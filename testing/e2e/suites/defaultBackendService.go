package suites

import (
	"context"
	"fmt"
	"regexp"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	dbScheme               = runtime.NewScheme()
	dbBasicNS              = make(map[string]*corev1.Namespace)
	ceBasicNS              = make(map[string]*corev1.Namespace)
	nonAlphaNumHyphenRegex = regexp.MustCompile(`[^a-zA-Z0-9- ]+`)
	trailingHyphenRegex    = regexp.MustCompile(`^-+|-+$`)
)

func init() {
	netv1.AddToScheme(dbScheme)
	v1alpha1.AddToScheme(dbScheme)
	batchv1.AddToScheme(dbScheme)
	corev1.AddToScheme(dbScheme)
	metav1.AddMetaToScheme(dbScheme)
	appsv1.AddToScheme(dbScheme)
	policyv1.AddToScheme(dbScheme)
	rbacv1.AddToScheme(dbScheme)
	secv1.AddToScheme(dbScheme)
}

func defaultBackendTests(in infra.Provisioned) []test {
	return []test{
		{
			name: "testing default backend service validity",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.AllUsedOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				c, err := client.New(config, client.Options{
					Scheme: dbScheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				ingressClassName := "dbingressclass"
				nic := &v1alpha1.NginxIngressController{
					TypeMeta: metav1.TypeMeta{
						Kind:       "NginxIngressController",
						APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "db-nginxingress",
						Annotations: map[string]string{
							manifests.ManagedByKey: manifests.ManagedByVal,
						},
					},
					Spec: v1alpha1.NginxIngressControllerSpec{
						IngressClassName:     ingressClassName,
						ControllerNamePrefix: "nginx-default-backend",
					},
				}

				if err := upsert(ctx, c, nic); err != nil {
					return fmt.Errorf("upserting nic: %w", err)
				}

				statusNIC, err := waitForNICAvailable(ctx, c, nic)
				if err != nil {
					return fmt.Errorf("waiting for NIC to be available: %w", err)
				}

				lgr.Info("checking for service in managed resource refs")
				service, err := getNginxLbServiceRef(statusNIC)
				if err != nil {
					return fmt.Errorf("getting nginx load balancer service: %w", err)
				}

				if err := defaultBackendClientServerTest(ctx, config, operator, uniqueNamespaceNamespacer{namespaces: dbBasicNS}, in, to.Ptr(service.Name), c, ingressClassName, nic); err != nil {
					return err
				}

				lgr.Info("finished testing")
				return nil
			},
		},
		{
			name: "testing custom http error validity",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.AllUsedOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting custom errors test")

				c, err := client.New(config, client.Options{
					Scheme: dbScheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				ingressClassName := "ceingressclass"
				nic := &v1alpha1.NginxIngressController{
					TypeMeta: metav1.TypeMeta{
						Kind:       "NginxIngressController",
						APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "ce-nginxingress",
						Annotations: map[string]string{
							manifests.ManagedByKey: manifests.ManagedByVal,
						},
					},
					Spec: v1alpha1.NginxIngressControllerSpec{
						IngressClassName:     ingressClassName,
						ControllerNamePrefix: "nginx-custom-errors",
						CustomHTTPErrors:     []int32{404, 503},
					},
				}
				if err := upsert(ctx, c, nic); err != nil {
					return fmt.Errorf("upserting nic: %w", err)
				}

				statusNic, err := waitForNICAvailable(ctx, c, nic)
				if err != nil {
					return fmt.Errorf("waiting for NIC to be available: %w", err)
				}

				lgr.Info("checking for service in managed resource refs")
				service, err := getNginxLbServiceRef(statusNic)
				if err != nil {
					return fmt.Errorf("getting nginx load balancer service: %w", err)
				}

				if err := defaultBackendClientServerTest(ctx, config, operator, uniqueNamespaceNamespacer{namespaces: ceBasicNS}, in, to.Ptr(service.Name), c, ingressClassName, nic); err != nil {
					return err
				}

				lgr.Info("finished testing")
				return nil
			},
		},
	}
}

var defaultBackendClientServerTest = func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig, namespacer namespacer, infra infra.Provisioned, serviceName *string, c client.Client, ingressClassName string, nic *v1alpha1.NginxIngressController) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting defaultBackendClientServer test")

	if serviceName == nil || *serviceName == "" {
		lgr.Info("Using controller name prefix for service name")
		serviceName = to.Ptr(nic.Spec.ControllerNamePrefix)
	}

	ns, err := namespacer.getNamespace(ctx, c, infra.Zones[0].Zone.GetName())
	if err := upsert(ctx, c, ns); err != nil {
		return fmt.Errorf("initial ns upsert: %w", err)
	}

	var zoners []zoner
	switch operator.Zones.Public {
	case manifests.DnsZoneCountNone:
	case manifests.DnsZoneCountOne:
		zs, err := toZoners(ctx, c, namespacer, infra.Zones[0])
		if err != nil {
			return fmt.Errorf("converting to zoners: %w", err)
		}
		zoners = append(zoners, zs...)
	case manifests.DnsZoneCountMultiple:
		for _, z := range infra.Zones {
			zs, err := toZoners(ctx, c, namespacer, z)
			if err != nil {
				return fmt.Errorf("converting to zoners: %w", err)
			}
			zoners = append(zoners, zs...)
		}
	}
	switch operator.Zones.Private {
	case manifests.DnsZoneCountNone:
	case manifests.DnsZoneCountOne:
		zs, err := toPrivateZoners(ctx, c, namespacer, infra.PrivateZones[0], infra.Cluster.GetDnsServiceIp())
		if err != nil {
			return fmt.Errorf("converting to zoners: %w", err)
		}
		zoners = append(zoners, zs...)
	case manifests.DnsZoneCountMultiple:
		for _, z := range infra.PrivateZones {
			zs, err := toPrivateZoners(ctx, c, namespacer, z, infra.Cluster.GetDnsServiceIp())
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

			ns, err = namespacer.getNamespace(ctx, c, zone.GetName())
			if err != nil {
				return fmt.Errorf("getting namespace: %w", err)
			}

			lgr = lgr.With("namespace", ns.Name)
			ctx = logger.WithContext(ctx, lgr)

			zoneName := nonAlphaNumHyphenRegex.ReplaceAllString(zone.GetName()[:23], "-")
			zoneName = trailingHyphenRegex.ReplaceAllString(zoneName, "")
			zoneNamespace := ns.Name
			zoneKVUri := zone.GetCertId()
			zoneHost := zone.GetHost()
			tlsHost := zone.GetTlsHost()

			testingResources := manifests.ClientServerResources{}
			upsertObjects := []client.Object{}

			if (zoneKVUri == "" || zoneKVUri == "null") && nic.Spec.DefaultSSLCertificate == nil {
				nic.Spec.DefaultSSLCertificate = &v1alpha1.DefaultSSLCertificate{
					Secret: &v1alpha1.Secret{
						Name:      zoneName,
						Namespace: zoneNamespace,
					},
				}
			} else {
				nic.Spec.DefaultSSLCertificate = &v1alpha1.DefaultSSLCertificate{
					KeyVaultURI: &zoneKVUri,
				}
			}

			if nic.Spec.CustomHTTPErrors != nil && len(nic.Spec.CustomHTTPErrors) > 1 {
				testingResources = manifests.CustomErrorsClientAndServer(zoneNamespace, zoneName, zone.GetNameserver(), zoneKVUri, zoneHost, tlsHost, ingressClassName, zone.GetCaCertB64(), serviceName)
				nic.Spec.DefaultBackendService = &v1alpha1.NICNamespacedName{Name: testingResources.Service.Name, Namespace: testingResources.Service.Namespace}
			} else {
				testingResources = manifests.DefaultBackendClientAndServer(zoneNamespace, zoneName, zone.GetNameserver(), zoneKVUri, ingressClassName, zoneHost, tlsHost, zone.GetCaCertB64())
				nic.Spec.DefaultBackendService = &v1alpha1.NICNamespacedName{Name: "default-" + zoneName + "-service", Namespace: zoneNamespace}
			}

			upsertObjects = append(upsertObjects, testingResources.Objects()...)
			upsertObjects = append(upsertObjects, nic)

			for _, object := range upsertObjects {
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
