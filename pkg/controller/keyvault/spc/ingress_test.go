// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package spc

import (
	"fmt"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIngressToSpcOpts(t *testing.T) {
	tests := []struct {
		name           string
		conf           *config.Config
		ingress        *netv1.Ingress
		ingressManager util.IngressManager
		wantSpcOpts    *spcOpts
		wantErr        bool
		wantErrString  string
	}{
		{
			name:          "nil config",
			conf:          nil,
			ingress:       &netv1.Ingress{},
			wantErr:       true,
			wantErrString: "config is nil",
		},
		{
			name:          "nil ingress",
			conf:          &config.Config{},
			ingress:       nil,
			wantErr:       true,
			wantErrString: "ingress is nil",
		},
		{
			name: "ingress without keyvault annotation",
			conf: &config.Config{},
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			wantSpcOpts: &spcOpts{
				action:     actionCleanup,
				name:       "keyvault-test-ingress",
				namespace:  "test-ns",
				secretName: "keyvault-test-ingress",
			},
		},
		{
			name: "unmanaged ingress with keyvault annotation",
			conf: &config.Config{},
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test-vault.vault.azure.net/secrets/test-cert",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return false, nil
			}),
			wantSpcOpts: &spcOpts{
				action:     actionCleanup,
				name:       "keyvault-test-ingress",
				namespace:  "test-ns",
				secretName: "keyvault-test-ingress",
			},
		},
		{
			name: "managed ingress with valid keyvault annotation",
			conf: &config.Config{
				MSIClientID: "test-client-id",
				TenantID:    "test-tenant-id",
				Cloud:       "AzurePublicCloud",
			},
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test-vault.vault.azure.net/secrets/test-cert",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			wantSpcOpts: &spcOpts{
				action:     actionReconcile,
				name:       "keyvault-test-ingress",
				namespace:  "test-ns",
				clientId:   "test-client-id",
				tenantId:   "test-tenant-id",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "keyvault-test-ingress",
				cloud:      "AzurePublicCloud",
			},
		},
		{
			name: "managed ingress with invalid keyvault URI",
			conf: &config.Config{
				MSIClientID: "test-client-id",
				TenantID:    "test-tenant-id",
			},
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "invalid-uri",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			wantErr:       true,
			wantErrString: "uri path contains too few segments",
		},
		{
			name: "managed ingress with tls cert managed annotation",
			conf: &config.Config{
				MSIClientID: "test-client-id",
				TenantID:    "test-tenant-id",
			},
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri":     "https://test-vault.vault.azure.net/secrets/test-cert",
						"kubernetes.azure.com/tls-cert-keyvault-managed": "true",
					},
				},
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "test.example.com",
						},
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			wantSpcOpts: &spcOpts{
				action:     actionReconcile,
				name:       "keyvault-test-ingress",
				namespace:  "test-ns",
				clientId:   "test-client-id",
				tenantId:   "test-tenant-id",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "keyvault-test-ingress",
			},
		},
		{
			name: "managed ingress with custom cloud and identity configuration",
			conf: &config.Config{
				MSIClientID: "custom-client-id",
				TenantID:    "custom-tenant-id",
				Cloud:       "AzureChinaCloud",
			},
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test-vault.vault.azure.cn/secrets/test-cert",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			wantSpcOpts: &spcOpts{
				action:     actionReconcile,
				name:       "keyvault-test-ingress",
				namespace:  "test-ns",
				clientId:   "custom-client-id",
				tenantId:   "custom-tenant-id",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "keyvault-test-ingress",
				cloud:      "AzureChinaCloud",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOpts []spcOpts
			var gotErrs []error

			for opts, err := range ingressToSpcOpts(tt.conf, tt.ingress, tt.ingressManager) {
				gotOpts = append(gotOpts, opts)
				gotErrs = append(gotErrs, err)
			}

			if tt.wantErr {
				require.Len(t, gotErrs, 1)
				require.Error(t, gotErrs[0])
				assert.Contains(t, gotErrs[0].Error(), tt.wantErrString)
				return
			}

			require.Len(t, gotOpts, 1)
			if tt.wantSpcOpts != nil {
				// Compare all fields except modifyOwner function
				got := gotOpts[0]
				got.modifyOwner = nil // Clear function for comparison
				assert.Equal(t, *tt.wantSpcOpts, got)
			}

			if tt.ingress != nil && tt.ingress.Annotations["kubernetes.azure.com/tls-cert-managed"] == "true" {
				require.NotNil(t, gotOpts[0].modifyOwner)

				// Test modifyOwner function
				ingress := &netv1.Ingress{}
				err := gotOpts[0].modifyOwner(ingress)
				require.NoError(t, err)
				assert.Equal(t, gotOpts[0].secretName, ingress.Spec.TLS[0].SecretName)
				assert.Equal(t, tt.ingress.Spec.Rules[0].Host, ingress.Spec.TLS[0].Hosts[0])
			}
		})
	}
}

func TestShouldReconcileIngress(t *testing.T) {
	expectedErr := fmt.Errorf("test error")

	tests := []struct {
		name           string
		ingress        *netv1.Ingress
		ingressManager util.IngressManager
		want           bool
		wantErr        bool
		wantErrString  string
	}{
		{
			name:    "nil ingress",
			ingress: nil,
			wantErr: true,
		},
		{
			name: "ingress without annotations",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			want: false,
		},
		{
			name: "unmanaged ingress with keyvault annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test-vault.vault.azure.net/secrets/test-cert",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return false, nil
			}),
			want: false,
		},
		{
			name: "managed ingress with keyvault annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test-vault.vault.azure.net/secrets/test-cert",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			want: true,
		},
		{
			name: "managed ingress without keyvault annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
					Annotations: map[string]string{
						"other": "annotation",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return true, nil
			}),
			want: false,
		},
		{
			name: "error checking if ingress is managed",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "https://test-vault.vault.azure.net/secrets/test-cert",
					},
				},
			},
			ingressManager: util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return false, expectedErr
			}),
			wantErr:       true,
			wantErrString: "checking if ingress test-ingress is managed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShouldReconcileIngress(tt.ingressManager, tt.ingress)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrString)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetIngressSpcName(t *testing.T) {
	tests := []struct {
		name    string
		ingress *netv1.Ingress
		want    string
	}{
		{
			name:    "nil ingress",
			ingress: nil,
			want:    "",
		},
		{
			name: "valid ingress",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
				},
			},
			want: "keyvault-test-ingress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIngressSpcName(tt.ingress)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetIngressCertSecretName(t *testing.T) {
	tests := []struct {
		name    string
		ingress *netv1.Ingress
		want    string
	}{
		{
			name:    "nil ingress",
			ingress: nil,
			want:    "",
		},
		{
			name: "valid ingress",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ingress",
				},
			},
			want: "keyvault-test-ingress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIngressCertSecretName(tt.ingress)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAddTlsRef(t *testing.T) {
	tests := []struct {
		name       string
		obj        client.Object
		secretName string
		wantErr    bool
		wantTLS    []netv1.IngressTLS
	}{
		{
			name: "single host",
			obj: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "test.example.com",
						},
					},
				},
			},
			secretName: "test-secret",
			wantTLS: []netv1.IngressTLS{
				{
					SecretName: "test-secret",
					Hosts:      []string{"test.example.com"},
				},
			},
		},
		{
			name: "multiple hosts",
			obj: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "test1.example.com",
						},
						{
							Host: "test2.example.com",
						},
					},
				},
			},
			secretName: "test-secret",
			wantTLS: []netv1.IngressTLS{
				{
					SecretName: "test-secret",
					Hosts:      []string{"test1.example.com", "test2.example.com"},
				},
			},
		},
		{
			name: "empty host rules",
			obj: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{},
					},
				},
			},
			secretName: "test-secret",
			wantTLS: []netv1.IngressTLS{
				{
					SecretName: "test-secret",
					Hosts:      []string{},
				},
			},
		},
		{
			name: "no rules",
			obj: &netv1.Ingress{
				Spec: netv1.IngressSpec{},
			},
			secretName: "test-secret",
			wantTLS: []netv1.IngressTLS{
				{
					SecretName: "test-secret",
					Hosts:      []string{},
				},
			},
		},
		{
			name:       "non-ingress object",
			obj:        &corev1.Pod{},
			secretName: "test-secret",
			wantErr:    true,
		},
		{
			name: "mixed empty and non-empty hosts",
			obj: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "test1.example.com",
						},
						{},
						{
							Host: "test2.example.com",
						},
					},
				},
			},
			secretName: "test-secret",
			wantTLS: []netv1.IngressTLS{
				{
					SecretName: "test-secret",
					Hosts:      []string{"test1.example.com", "test2.example.com"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addTlsRef(tt.obj, tt.secretName)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			ingress := tt.obj.(*netv1.Ingress)
			assert.Equal(t, tt.wantTLS, ingress.Spec.TLS)
		})
	}
}

func TestModifyOwner(t *testing.T) {
	conf := &config.Config{
		MSIClientID: "test-client-id",
		TenantID:    "test-tenant-id",
		Cloud:       "AzurePublicCloud",
	}
	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"kubernetes.azure.com/tls-cert-keyvault-uri":     "https://test-vault.vault.azure.net/secrets/test-cert",
				"kubernetes.azure.com/tls-cert-keyvault-managed": "true",
			},
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: "test.example.com",
				},
			},
		},
	}
	ingressManager := util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
		return true, nil
	})

	ingressOpts := ingressToSpcOpts(conf, ingress, ingressManager)
	for opts, err := range ingressOpts {
		require.NoError(t, err)
		require.NotNil(t, opts.modifyOwner)

		err = opts.modifyOwner(ingress)
		require.NoError(t, err)

		assert.Equal(t, opts.secretName, ingress.Spec.TLS[0].SecretName)
		assert.Equal(t, []string{"test.example.com"}, ingress.Spec.TLS[0].Hosts)
	}
}
