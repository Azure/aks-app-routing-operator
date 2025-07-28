package suites

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func defaultDomainTests(in infra.Provisioned) []test {
	return []test{
		{
			name: "default domain certificate crd rejection tests",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("Running default domain certificate crd rejection tests")

				scheme := runtime.NewScheme()
				v1alpha1.AddToScheme(scheme)
				cl, err := client.New(config, client.Options{Scheme: scheme})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				namespace := "kube-system" // we know this exists

				cases := []struct {
					name          string
					defaultDomain *v1alpha1.DefaultDomainCertificate
					expectedError string
				}{
					{
						name: "missing target",
						defaultDomain: &v1alpha1.DefaultDomainCertificate{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "missing-target",
								Namespace: namespace,
							},
							Spec: v1alpha1.DefaultDomainCertificateSpec{},
						},
						expectedError: "spec.target: Invalid value: 0: spec.target in body should have at least 1 properties",
					},
					{
						name: "target without secret",
						defaultDomain: &v1alpha1.DefaultDomainCertificate{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "target-without-secret",
								Namespace: namespace,
							},
							Spec: v1alpha1.DefaultDomainCertificateSpec{
								Target: v1alpha1.DefaultDomainCertificateTarget{},
							},
						},
						expectedError: "spec.target: Invalid value: 0: spec.target in body should have at least 1 properties",
					},
					{
						name: "target with secret name with invalid characters",
						defaultDomain: &v1alpha1.DefaultDomainCertificate{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-secret-name",
								Namespace: namespace,
							},
							Spec: v1alpha1.DefaultDomainCertificateSpec{
								Target: v1alpha1.DefaultDomainCertificateTarget{
									Secret: util.ToPtr("invalid.secret.name"),
								},
							},
						},
						expectedError: "target.secret: Invalid value",
					},
					{
						name: "target with secret name with capital characters",
						defaultDomain: &v1alpha1.DefaultDomainCertificate{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "invalid-secret-name-caps",
								Namespace: namespace,
							},
							Spec: v1alpha1.DefaultDomainCertificateSpec{
								Target: v1alpha1.DefaultDomainCertificateTarget{
									Secret: util.ToPtr("Capitals"),
								},
							},
						},
						expectedError: "target.secret: Invalid value",
					},
					{
						name: "target with long secret name",
						defaultDomain: &v1alpha1.DefaultDomainCertificate{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "long-secret-name",
								Namespace: namespace,
							},
							Spec: v1alpha1.DefaultDomainCertificateSpec{
								Target: v1alpha1.DefaultDomainCertificateTarget{
									Secret: util.ToPtr(strings.Repeat("a", 64)),
								},
							},
						},
						expectedError: "target.secret: Too long: may not be longer than 63",
					},
				}

				for _, tc := range cases {
					lgr.Info("Running test case", "name", tc.name)
					err := cl.Create(ctx, tc.defaultDomain)
					if err == nil {
						return fmt.Errorf("expected error creating DefaultDomainCertificate %s, but got none", tc.name)
					}

					if !strings.Contains(err.Error(), tc.expectedError) {
						return fmt.Errorf("test case %s: expected error containing %q, but got %q", tc.name, tc.expectedError, err.Error())
					}

					lgr.Info("Test case passed", "name", tc.name, "error", err.Error())
				}

				return nil
			},
		},
	}
}
