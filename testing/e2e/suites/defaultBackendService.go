package suites

import (
	"context"
	"fmt"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/keyvault"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	dbeScheme           = runtime.NewScheme()
	NGINX_BACKEND_IMAGE = "mcr.microsoft.com/oss/kubernetes/defaultbackend"
	NGINX_BACKEND_TAG   = "v1.20.2"
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

			c, err := client.New(config, client.Options{
				Scheme: dbeScheme,
			})
			if err != nil {
				return fmt.Errorf("creating client: %w")
			}

			// get keyvault uri
			kvuri := in.Zones[0].Cert.GetId()

			// create defaultSSLCert
			defaultSSLCert := v1alpha1.DefaultSSLCertificate{
				KeyVaultURI: &kvuri,
			}

			// create the defaultBackendService

			// create nic and upsert
			testNIC := manifests.NewNginxIngressController("nginx-ingress-controller", "nginxingressclass")
			testNIC.Spec.DefaultSSLCertificate = &defaultSSLCert
			testNIC.Spec.DefaultBackendService = &v1alpha1.NICNamespacedName{"test", "app-routing-system"}
			if err := upsert(ctx, c, testNIC); err != nil {
				return fmt.Errorf("upserting NIC: %w", err)
			}

			var service *v1alpha1.ManagedObjectReference
			var nic v1alpha1.NginxIngressController
			lgr.Info("waiting for NIC to be available")
			if err = wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
				lgr.Info("checking if NIC is available")
				if err := c.Get(ctx, client.ObjectKeyFromObject(testNIC), &nic); err != nil {
					return false, fmt.Errorf("get nic: %w", err)
				}

				for _, cond := range nic.Status.Conditions {
					if cond.Type == v1alpha1.ConditionTypeAvailable {
						lgr.Info("found nic")
						if len(nic.Status.ManagedResourceRefs) == 0 {
							lgr.Info("nic has no ManagedResourceRefs")
							return false, nil
						}
						return true, nil
					}
				}
				lgr.Info("nic not available")
				return false, nil
			}); err != nil {
				return fmt.Errorf("waiting for test NIC to be available: %w", err)
			}

			lgr.Info("checking if associated SPC is created")
			spc := &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      keyvault.DefaultNginxCertName(testNIC),
					Namespace: "app-routing-system",
				},
			}
			cleanSPC := &secv1.SecretProviderClass{}

			if err := c.Get(ctx, client.ObjectKeyFromObject(spc), cleanSPC); err != nil {
				lgr.Info("spc not found")
				return err
			}
			lgr.Info("found spc")

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

			if err := defaultBackendClientServerTest(ctx, config, operator, nil, in, func(nic *v1alpha1.NginxIngressController, service *corev1.Service, z zoner) error {
				nic.Spec.IngressClassName = testNIC.Spec.IngressClassName
				return nil
			}, to.Ptr(service.Name)); err != nil {
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

	c, err := client.New(config, client.Options{})
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

			testingResources := manifests.DefaultBackendClientAndServer(ns.Name, zone.GetName()[:40], zone.GetNameserver(), zone.GetCertId(), zone.GetHost(), zone.GetTlsHost())
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

func newBackendDeployment(contents, namespace, name string) *appsv1.Deployment {
	command := []string{
		"/bin/sh",
		"-c",
		"mkdir source && cd source && go mod init source && echo '" + contents + "' > main.go && go mod tidy && go run main.go",
	}

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				ManagedByKey: ManagedByVal,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: to.Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": name},
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "container",
						Image:   NGINX_BACKEND_IMAGE + ":" + NGINX_BACKEND_TAG,
						Command: command,
					}},
				},
			},
		},
	}
}
