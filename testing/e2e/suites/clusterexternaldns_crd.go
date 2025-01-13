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

func validClusterExternalDNS() *v1alpha1.ClusterExternalDNS {
	return &v1alpha1.ClusterExternalDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceNamespace: "default",
			TenantID:          "123e4567-e89b-12d3-a456-426614174000",
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

func clusterExternalDnsCrdTests(in infra.Provisioned) []test {
	return []test{
		{
			name: "clusterexternaldns crd validations",
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
					ced           *v1alpha1.ClusterExternalDNS
					expectedError error
				}{
					{
						name:          "valid",
						ced:           validClusterExternalDNS(),
						expectedError: nil,
					},
					{
						name: "no resourcenamespace",
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-resourcens",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						},
						expectedError: errors.New("missing required field \"resourceNamespace\""),
					},
					{
						name: "invalid tenant ID",
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-tenant",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceNamespace: "default",
								TenantID:          "test",
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
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-tenant",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("missing required field \"tenantID\""),
					},
					{
						name: "different subs",
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-sub",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-type",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "duplicate-zones",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-rg",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-zones",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								TenantID:           "123e4567-e89b-12d3-a456-426614174000",
								DNSZoneResourceIDs: []string{},
								ResourceTypes:      []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("missing required field \"dnsZoneResourceIDs\""),
					},
					{
						name: "no resourcetypes",
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-resourcetypes",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						expectedError: errors.New("missing required field \"resourceTypes\""),
					},
					{
						name: "empty resourcetypes",
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "empty-resourcetypes",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						expectedError: errors.New("resourceTypes in body should have at least 1 items"),
					},
					{
						name: "invalid resourcetypes",
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-resourcetypes",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-identity",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
						ced: &v1alpha1.ClusterExternalDNS{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-sa",
								Namespace: "default",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
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
					err := c.Create(context.Background(), tc.ced)
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
