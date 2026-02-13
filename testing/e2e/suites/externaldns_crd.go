package suites

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const externalDNSTestNamespace = "external-dns-test-ns"

func validExternalDNS() *v1alpha1.ExternalDNS {
	return &v1alpha1.ExternalDNS{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "ExternalDNS",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-no-filters",
			Namespace: externalDNSTestNamespace,
		},
		Spec: v1alpha1.ExternalDNSSpec{
			ResourceName: "test",
			TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
			DNSZoneResourceIDs: []string{
				"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
				"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
			},
			ResourceTypes: []string{"ingress", "gateway"},
			Identity: v1alpha1.ExternalDNSIdentity{
				Type:           v1alpha1.IdentityTypeWorkloadIdentity,
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
				lgr.With("test", "externaldns crd validations")
				lgr.Info("starting test")

				c, err := client.New(config, client.Options{
					Scheme: scheme,
				})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				// Create dedicated test namespace
				testNs := &corev1.Namespace{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: externalDNSTestNamespace,
					},
				}
				if err := upsert(ctx, c, testNs); err != nil {
					return fmt.Errorf("creating test namespace: %w", err)
				}

				tcs := []struct {
					name                 string
					ed                   *v1alpha1.ExternalDNS
					prereqs              []client.Object // objects to create before running test case
					expectedError        error
					expectedWarningEvent *string // controller-level validation failure message
				}{
					{
						name:          "valid",
						ed:            validExternalDNS(),
						expectedError: nil,
					},
					{
						name: "invalid zone ID format",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-zone-id",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/not/a/valid/resource/id/but/has/enough/slashes",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						prereqs: []client.Object{
							&corev1.ServiceAccount{
								TypeMeta: metav1.TypeMeta{
									APIVersion: "v1",
									Kind:       "ServiceAccount",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "test-sa",
									Namespace: externalDNSTestNamespace,
									Annotations: map[string]string{
										"azure.workload.identity/client-id": "test-client-id",
									},
								},
							},
						},
						expectedWarningEvent: to.Ptr("invalid dns zone resource id"),
					},
					{
						name: "serviceaccount does not exist",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "sa-not-exist",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "nonexistent-sa",
								},
							},
						},
						expectedWarningEvent: to.Ptr("serviceAccount nonexistent-sa does not exist in namespace " + externalDNSTestNamespace),
					},
					{
						name: "serviceaccount missing WI annotation",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "sa-missing-wi",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									ServiceAccount: "sa-no-annotation",
								},
							},
						},
						prereqs: []client.Object{
							&corev1.ServiceAccount{
								TypeMeta: metav1.TypeMeta{
									APIVersion: "v1",
									Kind:       "ServiceAccount",
								},
								ObjectMeta: metav1.ObjectMeta{
									Name:      "sa-no-annotation",
									Namespace: externalDNSTestNamespace,
								},
								// No annotations - missing azure.workload.identity/client-id
							},
						},
						expectedWarningEvent: to.Ptr("serviceAccount sa-no-annotation was specified but does not include necessary annotation for workload identity"),
					},
					{
						name: "invalid tenant ID",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-tenant",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("test"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("must be of type uuid"),
					},
					{
						name: "empty tenant ID",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "empty-tenant",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								TenantID:     to.Ptr(""),
								ResourceName: "test",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.tenantId in body should match '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'"),
					},
					{
						name: "nil tenant ID",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "nil-tenant",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
					},
					{
						name: "different subs",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-sub",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174001/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must have the same subscription ID"),
					},
					{
						name: "different types of zones",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-type",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/privatednszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must be of the same resource type"),
					},
					{
						name: "duplicate zones",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "duplicate-zones",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("Duplicate value: \"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test\""),
					},
					{
						name: "different rg",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "diff-rg",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test2/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must have the same resource group"),
					},
					{
						name: "no zones",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-zones",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName:       "test",
								TenantID:           to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{},
								ResourceTypes:      []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.dnsZoneResourceIDs in body should have at least 1 items"),
					},
					{
						name: "no resourcetypes",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-resourcetypes",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("Required value, <nil>: Invalid value: \"null\""),
					},
					{
						name: "empty resourcetypes",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "empty-resourcetypes",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.resourceTypes in body should have at least 1 items"),
					},
					{
						name: "invalid resourcetypes",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-resourcetypes",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "deployment"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("all items must be either 'gateway' or 'ingress'"),
					},
					{
						name: "valid managed identity with gateway",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "valid-msi-gateway",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:     v1alpha1.IdentityTypeManagedIdentity,
									ClientID: "123e4567-e89b-12d3-a456-426614174000",
								},
							},
						},
						expectedError: nil,
					},
					{
						name: "valid managed identity with ingress",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "valid-msi-ingress",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:     v1alpha1.IdentityTypeManagedIdentity,
									ClientID: "123e4567-e89b-12d3-a456-426614174000",
								},
							},
						},
						expectedError: nil,
					},
					{
						name: "managed identity without clientID",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "msi-no-clientid",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type: v1alpha1.IdentityTypeManagedIdentity,
								},
							},
						},
						expectedError: errors.New("clientID is required when type is managedIdentity"),
					},
					{
						name: "managed identity with invalid clientID format",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "msi-invalid-clientid",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:     v1alpha1.IdentityTypeManagedIdentity,
									ClientID: "not-a-valid-uuid",
								},
							},
						},
						expectedError: errors.New("spec.identity.clientID in body should match"),
					},
					{
						name: "workload identity without serviceAccount",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "wi-no-sa",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type: v1alpha1.IdentityTypeWorkloadIdentity,
								},
							},
						},
						expectedError: errors.New("serviceAccount is required when type is workloadIdentity"),
					},
					{
						name: "invalid identity type",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-identity-type",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
								},
								ResourceTypes: []string{"ingress"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           "invalidType",
									ServiceAccount: "test-sa",
								},
							},
						},
						expectedError: errors.New("spec.identity.type: Unsupported value"),
					},
					{
						name: "no identity - defaults to workloadIdentity requiring serviceAccount",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-identity",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
							},
						},
						expectedError: errors.New("serviceAccount is required when type is workloadIdentity"),
					},
					{
						name: "no serviceaccount with default workloadIdentity type",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "no-sa",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity:      v1alpha1.ExternalDNSIdentity{},
							},
						},
						expectedError: errors.New("serviceAccount is required when type is workloadIdentity"),
					},
					{
						name: "valid filters",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{
									GatewayLabelSelector:         to.Ptr("test=test"),
									RouteAndIngressLabelSelector: to.Ptr("testRoute=testRoute"),
								},
							},
						},
					},
					{
						name: "nil filters object",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
							},
						},
					},
					{
						name: "empty filters object with nil filters",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{},
							},
						},
					},
					{
						name: "empty string filters",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{
									GatewayLabelSelector: to.Ptr(""),
								},
							},
						},
						expectedError: errors.New("should match '^[^=]+=[^=]+$'"),
					},
					{
						name: "invalid filters",
						ed: &v1alpha1.ExternalDNS{
							TypeMeta: metav1.TypeMeta{
								APIVersion: v1alpha1.GroupVersion.String(),
								Kind:       "ExternalDNS",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test",
								Namespace: externalDNSTestNamespace,
							},
							Spec: v1alpha1.ExternalDNSSpec{
								ResourceName: "test",
								TenantID:     to.Ptr("123e4567-e89b-12d3-a456-426614174000"),
								DNSZoneResourceIDs: []string{
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test",
									"/subscriptions/123e4567-e89b-12d3-a456-426614174000/resourceGroups/test/providers/Microsoft.network/dnszones/test2",
								},
								ResourceTypes: []string{"ingress", "gateway"},
								Identity: v1alpha1.ExternalDNSIdentity{
									Type:           v1alpha1.IdentityTypeWorkloadIdentity,
									ServiceAccount: "test-sa",
								},
								Filters: &v1alpha1.ExternalDNSFilters{
									GatewayLabelSelector: to.Ptr("bad==filter=="),
								},
							},
						},
						expectedError: errors.New("should match '^[^=]+=[^=]+$'"),
					},
				}

				clientset, err := kubernetes.NewForConfig(config)
				if err != nil {
					return fmt.Errorf("creating kubernetes clientset: %w", err)
				}

				for _, tc := range tcs {
					lgr.Info("running test case", "name", tc.name)

					// Create prerequisite objects
					for _, prereq := range tc.prereqs {
						if err := upsert(ctx, c, prereq); err != nil {
							return fmt.Errorf("for case %s: creating prereq %T %s: %w", tc.name, prereq, prereq.GetName(), err)
						}

						// ensure pre req was created
						sa := &corev1.ServiceAccount{}
						if err := c.Get(ctx, client.ObjectKey{Name: prereq.GetName(), Namespace: prereq.GetNamespace()}, sa); err != nil {
							return fmt.Errorf("for case %s: getting prereq %T %s: %w", tc.name, prereq, prereq.GetName(), err)
						} else {
							lgr.Info("created prereq", "type", fmt.Sprintf("%T", prereq), "name", prereq.GetName(), "annotations", sa.Annotations)
						}
					}

					err := upsert(context.Background(), c, tc.ed)
					if tc.expectedError != nil {
						if err == nil {
							return fmt.Errorf("for case %s expected error: %s", tc.name, tc.expectedError.Error())
						}
						if !strings.Contains(err.Error(), tc.expectedError.Error()) {
							return fmt.Errorf("for case %s expected error: %s, got: %s", tc.name, tc.expectedError.Error(), err.Error())
						}
						continue
					}

					if err != nil {
						return fmt.Errorf("for case %s unexpected error: %s", tc.name, err.Error())
					}

					// Check for expected warning event (controller-level validation)
					if tc.expectedWarningEvent != nil {
						lgr.Info("waiting for warning event", "expectedMessage", *tc.expectedWarningEvent)
						var observedMessages []string
						err := wait.PollImmediate(2*time.Second, 2*time.Minute, func() (bool, error) {
							// Get the ExternalDNS to retrieve its UID
							edns := &v1alpha1.ExternalDNS{}
							if err := c.Get(ctx, client.ObjectKey{Name: tc.ed.Name, Namespace: tc.ed.Namespace}, edns); err != nil {
								return false, fmt.Errorf("getting ExternalDNS: %w", err)
							}
							uid := edns.GetUID()

							// List events by involvedObject.uid
							events, err := clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
								FieldSelector: fmt.Sprintf("involvedObject.uid=%s", uid),
							})
							if err != nil {
								return false, fmt.Errorf("listing events: %w", err)
							}

							observedMessages = nil // reset on each poll
							for _, event := range events.Items {
								observedMessages = append(observedMessages, fmt.Sprintf("[%s] %s: %s", event.Type, event.Reason, event.Message))
								if strings.Contains(event.Message, *tc.expectedWarningEvent) {
									lgr.Info("found expected warning event", "message", event.Message)
									return true, nil
								}
							}
							return false, nil
						})
						if err != nil {
							return fmt.Errorf("for case %s: waiting for warning event containing '%s': %w. Observed events: %v", tc.name, *tc.expectedWarningEvent, err, observedMessages)
						}

						// Clean up the CRD after controller-level validation test
						if err := c.Delete(ctx, tc.ed); err != nil {
							lgr.Info("failed to delete test CRD", "error", err)
						}

						// Clean up prerequisite objects
						for _, prereq := range tc.prereqs {
							if err := client.IgnoreNotFound(c.Delete(ctx, prereq)); err != nil {
								lgr.Info("failed to delete prereq", "type", fmt.Sprintf("%T", prereq), "name", prereq.GetName(), "error", err)
							}
						}
					}
				}
				return nil
			},
		},
	}
}
