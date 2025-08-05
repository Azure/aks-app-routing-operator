package suites

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/tls"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/manifests"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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
						expectedError: "target.secret: Too long: may not be more than 63",
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
		{
			name: "default domain happy path",
			cfgs: builderFromInfra(in).
				withOsm(in, false, true).
				withVersions(manifests.OperatorVersionLatest).
				withZones(manifests.AllDnsZoneCounts, manifests.AllDnsZoneCounts).
				build(),
			run: func(ctx context.Context, config *rest.Config, operator manifests.OperatorConfig) error {
				lgr := logger.FromContext(ctx)
				lgr.Info("Running default domain happy path")

				scheme := runtime.NewScheme()
				v1alpha1.AddToScheme(scheme)
				corev1.AddToScheme(scheme)
				cl, err := client.New(config, client.Options{Scheme: scheme})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default-domain",
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: corev1.SchemeGroupVersion.String(),
					},
				}
				if err := util.Upsert(ctx, cl, namespace); err != nil {
					return fmt.Errorf("upserting namespace: %w", err)
				}

				secretTarget := "test-secret-target"
				ddc := &v1alpha1.DefaultDomainCertificate{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DefaultDomainCertificate",
						APIVersion: v1alpha1.GroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ddc",
						Namespace: namespace.GetName(),
					},
					Spec: v1alpha1.DefaultDomainCertificateSpec{
						Target: v1alpha1.DefaultDomainCertificateTarget{
							Secret: util.ToPtr(secretTarget),
						},
					},
				}

				if err := util.Upsert(ctx, cl, ddc); err != nil {
					return fmt.Errorf("upserting DefaultDomainCertificate: %w", err)
				}

				lgr.Info("DefaultDomainCertificate created", "name", ddc.Name)
				lgr.Info("Waiting for DefaultDomainCertificate to be available")

				start := time.Now()
				timeout := 30 * time.Second
				sleep := 5 * time.Second
				for {
					if err := cl.Get(ctx, client.ObjectKeyFromObject(ddc), ddc); err != nil {
						return fmt.Errorf("getting DefaultDomainCertificate: %w", err)
					}

					available := ddc.GetCondition(v1alpha1.DefaultDomainCertificateConditionTypeAvailable)
					if available != nil && available.Status == metav1.ConditionTrue {
						lgr.Info("DefaultDomainCertificate is available", "name", ddc.Name)
						break
					}

					if time.Since(start) > timeout {
						return fmt.Errorf("timed out waiting for DefaultDomainCertificate to be available")
					}

					lgr.Info("DefaultDomainCertificate not available yet, waiting", "elapsed", time.Since(start))
					time.Sleep(sleep)
				}

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretTarget,
						Namespace: namespace.GetName(),
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Secret",
						APIVersion: corev1.SchemeGroupVersion.String(),
					},
				}
				if err := cl.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
					return fmt.Errorf("getting Secret %s/%s: %w", secret.Namespace, secret.Name, err)
				}

				tlsCert, ok := secret.Data["tls.crt"]
				if !ok {
					return fmt.Errorf("Secret %s/%s does not contain tls.crt data", secret.Namespace, secret.Name)
				}
				tlsKey, ok := secret.Data["tls.key"]
				if !ok {
					return fmt.Errorf("Secret %s/%s does not contain tls.key data", secret.Namespace, secret.Name)
				}

				lgr.Info("Validating Certificate and Key")
				if _, err := tls.ParseTLSCertificate(tlsCert, tlsKey); err != nil {
					return fmt.Errorf("parsing and verifying TLS certificate: %w", err)
				}

				lgr.Info("Validating Rotation Story")
				newCert, newKey, err := manifests.GenerateSelfSignedCert()
				if err != nil {
					return fmt.Errorf("generating self-signed cert: %w", err)
				}

				dds := manifests.CreateDefaultDomainSecret(newCert, newKey)
				if err := util.Upsert(ctx, cl, dds); err != nil {
					return fmt.Errorf("upserting DefaultDomainSecret: %w", err)
				}

				// Retry waiting for certificate rotation with timeout
				if err := wait.PollImmediate(1*time.Second, 30*time.Second, func() (bool, error) {
					rotatedSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      secretTarget,
							Namespace: namespace.GetName(),
						},
						TypeMeta: metav1.TypeMeta{
							Kind:       "Secret",
							APIVersion: corev1.SchemeGroupVersion.String(),
						},
					}
					if err := cl.Get(ctx, client.ObjectKeyFromObject(rotatedSecret), rotatedSecret); err != nil {
						lgr.Info("failed to get rotated secret, retrying", "error", err.Error())
						return false, nil // Retry on error
					}

					rotatedCert, ok := rotatedSecret.Data["tls.crt"]
					if !ok {
						lgr.Info("rotated secret does not contain tls.crt data, retrying")
						return false, nil // Retry
					}
					rotatedKey, ok := rotatedSecret.Data["tls.key"]
					if !ok {
						lgr.Info("rotated secret does not contain tls.key data, retrying")
						return false, nil // Retry
					}

					// Check if certificate matches the new one
					if !bytes.Equal(rotatedCert, newCert) {
						lgr.Info("rotated certificate does not match the upserted certificate yet, retrying")
						return false, nil // Retry
					}
					if bytes.Equal(rotatedCert, tlsCert) {
						lgr.Info("rotated certificate is still the same as the original, retrying")
						return false, nil // Retry
					}

					// Check if key matches the new one
					if !bytes.Equal(rotatedKey, newKey) {
						lgr.Info("rotated key does not match the upserted key yet, retrying")
						return false, nil // Retry
					}
					if bytes.Equal(rotatedKey, tlsKey) {
						lgr.Info("rotated key is still the same as the original, retrying")
						return false, nil // Retry
					}

					lgr.Info("Validating Rotated Certificate and Key")
					if _, err := tls.ParseTLSCertificate(rotatedCert, rotatedKey); err != nil {
						lgr.Info("failed to parse rotated TLS certificate, retrying", "error", err.Error())
						return false, nil // Retry
					}
				}); err != nil {
					return fmt.Errorf("waiting for certificate rotation: %w", err)
				}

				lgr.Info("Certificate rotation validation successful")

				lgr.Info("DefaultDomainCertificate happy path test completed successfully")
				return nil
			},
		},
	}
}
