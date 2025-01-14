package suites

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func validExternalDNS() *v1alpha1.ExternalDNS {
	return &v1alpha1.ExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: v1alpha1.ExternalDNSSpec{
			TenantID: "123e4567-e89b-12d3-a456-426614174000",
			DNSZoneResourceIDs: []string{
				"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
				"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
			},
			ResourceTypes: []string{"ingress", "gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				ServiceAccount: "test-sa",
			},
		},
	}
}

func externalDnsCrdTests(in infra.Provisioned) []test {
	return []test{
		{
			name: "externaldns crd validations",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.NonZeroDnsZoneCounts, manifests.NonZeroDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("starting test")

				tcs := []struct {
					name          string
					ed            *v1alpha1.ExternalDNS
					expectedError error
				}{
					{
						name:          "valid",
						ed:            validExternalDNS(),
						expectedError: nil,
					},
					{
						name: "invalid tenant ID",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-tenant",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "test",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("must be of type uuid"),
					},
					{
						name: "empty tenant ID",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-tenant",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("should match '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'"),
					},
					{
						name: "different subs",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-sub",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must have the same subscription ID"),
					},
					{
						name: "different types of zones",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-type",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/privatednszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must be of the same resource type"),
					},
					{
						name: "duplicate zones",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "duplicate-zones",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("Duplicate value: \"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test\""),
					},
					{
						name: "different rg",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-rg",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test2/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must have the same resource group"),
					},
					{
						name: "no zones",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-zones",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID:           "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{},
								ResourceTypes:      []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.dnsZoneResourceIDs in body should have at least 1 items"),
					},
					{
						name: "no resourcetypes",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-resourcetypes",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.dnsZoneResourceIDs in body should have at least 1 items"),
					},
					{
						name: "empty resourcetypes",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "empty-resourcetypes",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.dnsZoneResourceIDs in body should have at least 1 items"),
					},
					{
						name: "invalid resourcetypes",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-resourcetypes",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "deployment"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must be either 'gateway' or 'ingress'"),
					},
					{
						name: "no identity",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-identity",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{},
							},
						},
						expectedError: errors.New(" missing required field \"identity\""),
					},
					{
						name: "no serviceaccount",
						ed: &v1alpha1.ExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-sa",
								Namespace: "default",
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID: "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{},
								Identity:      v1alpha1.ExternalDNSIdentity{},
							},
						},
						expectedError: errors.New("serviceAccount in body should be at least 1 chars long"),
					},
				}

				c, err := client.New(config, client.Options{
					Scheme: scheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w")
				}

				for _, tc := range tcs {
					err := c.Create(context.Background(), tc.ed)
					if tc.expectedError != nil {
						if err == nil {
							return fmt.Errorf("expected error: %s", tc.expectedError.Error())
						}
						if !strings.Contains(err.Error(), tc.expectedError.Error()) {
							return fmt.Errorf("expected error: %s, got: %s", tc.expectedError.Error(), err.Error())
						}

					} else {
						if err != nil {
							return fmt.Errorf("unexpected error: %s", err.Error())
						}
					}
				}
				return nil
			},
		},
	}
}
