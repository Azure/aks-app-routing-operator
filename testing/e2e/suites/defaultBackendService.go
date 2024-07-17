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
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
<<<<<<< HEAD
	dbeScheme            = runtime.NewScheme()
	dbeBasicNS           = make(map[string]*corev1.Namespace)
	dbeServiceName       = "dbeservice"
	ceServiceName        = "ceservice"
	nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
=======
	dbeScheme              = runtime.NewScheme()
	dbeBasicNS             = make(map[string]*corev1.Namespace)
	dbeServiceName         = "dbeservice"
	nonAlphaNumHyphenRegex = regexp.MustCompile(`[^a-zA-Z0-9- ]+`)
	trailingHyphenRegex    = regexp.MustCompile(`^-+|-+$`)
>>>>>>> aamgayle/defaultbackendservice
)

func init() {
	netv1.AddToScheme(dbeScheme)
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
<<<<<<< HEAD
	return []test{
		//{
		//	name: "testing default backend service validity",
		//	cfgs: builderFromInfra(in).
		//		withOsm(in, false, true).
		//		withVersions(manifests.OperatorVersionLatest).
		//		withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
		//		build(),
		//	run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
		//		lgr := logger.FromContext(ctx)
		//		lgr.Info("starting test")
		//
		//		nic := &v1alpha1.NginxIngressController{
		//			TypeMeta: metav1.TypeMeta{
		//				Kind:       "NginxIngressController",
		//				APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
		//			},
		//			ObjectMeta: metav1.ObjectMeta{
		//				Name:      zoneName + "-dbe-nginxingress",
		//				Namespace: zoneNamespace,
		//				Annotations: map[string]string{
		//					manifests.ManagedByKey: manifests.ManagedByVal,
		//				},
		//			},
		//			Spec: v1alpha1.NginxIngressControllerSpec{
		//				IngressClassName:      ingressClassName,
		//				ControllerNamePrefix:  "nginx-" + zoneName[len(zoneName)-7:],
		//				DefaultSSLCertificate: defaultSSLCert,
		//				DefaultBackendService: &v1alpha1.NICNamespacedName{"default-" + zoneName + "-service", zoneNamespace},
		//			},
		//		}
		//		if err := defaultBackendClientServerTest(ctx, config, operator, dbeBasicNS, in, nil, &dbeServiceName); err != nil {
		//			return err
		//		}
		//
		//		lgr.Info("finished testing")
		//		return nil
		//	},
		//},
		{
			name: "testing custom http error validity",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting custom errors test")

				c, err := client.New(config, client.Options{
					Scheme: dbeScheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}
=======
	return []test{{
		name: "testing default backend service validity",
		cfgs: builderFromInfra(in).
			withOsm(in, false, true).
			withVersions(manifests.OperatorVersionLatest).
			withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
			build(),
		run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
			lgr := logger.FromContext(ctx)
			lgr.Info("starting test")

			c, err := client.New(config, client.Options{
				Scheme: dbeScheme,
			})
			if err != nil {
				return fmt.Errorf("creating client: %w", err)
			}

			ingressClassName := "nginxingressclass"
			nic := &v1alpha1.NginxIngressController{
				TypeMeta: metav1.TypeMeta{
					Kind:       "NginxIngressController",
					APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-nginxingress",
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

			var service = &v1alpha1.ManagedObjectReference{}
			lgr.Info("checking for service in managed resource refs")
			for _, ref := range nic.Status.ManagedResourceRefs {
				if ref.Kind == "Service" {
					lgr.Info("found service")
					service = &ref
				}
			}

			if service == nil {
				return fmt.Errorf("no service available in resource refs")
			}

			if err := defaultBackendClientServerTest(ctx, config, operator, dbeBasicNS, in, to.Ptr(service.Name), c, ingressClassName, nic); err != nil {
				return err
			}
>>>>>>> aamgayle/defaultbackendservice

				ingressClassName := "nginxingressclass"
				nic :=
					&v1alpha1.NginxIngressController{
						TypeMeta: metav1.TypeMeta{
							Kind:       "NginxIngressController",
							APIVersion: "approuting.kubernetes.azure.com/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name: "nginxingress",
							Annotations: map[string]string{
								manifests.ManagedByKey: manifests.ManagedByVal,
							},
						},
						Spec: v1alpha1.NginxIngressControllerSpec{
							IngressClassName:     ingressClassName,
							ControllerNamePrefix: "nginx-custom-errors",
							CustomHTTPErrors:     []int{404, 503},
						},
					}
				if err := upsert(ctx, c, nic); err != nil {
					return fmt.Errorf("upserting nic: %w", err)
				}

				var service = &v1alpha1.ManagedObjectReference{}
				lgr.Info("checking for service in managed resource refs")
				for _, ref := range nic.Status.ManagedResourceRefs {
					if ref.Kind == "Service" {
						lgr.Info("found service")
						service = &ref
					}
				}

				if service == nil {
					return fmt.Errorf("no service available in resource refs")
				}

				if err := defaultBackendClientServerTest(ctx, config, operator, dbeBasicNS, in, nil, to.Ptr(service.Name), c, ingressClassName, nic); err != nil {
					return err
				}

				lgr.Info("finished testing")
				return nil
			},
		},
	}
}

<<<<<<< HEAD
// modifier is a function that can be used to modify the ingress and service
type nicModifier func(nic *v1alpha1.NginxIngressController, z zoner) error

var defaultBackendClientServerTest = func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig, namespaces map[string]*corev1.Namespace, infra infra.Provisioned, mod nicModifier, serviceName *string, c client.Client, ingressClassName string, nic *v1alpha1.NginxIngressController) error {
=======
var defaultBackendClientServerTest = func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig, namespaces map[string]*corev1.Namespace, infra infra.Provisioned, serviceName *string, c client.Client, ingressClassName string, nic *v1alpha1.NginxIngressController) error {
>>>>>>> aamgayle/defaultbackendservice
	lgr := logger.FromContext(ctx)
	lgr.Info("starting defaultBackendClientServer test")

	if namespaces == nil {
		namespaces = make(map[string]*corev1.Namespace)
	}
	if serviceName == nil || *serviceName == "" {
<<<<<<< HEAD
=======
		lgr.Info("Using controller name prefix for service name")
>>>>>>> aamgayle/defaultbackendservice
		serviceName = to.Ptr(nic.Spec.ControllerNamePrefix)
	}

	ns, err := getNamespace(ctx, c, namespaces, infra.Zones[0].Zone.GetName())
	if err := upsert(ctx, c, ns); err != nil {
		return fmt.Errorf("initial ns upsert: %w", err)
	}

	var zoners []zoner
	switch operator.Zones.Public {
	case manifests.DnsZoneCountNone:
	case manifests.DnsZoneCountOne:
		zs, err := toZoners(ctx, c, namespaces, infra.Zones[0])
		if err != nil {
			return fmt.Errorf("converting to zoners: %w", err)
		}
		zoners = append(zoners, zs...)
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
		zs, err := toPrivateZoners(ctx, c, namespaces, infra.PrivateZones[0], infra.Cluster.GetDnsServiceIp())
		if err != nil {
			return fmt.Errorf("converting to zoners: %w", err)
		}
		zoners = append(zoners, zs...)
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
			host:       fmt.Sprintf("%s-0.app-routing-system.svc.cluster.local", *serviceName),
		})
	}

	lgr.Info(fmt.Sprintf("Zoner array size: %d", len(zoners)))

	var eg errgroup.Group
	for _, zone := range zoners {
		zone := zone
		eg.Go(func() error {
			lgr := logger.FromContext(ctx).With("zone", zone.GetName())
			ctx := logger.WithContext(ctx, lgr)

			ns, err = getNamespace(ctx, c, namespaces, zone.GetName())
			if err != nil {
				return fmt.Errorf("getting namespace: %w", err)
			}

			lgr = lgr.With("namespace", ns.Name)
			ctx = logger.WithContext(ctx, lgr)

<<<<<<< HEAD
			zoneName := zone.GetName()[:26]
=======
			zoneName := nonAlphaNumHyphenRegex.ReplaceAllString(zone.GetName()[:26], "-")
			zoneName = trailingHyphenRegex.ReplaceAllString(zoneName, "")
>>>>>>> aamgayle/defaultbackendservice
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

<<<<<<< HEAD
			if mod != nil {
				if err := mod(nic, zone); err != nil {
					return fmt.Errorf("modifying nginx ingress controller and service: %w", err)
				}
			}

			if nic.Spec.CustomHTTPErrors != nil && len(nic.Spec.CustomHTTPErrors) > 1 {
				testingResources = manifests.CustomErrorsClientAndServer(zoneNamespace, zoneName, zone.GetNameserver(), zoneKVUri, zoneHost, tlsHost, ingressClassName, serviceName)
				nic.Spec.DefaultBackendService = &v1alpha1.NICNamespacedName{testingResources.Service.Name, testingResources.Service.Namespace}
			} else {
				testingResources = manifests.DefaultBackendClientAndServer(zoneNamespace, zoneName, zone.GetNameserver(), zoneKVUri, zoneHost, tlsHost)
			}

			upsertObjects = append(upsertObjects, testingResources.Objects()...)
			upsertObjects = append(upsertObjects, nic)
			lgr.Info(fmt.Sprintf("upsertObjects size: %d", len(upsertObjects)))
=======
			testingResources = manifests.DefaultBackendClientAndServer(zoneNamespace, zoneName, zone.GetNameserver(), zoneKVUri, ingressClassName, zoneHost, tlsHost)
			nic.Spec.DefaultBackendService = &v1alpha1.NICNamespacedName{"default-" + zoneName + "-service", zoneNamespace}

			upsertObjects = append(upsertObjects, testingResources.Objects()...)
			upsertObjects = append(upsertObjects, nic)
>>>>>>> aamgayle/defaultbackendservice

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
