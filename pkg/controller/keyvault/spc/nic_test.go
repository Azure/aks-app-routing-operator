// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package spc

import (
	"strings"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNicToSpcOpts(t *testing.T) {
	tests := []struct {
		name       string
		conf       *config.Config
		nic        *approutingv1alpha1.NginxIngressController
		wantOpts   *spcOpts
		wantErr    bool
		wantErrStr string
	}{
		{
			name:       "nil config",
			conf:       nil,
			nic:        &approutingv1alpha1.NginxIngressController{},
			wantErr:    true,
			wantErrStr: "config is nil",
		},
		{
			name:       "nil nginx ingress controller",
			conf:       &config.Config{},
			nic:        nil,
			wantErr:    true,
			wantErrStr: "nginx ingress controller is nil",
		},
		{
			name: "valid configuration",
			conf: &config.Config{
				NS:       "test-ns",
				TenantID: "test-tenant",
				Cloud:    "AzurePublicCloud",
			},
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nic",
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: &approutingv1alpha1.DefaultSSLCertificate{
						KeyVaultURI: util.ToPtr("https://test-vault.vault.azure.net/secrets/test-cert"),
					},
				},
			},
			wantOpts: &spcOpts{
				action:     actionReconcile,
				name:       "keyvault-nginx-test-nic",
				namespace:  "test-ns",
				tenantId:   "test-tenant",
				cloud:      "AzurePublicCloud",
				vaultName:  "test-vault",
				certName:   "test-cert",
				secretName: "keyvault-nginx-test-nic",
			},
		},
		{
			name: "invalid keyvault uri",
			conf: &config.Config{
				NS:       "test-ns",
				TenantID: "test-tenant",
				Cloud:    "AzurePublicCloud",
			},
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nic",
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: &approutingv1alpha1.DefaultSSLCertificate{
						KeyVaultURI: util.ToPtr("invalid-uri"),
					},
				},
			},
			wantErr:    true,
			wantErrStr: "uri path contains too few segments",
		},
		{
			name: "should not reconcile - cleanup",
			conf: &config.Config{
				NS:       "test-ns",
				TenantID: "test-tenant",
				Cloud:    "AzurePublicCloud",
			},
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nic",
				},
			},
			wantOpts: &spcOpts{
				action:     actionCleanup,
				name:       "keyvault-nginx-test-nic",
				namespace:  "test-ns",
				tenantId:   "test-tenant",
				cloud:      "AzurePublicCloud",
				secretName: "keyvault-nginx-test-nic",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotOpts []spcOpts
			var gotErr error

			for opts, err := range nicToSpcOpts(tt.conf, tt.nic) {
				if err != nil {
					gotErr = err
					break
				}
				gotOpts = append(gotOpts, opts)
			}

			if tt.wantErr {
				require.Error(t, gotErr)
				assert.Contains(t, gotErr.Error(), tt.wantErrStr)
				return
			}

			require.NoError(t, gotErr)
			require.Len(t, gotOpts, 1)

			// Compare relevant fields
			got := &gotOpts[0]
			assert.Equal(t, tt.wantOpts.action, got.action)
			assert.Equal(t, tt.wantOpts.name, got.name)
			assert.Equal(t, tt.wantOpts.namespace, got.namespace)
			assert.Equal(t, tt.wantOpts.tenantId, got.tenantId)
			assert.Equal(t, tt.wantOpts.cloud, got.cloud)
			assert.Equal(t, tt.wantOpts.vaultName, got.vaultName)
			assert.Equal(t, tt.wantOpts.certName, got.certName)
			assert.Equal(t, tt.wantOpts.secretName, got.secretName)
		})
	}
}

func TestNicDefaultCertName(t *testing.T) {
	tooLongLen := 300

	tests := []struct {
		name string
		nic  *approutingv1alpha1.NginxIngressController
		want string
	}{
		{
			name: "nil nic",
			nic:  nil,
			want: "",
		},
		{
			name: "normal name",
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nic",
				},
			},
			want: "keyvault-nginx-test-nic",
		},
		{
			name: "very long name",
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: strings.Repeat("a", tooLongLen),
				},
			},
			want: "keyvault-nginx-" + strings.Repeat("a", 253-len("keyvault-nginx-")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nicDefaultCertName(tt.nic)
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), 253, "name should not exceed kubernetes name length limit")
		})
	}
}

func TestShouldReconcileNic(t *testing.T) {
	tests := []struct {
		name string
		nic  *approutingv1alpha1.NginxIngressController
		want bool
	}{
		{
			name: "nil nic",
			nic:  nil,
			want: false,
		},
		{
			name: "nil default ssl certificate",
			nic: &approutingv1alpha1.NginxIngressController{
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: nil,
				},
			},
			want: false,
		},
		{
			name: "nil keyvault uri",
			nic: &approutingv1alpha1.NginxIngressController{
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: &approutingv1alpha1.DefaultSSLCertificate{
						KeyVaultURI: nil,
					},
				},
			},
			want: false,
		},
		{
			name: "empty keyvault uri",
			nic: &approutingv1alpha1.NginxIngressController{
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: &approutingv1alpha1.DefaultSSLCertificate{
						KeyVaultURI: util.ToPtr(""),
					},
				},
			},
			want: false,
		},
		{
			name: "valid configuration",
			nic: &approutingv1alpha1.NginxIngressController{
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: &approutingv1alpha1.DefaultSSLCertificate{
						KeyVaultURI: util.ToPtr("https://test-vault.vault.azure.net/secrets/test-cert"),
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldReconcileNic(tt.nic)
			assert.Equal(t, tt.want, got)
		})
	}
}
