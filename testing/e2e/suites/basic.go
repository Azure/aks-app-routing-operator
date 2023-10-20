package suites

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
)

var (
	// basicNs is a map of namespaces that are used by the basic suite.
	// the key is the dns zone name, and the value is the namespace that
	// is used for the tests for that dns zone. Using shared namespaces
	// allow us to appropriately test upgrade scenarios.
	basicNs = make(map[string]*corev1.Namespace)
)

func basicSuite(in infra.Provisioned) []test {
	return []test{
		{
			name: "basic ingress",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(in, manifests.AllOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				if err := clientServerTest(ctx, config, operator, basicNs, in, nil); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "basic service",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(in, manifests.AllOperatorVersions...).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				if err := clientServerTest(ctx, config, operator, basicNs, in, func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error {
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

// modifier is a function that can be used to modify the ingress and service
type modifier func(ingress *netv1.Ingress, service *corev1.Service, z zoner) error

// clientServerTest is a test that deploys a client and server application and ensures the client can reach the server.
// This is the standard test used to check traffic flow is working.
var clientServerTest = func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig, namespaces map[string]*corev1.Namespace, prov infra.Provisioned, mod modifier) error {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting test")

	c, err := client.New(config, client.Options{})
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	var zones []zoner
	switch operator.Zones.Public {
	case manifests.DnsZoneCountNone:
	case manifests.DnsZoneCountOne:
		z := prov.Zones[0]
		zones = append(zones, zone{name: z.GetName(), nameserver: z.GetNameservers()[0]})
	case manifests.DnsZoneCountMultiple:
		for _, z := range prov.Zones {
			zones = append(zones, zone{name: z.GetName(), nameserver: z.GetNameservers()[0]})
		}
	}
	if prov.AuthType == infra.AuthTypeServicePrincipal && operator.Zones.Public != manifests.DnsZoneCountNone {
		lgr.Info("hydrating external dns secret")
		externalDnsSecret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sp-creds-external-dns",
				Namespace: "app-routing-system",
			},
			Type: corev1.SecretTypeOpaque,
		}
		prov.Zones[0].GetDnsZone(ctx)
		err = upsertDNSSecret(ctx, c, externalDnsSecret, prov.ServicePrincipal, prov.SubscriptionId, prov.TenantId, prov.ResourceGroup.GetName())
		if err != nil {
			return fmt.Errorf("upserting external DNS secret: %w", err)
		}
	}

	switch operator.Zones.Private {
	case manifests.DnsZoneCountNone:
	case manifests.DnsZoneCountOne:
		z := prov.PrivateZones[0]
		zones = append(zones, zone{name: z.GetName(), nameserver: prov.Cluster.GetDnsServiceIp()})
	case manifests.DnsZoneCountMultiple:
		for _, z := range prov.PrivateZones {
			zones = append(zones, zone{name: z.GetName(), nameserver: prov.Cluster.GetDnsServiceIp()})
		}
	}
	if prov.AuthType == infra.AuthTypeServicePrincipal && operator.Zones.Private != manifests.DnsZoneCountNone {
		lgr.Info("hydrating external dns private secret")
		externalDnsSecret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sp-creds-external-dns-private",
				Namespace: "app-routing-system",
			},
		}
		err = upsertDNSSecret(ctx, c, externalDnsSecret, prov.ServicePrincipal, prov.SubscriptionId, prov.TenantId, prov.ResourceGroup.GetName())
		if err != nil {
			return fmt.Errorf("upserting external DNS secret: %w", err)
		}
	}

	var eg errgroup.Group
	for _, zone := range zones {
		zone := zone
		eg.Go(func() error {
			lgr := logger.FromContext(ctx).With("zone", zone.GetName())
			ctx := logger.WithContext(ctx, lgr)

			if val, ok := namespaces[zone.GetName()]; !ok || val == nil {
				namespaces[zone.GetName()] = manifests.UncollisionedNs()
			}
			ns := namespaces[zone.GetName()]

			lgr.Info("creating namespace")
			if err := upsert(ctx, c, ns); err != nil {
				return fmt.Errorf("upserting ns: %w", err)
			}

			lgr = lgr.With("namespace", ns.Name)
			ctx = logger.WithContext(ctx, lgr)

			testingResources := manifests.ClientAndServer(ns.Name, "e2e-testing", zone.GetName(), zone.GetNameserver(), prov.Cert.GetId())
			for _, object := range testingResources.Objects() {
				if err := upsert(ctx, c, object); err != nil {
					return fmt.Errorf("upserting resource: %w", err)
				}
			}

			// Populate Service Principal credentials if needed
			if prov.AuthType == infra.AuthTypeServicePrincipal {
				lgr.Info("creating service principal secrets")
				sp := prov.ServicePrincipal
				if err != nil {
					return fmt.Errorf("marshalling service principal credentials: %w", err)
				}

				sec := &corev1.Secret{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Secret",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "keyvault-service-principal",
						Namespace: ns.Name,
					},
					Data: map[string][]byte{
						"clientid":     []byte(sp.ApplicationClientID),
						"clientsecret": []byte(sp.ServicePrincipalCredPassword),
					},
				}
				lgr.Info("upserting keyvault service principal secret")
				err := util.Upsert(ctx, c, sec)
				if err != nil {
					return fmt.Errorf("upserting keyvault service principal secret: %w", err)
				}

			}

			if mod != nil {
				if err := mod(testingResources.Ingress, testingResources.Service, zone); err != nil {
					return fmt.Errorf("modifying ingress and service: %w", err)
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

func upsertDNSSecret(ctx context.Context, c client.Client, externalDnsSecret *corev1.Secret, sp clients.ServicePrincipal, subscriptionId, tenantId, resourceGroup string) error {
	lgr := logger.FromContext(ctx)

	lgr.Info("hydrating external dns secret")
	err := hydrateExternalDNSSecret(ctx, externalDnsSecret, sp, subscriptionId, tenantId, resourceGroup)
	if err != nil {
		return fmt.Errorf("hydrating external dns secret: %w", err)
	}
	err = upsert(ctx, c, externalDnsSecret)
	if err != nil {
		return fmt.Errorf("upserting external dns secret: %w", err)
	}
	return nil
}

type ExternalDNSAzureJson struct {
	TenantId        string `json:"tenantId"`
	SubscriptionId  string `json:"subscriptionId"`
	ResourceGroup   string `json:"resourceGroup"`
	AadClientId     string `json:"aadClientId"`
	AadClientSecret string `json:"aadClientSecret"`
}

func hydrateExternalDNSSecret(ctx context.Context, secret *corev1.Secret, sp clients.ServicePrincipal, subscriptionId, tenantId, resourceGroup string) error {
	lgr := logger.FromContext(ctx)

	az := ExternalDNSAzureJson{}
	azureJson := secret.Data["azure.json"]
	if len(azureJson) > 0 {
		err := json.Unmarshal(azureJson, &az)
		if err != nil {
			return fmt.Errorf("unmarshaling externaldns json secret: %w", err)
		}
	}
	az.SubscriptionId = subscriptionId
	az.ResourceGroup = resourceGroup
	az.TenantId = tenantId
	az.AadClientId = sp.ApplicationClientID
	az.AadClientSecret = sp.ServicePrincipalCredPassword

	azureJsonNew, err := json.Marshal(az)
	if err != nil {
		return fmt.Errorf("marshaling azure json for external dns: %w", err)
	}
	if secret.Data == nil {
		lgr.Info("nil data map found in external dns secret, creating new map")
		secret.Data = make(map[string][]byte)
	}
	lgr.Info("writing new azure.json for externaldns secret")
	secret.Data["azure.json"] = azureJsonNew
	return nil
}

type zoner interface {
	GetName() string
	GetNameserver() string
}

type zone struct {
	name       string
	nameserver string
}

func (z zone) GetName() string {
	return z.name
}

func (z zone) GetNameserver() string {
	return z.nameserver
}
