package suites

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// basicNs is a map of namespaces that are used by the basic suite.
	// the key is the dns zone name, and the value is the namespace that
	// is used for the tests for that dns zone. Using shared namespaces
	// allow us to appropriately test upgrade scenarios.
	basicNs = make(map[string]*corev1.Namespace)
	// nsMutex is a mutex that is used to protect the basicNs map. Without this we chance concurrent goroutine map access panics
	nsMutex = sync.RWMutex{}
)

func getNamespace(ctx context.Context, cl client.Client, namespaces map[string]*corev1.Namespace, key string) (*corev1.Namespace, error) {
	// multiple goroutines access the same map at the same time which is not safe
	nsMutex.Lock()
	if val, ok := namespaces[key]; !ok || val == nil {
		namespaces[key] = manifests.UncollisionedNs()
	}
	ns := namespaces[key]
	nsMutex.Unlock()

	if err := upsert(ctx, cl, ns); err != nil {
		return nil, fmt.Errorf("upserting ns: %w", err)
	}

	return ns, nil
}

func basicSuite(in infra.Provisioned) []test {
	return []test{
		{
			name: "basic ingress",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.AllUsedOperatorVersions...).
				withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				if err := clientServerTest(ctx, config, operator, basicNs, in, nil, nil); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "basic service",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.AllUsedOperatorVersions...).
				withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				if err := clientServerTest(ctx, config, operator, basicNs, in, func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error {
					ingress = nil
					annotations := service.GetAnnotations()
					annotations["kubernetes.azure.com/ingress-host"] = z.GetName()
					annotations["kubernetes.azure.com/tls-cert-keyvault-uri"] = z.GetCertId()
					service.SetAnnotations(annotations)

					return nil
				}, nil); err != nil {
					return err
				}

				return nil
			},
		},
	}
}

// modifier is a function that can be used to modify the ingress and service
type modifier func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error

// clientServerTest is a test that deploys a client and server application and ensures the client can reach the server.
// This is the standard test used to check traffic flow is working.
var clientServerTest = func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig, namespaces map[string]*corev1.Namespace, infra infra.Provisioned, mod modifier, serviceName *string) error {
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
			zoners, err := toZoners(ctx, c, namespaces, z)
			if err != nil {
				return fmt.Errorf("converting to zoners: %w", err)
			}
			zoners = append(zoners, zoners...)
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
			zoners, err := toPrivateZoners(ctx, c, namespaces, z, infra.Cluster.GetDnsServiceIp())
			if err != nil {
				return fmt.Errorf("converting to zoners: %w", err)
			}
			zoners = append(zoners, zoners...)
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

			testingResources := manifests.ClientAndServer(ns.Name, zone.GetName(), zone.GetNameserver(), zone.GetCertId(), zone.GetHost(), zone.GetTlsHost())
			if mod != nil {
				if err := mod(testingResources.Ingress, testingResources.Service, zone); err != nil {
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

func toZoners(ctx context.Context, cl client.Client, namespaces map[string]*corev1.Namespace, z infra.WithCert[infra.Zone]) ([]zoner, error) {
	name := z.Zone.GetName()
	nameserver := z.Zone.GetNameservers()[0]
	certName := z.Cert.GetName()
	certId := z.Cert.GetId()
	ns, err := getNamespace(ctx, cl, namespaces, name)
	if err != nil {
		return nil, fmt.Errorf("getting namespaces: %w", err)
	}

	return []zoner{
		zone{
			name:       name,
			nameserver: nameserver,
			certName:   certName,
			certId:     certId,
			host:       strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
			tlsHost:    strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
		},
		zone{
			name:       name + "wildcard",
			nameserver: nameserver,
			certName:   certName,
			certId:     certId,
			host:       "wildcard." + strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
			tlsHost:    "*" + strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
		},
	}, nil
}

func toPrivateZoners(ctx context.Context, cl client.Client, namespaces map[string]*corev1.Namespace, z infra.WithCert[infra.PrivateZone], nameserver string) ([]zoner, error) {
	name := z.Zone.GetName()
	certName := z.Cert.GetName()
	certId := z.Cert.GetId()
	ns, err := getNamespace(ctx, cl, namespaces, name)
	if err != nil {
		return nil, fmt.Errorf("getting namespaces: %w", err)
	}

	return []zoner{
		zone{
			name:       name,
			nameserver: nameserver,
			certName:   certName,
			certId:     certId,
			host:       strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
			tlsHost:    strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
		},
		zone{
			name:       name + "wildcard",
			nameserver: nameserver,
			certName:   certName,
			certId:     certId,
			host:       "wildcard." + strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
			tlsHost:    "*" + strings.ToLower(ns.Name) + "." + strings.TrimRight(name, "."),
		},
	}, nil
}

// zoner represents a DNS endpoint and the host, nameserver, and cert information used to connect to it
type zoner interface {
	GetName() string
	GetNameserver() string
	GetCertName() string
	GetCertId() string
	GetHost() string
	GetTlsHost() string
}

type zone struct {
	name       string
	nameserver string
	certName   string
	certId     string
	host       string
	tlsHost    string
}

func (z zone) GetName() string {
	return z.name
}

func (z zone) GetNameserver() string {
	return z.nameserver
}

func (z zone) GetCertName() string {
	return z.certName
}

func (z zone) GetCertId() string {
	return z.certId
}

func (z zone) GetHost() string {
	return z.host
}

func (z zone) GetTlsHost() string {
	return z.tlsHost
}
