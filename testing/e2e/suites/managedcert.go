package suites

import (
	"context"
	"fmt"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func managedCertCelTests(in infra.Provisioned) []test {
	cfgs := builderFromInfra(in).
		withOsm(in, false, true).
		withVersions(manifests.OperatorVersionLatest).
		withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
		build()

	ret := []test{}
	createTests := []struct {
		name        string
		managedCert *approutingv1alpha1.ManagedCertificate
		shouldError bool
	}{
		{
			name:        "valid managed certificate",
			managedCert: newValidManagedCert(),
			shouldError: false,
		},
		{
			name: "missing target",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.Target = approutingv1alpha1.ManagedCertificateTarget{}
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "missing target secret",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.Target.Secret = ""
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "missing dns zone",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DnsZone = approutingv1alpha1.ManagedCertificateDnsZone{}
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "missing dns zone resource id",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DnsZone.ResourceId = ""
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "malformed dns zone resource id",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DnsZone.ResourceId = "malformed-resource-id"
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "private dns zone",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DnsZone.ResourceId = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Network/privatednszones/zone.com"
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "valid tenant id",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DnsZone.TenantId = "00000000-0000-0000-0000-000000000000"
				return mc
			}(),
			shouldError: false,
		},
		{
			name: "invalid tenant id",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DnsZone.TenantId = "malformed-tenant-id"
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "active directory authority host",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DnsZone.ActiveDirectoryAuthorityHost = "https://login.microsoftonline.com"
				return mc
			}(),
			shouldError: false,
		},
		{
			name: "duplicate domain names",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DomainNames = []string{"zone.com", "zone.com"}
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "empty domain name",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.DomainNames = []string{""}
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "too many domain names",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				maxDomains := 10
				domains := make([]string, maxDomains+1)
				for i := 0; i < maxDomains+1; i++ {
					domains[i] = fmt.Sprintf("%d.zone.com", i)
				}
				mc.Spec.DomainNames = domains
				return mc
			}(),
			shouldError: true,
		},
		{
			name: "missing service account",
			managedCert: func() *approutingv1alpha1.ManagedCertificate {
				mc := newValidManagedCert()
				mc.Spec.ServiceAccount = ""
				return mc
			}(),
			shouldError: true,
		},
	}
	for _, tt := range createTests {
		ret = append(ret, test{
			name: tt.name,
			cfgs: cfgs,
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				sch := runtime.NewScheme()
				if err := approutingv1alpha1.AddToScheme(sch); err != nil {
					return fmt.Errorf("adding approutingv1alpha1 to scheme: %w", err)
				}
				cl, err := client.New(config, client.Options{
					Scheme: sch,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				dryRunCl := client.NewDryRunClient(cl)

				if err := dryRunCl.Create(ctx, tt.managedCert, nil); err != nil {
					if tt.shouldError {
						return nil
					}

					return fmt.Errorf("unexpected error creating unmanaged certificate: %w", err)
				}

				if !tt.shouldError {
					return fmt.Errorf("expected error creating unmanaged certificate")
				}

				return nil
			},
		})
	}

	// confirm immutable fields

	return ret
}

func newValidManagedCert() *approutingv1alpha1.ManagedCertificate {
	return &approutingv1alpha1.ManagedCertificate{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "generated-name",
		},
		Spec: approutingv1alpha1.ManagedCertificateSpec{
			Target: approutingv1alpha1.ManagedCertificateTarget{
				Secret: "secret-name",
			},
			DnsZone: approutingv1alpha1.ManagedCertificateDnsZone{
				ResourceId: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Network/dnszones/zone.com",
			},
			DomainNames:    []string{"zone.com"},
			ServiceAccount: "service-account",
		},
	}
}
