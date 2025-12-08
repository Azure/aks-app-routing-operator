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
	appsv1 "k8s.io/api/apps/v1"
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
				appsv1.AddToScheme(scheme) // Added appsv1 scheme
				cl, err := client.New(config, client.Options{Scheme: scheme})
				if err != nil {
					return fmt.Errorf("creating client: %w", err)
				}

				// deploy the default domain server
				serverName := "default-domain-server"
				serverObjs := manifests.DefaultDomainServer("kube-system", serverName)
				for _, obj := range serverObjs {
					if err := util.Upsert(ctx, cl, obj); err != nil {
						return fmt.Errorf("upserting default domain server object: %w", err)
					}
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

				// wait for the secret to be updated with the new cert
				if err := wait.PollImmediate(1*time.Second, 1*time.Minute, func() (bool, error) {
					// update the secret in the default domain server
					secret := &corev1.Secret{}
					if err := cl.Get(ctx, client.ObjectKey{Name: "default-domain-cert", Namespace: "kube-system"}, secret); err != nil {
						return false, fmt.Errorf("getting default domain secret: %w", err)
					}

					if !bytes.Equal(secret.Data["tls.crt"], newCert) {
						return false, nil
					}
					if !bytes.Equal(secret.Data["tls.key"], newKey) {
						return false, nil
					}

					return true, nil
				}); err != nil {
					return fmt.Errorf("waiting for default domain secret to be updated: %w", err)
				}

				// bounce the pod to pick up the new secret
				podList := &corev1.PodList{}
				if err := cl.List(ctx, podList, client.InNamespace("kube-system"), client.MatchingLabels{"app": serverName}); err != nil {
					return fmt.Errorf("listing default domain server pods: %w", err)
				}
				for _, pod := range podList.Items {
					if err := cl.Delete(ctx, &pod); err != nil {
						return fmt.Errorf("deleting default domain server pod: %w", err)
					}
				}

				lgr.Info("Starting rotation polling")

				// Retry waiting for certificate rotation with timeout
				if err := wait.PollImmediate(5*time.Second, 3*time.Minute, func() (bool, error) {
					lgr.Info("Waiting for certificate rotation to complete")
					if err := cl.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
						return false, fmt.Errorf("getting Secret %s/%s: %w", secret.Namespace, secret.Name, err)
					}

					rotatedCert, ok := secret.Data["tls.crt"]
					if !ok {
						return false, fmt.Errorf("rotated secret does not contain tls.crt data")
					}
					rotatedKey, ok := secret.Data["tls.key"]
					if !ok {
						return false, fmt.Errorf("rotated secret does not contain tls.key data")
					}

					lgr.Info("Validating Rotated Certificate and Key")
					if _, err := tls.ParseTLSCertificate(rotatedCert, rotatedKey); err != nil {
						return true, fmt.Errorf("parsing and verifying TLS certificate: %w", err)
					}

					if bytes.Equal(rotatedCert, tlsCert) {
						lgr.Info("rotated certificate is still the same as the original, retrying")
						return false, nil
					}
					if !bytes.Equal(rotatedCert, newCert) {
						lgr.Info("rotated certificate does not match what was upserted, retrying")
						return false, nil
					}

					if bytes.Equal(rotatedKey, tlsKey) {
						lgr.Info("rotated key is still the same as the original, retrying")
						return false, nil
					}
					if !bytes.Equal(rotatedKey, newKey) {
						lgr.Info("rotated key does not match what was upserted, retrying")
						return false, nil
					}

					return true, nil // Success
				}); err != nil {
					return fmt.Errorf("waiting for certificate rotation: %w", err)
				}

				lgr.Info("Certificate rotation successful")

				lgr.Info("DefaultDomainCertificate happy path test completed successfully")
				return nil
			},
		},
	}
}
