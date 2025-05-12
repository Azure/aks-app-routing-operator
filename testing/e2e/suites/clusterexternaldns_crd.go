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
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func validClusterExternalDNS() *v1alpha1.ClusterExternalDNS {
	return &v1alpha1.ClusterExternalDNS{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "ClusterExternalDNS",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: v1alpha1.ClusterExternalDNSSpec{
			ResourceName:      "test",
			ResourceNamespace: "default",
			TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
				withVersions(manifests.AllUsedOperatorVersions...).
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "no-resourcens",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								TenantID: to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
						expectedError: errors.New("resourceNamespace in body should be at least 1 chars long"),
					},
					{
						name: "invalid tenant ID",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "invalid-tenant",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("test"),
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "no-tenant",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								TenantID:          to.Ptr(""),
								ResourceName:      "test",
								ResourceNamespace: "default",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.tenantId in body should match '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'"),
					},
					{
						name: "nil tenant ID",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "nil-tenant",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
					},
					{
						name: "different subs",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "diff-sub",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								ResourceName:      "test",
								ResourceNamespace: "default",
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "diff-type",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "duplicate-zones",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "diff-rg",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "no-zones",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceNamespace:  "default",
								TenantID:           to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "no-resourcetypes",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("Required value, <nil>: Invalid value: \"null\""),
					},
					{
						name: "empty resourcetypes",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "empty-resourcetypes",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "invalid-resourcetypes",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "no-identity",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "deployment"},
							},
						},
						expectedError: errors.New(".identity.serviceAccount: Invalid value: \"\": spec.identity.serviceAccount in body should be at least 1 chars long"),
					},
					{
						name: "no serviceaccount",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "no-sa",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								ResourceName:      "test",
								ResourceNamespace: "default",
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
					{
						name: "valid filters",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "test",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{
									GatewayLabelSelector:         to.Ptr("test=test"),
									RouteAndIngressLabelSelector: to.Ptr("test=test"),
								},
							},
						},
					},
					{
						name: "invalid filters - multiple equals",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "test",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{
									GatewayLabelSelector: to.Ptr("test=tes==t"),
								},
							},
						},
						expectedError: errors.New("should match '^[^=]+=[^=]+$'"),
					},
					{
						name: "invalid filters - ends with equals",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "test",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{
									GatewayLabelSelector: to.Ptr("test="),
								},
							},
						},
						expectedError: errors.New("should match '^[^=]+=[^=]+$'"),
					},
					{
						name: "nil filters object",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "test",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
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
					},
					{
						name: "empty filters object with nil filters",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "test",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{},
							},
						},
					},
					{
						name: "empty string filters",
						ced: &v1alpha1.ClusterExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ClusterExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name: "test",
							},
							Spec: v1alpha1.ClusterExternalDNSSpec{
								ResourceName:      "test",
								ResourceNamespace: "default",
								TenantID:          to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{
									GatewayLabelSelector: to.Ptr(""),
								},
							},
						},
						expectedError: errors.New("should match '^[^=]+=[^=]+$'"),
					},
				}

				c, err := client.New(config, client.Options{
					Scheme: scheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				for _, tc := range tcs {
					err := upsert(context.Background(), c, tc.ced)
					if tc.expectedError != nil {
						if err == nil {
							return fmt.Errorf("for case %s expected error: %s", tc.name, tc.expectedError.Error())
						}
						if !strings.Contains(err.Error(), tc.expectedError.Error()) {
							return fmt.Errorf("for case %s expected error: %s, got: %s", tc.name, tc.expectedError.Error(), err.Error())
						}

					} else {
						// ignore already exists since same cluster is used for multiple tests
						if client.IgnoreAlreadyExists(err) != nil {
							return fmt.Errorf("for case %s unexpected error: %s", tc.name, err.Error())
						}
					}
				}
				return nil
			},
		},
	}
}
