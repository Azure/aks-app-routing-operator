// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package spc

import (
	"context"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	testClientID = "test-client-id"
)

func TestGatewayToSpcOpts(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))

	validServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"azure.workload.identity/client-id": testClientID,
			},
		},
	}

	tests := []struct {
		name               string
		conf               *config.Config
		gateway            *gatewayv1.Gateway
		objects            []client.Object
		wantSpcOpts        []spcOpts
		wantErr            bool
		wantErrStr         string
		verifyModifyOwner  bool
		wantCertificateRef *gatewayv1.SecretObjectReference
	}{
		{
			name: "nil config",
			conf: nil,
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
			},
			wantErr:    true,
			wantErrStr: "config is nil",
		},
		{
			name:       "nil gateway",
			conf:       &config.Config{},
			gateway:    nil,
			wantErr:    true,
			wantErrStr: "gateway is nil",
		},
		{
			name: "unmanaged gateway",
			conf: &config.Config{},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "some-other-class",
				},
			},
			wantSpcOpts: nil,
		},
		{
			name: "managed gateway without listeners",
			conf: &config.Config{},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
				},
			},
			wantSpcOpts: nil,
		},
		{
			name: "managed gateway with listener but no TLS",
			conf: &config.Config{},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "http",
						},
					},
				},
			},
			wantSpcOpts: []spcOpts{
				{
					action:     actionCleanup,
					name:       "kv-gw-cert-test-gateway-http",
					namespace:  "test-ns",
					secretName: "kv-gw-cert-test-gateway-http",
				},
			},
		},
		{
			name: "managed gateway with listener and valid TLS config",
			conf: &config.Config{
				TenantID: "test-tenant-id",
				Cloud:    "AzurePublicCloud",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.net/secrets/test-cert",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects: []client.Object{validServiceAccount},
			wantSpcOpts: []spcOpts{
				{
					action:     actionReconcile,
					name:       "kv-gw-cert-test-gateway-https",
					namespace:  "test-ns",
					clientId:   testClientID,
					tenantId:   "test-tenant-id",
					vaultName:  "test-vault",
					certName:   "test-cert",
					secretName: "kv-gw-cert-test-gateway-https",
					cloud:      "AzurePublicCloud",
				},
			},
		},
		{
			name: "managed gateway with multiple listeners",
			conf: &config.Config{
				TenantID: "test-tenant-id",
				Cloud:    "AzurePublicCloud",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "http",
						},
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.net/secrets/test-cert",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects: []client.Object{validServiceAccount},
			wantSpcOpts: []spcOpts{
				{
					action:     actionCleanup,
					name:       "kv-gw-cert-test-gateway-http",
					namespace:  "test-ns",
					secretName: "kv-gw-cert-test-gateway-http",
					tenantId:   "test-tenant-id",
					cloud:      "AzurePublicCloud",
				},
				{
					action:     actionReconcile,
					name:       "kv-gw-cert-test-gateway-https",
					namespace:  "test-ns",
					clientId:   testClientID,
					tenantId:   "test-tenant-id",
					vaultName:  "test-vault",
					certName:   "test-cert",
					secretName: "kv-gw-cert-test-gateway-https",
					cloud:      "AzurePublicCloud",
				},
			},
		},
		{
			name: "missing service account",
			conf: &config.Config{
				TenantID: "test-tenant-id",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.net/secrets/test-cert",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			wantErr:    true,
			wantErrStr: "serviceaccounts \"test-sa\" not found",
		},
		{
			name: "service account without client ID annotation",
			conf: &config.Config{
				TenantID: "test-tenant-id",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.net/secrets/test-cert",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects: []client.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sa",
						Namespace: "test-ns",
					},
				},
			},
			wantErr:    true,
			wantErrStr: "user-specified service account does not contain WI annotation",
		},
		{
			name: "cert URI without service account",
			conf: &config.Config{
				TenantID: "test-tenant-id",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.net/secrets/test-cert",
								},
							},
						},
					},
				},
			},
			wantErr:    true,
			wantErrStr: "user specified cert URI but no ServiceAccount in a listener",
		},
		{
			name: "service account without cert URI",
			conf: &config.Config{
				TenantID: "test-tenant-id",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects: []client.Object{validServiceAccount},
			wantSpcOpts: []spcOpts{
				{
					action:     actionCleanup,
					name:       "kv-gw-cert-test-gateway-https",
					namespace:  "test-ns",
					tenantId:   "test-tenant-id",
					secretName: "kv-gw-cert-test-gateway-https",
				},
			},
			wantErr: false,
		},
		{
			name: "malformed certificate URI - invalid URL",
			conf: &config.Config{
				TenantID: "test-tenant-id",
				Cloud:    "AzurePublicCloud",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "not-a-url",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects:    []client.Object{validServiceAccount},
			wantErr:    true,
			wantErrStr: "uri path contains too few segments",
		},
		{
			name: "malformed certificate URI - missing certificate name",
			conf: &config.Config{
				TenantID: "test-tenant-id",
				Cloud:    "AzurePublicCloud",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.net/secrets/",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects:    []client.Object{validServiceAccount},
			wantErr:    true,
			wantErrStr: "parsing KeyVault cert URI",
		},
		{
			name: "valid with custom national cloud",
			conf: &config.Config{
				TenantID: "test-tenant-id",
				Cloud:    "AzureChinaCloud",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.cn/secrets/test-cert",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects: []client.Object{validServiceAccount},
			wantSpcOpts: []spcOpts{
				{
					action:     actionReconcile,
					name:       "kv-gw-cert-test-gateway-https",
					namespace:  "test-ns",
					clientId:   testClientID,
					tenantId:   "test-tenant-id",
					vaultName:  "test-vault",
					certName:   "test-cert",
					secretName: "kv-gw-cert-test-gateway-https",
					cloud:      "AzureChinaCloud",
				},
			},
		},
		{
			name: "verify modify owner updates certificate references",
			conf: &config.Config{
				TenantID: "test-tenant-id",
				Cloud:    "AzurePublicCloud",
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
					Listeners: []gatewayv1.Listener{
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									certUriTLSOption: "https://test-vault.vault.azure.net/secrets/test-cert",
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			objects: []client.Object{validServiceAccount},
			wantSpcOpts: []spcOpts{
				{
					action:     actionReconcile,
					name:       "kv-gw-cert-test-gateway-https",
					namespace:  "test-ns",
					clientId:   testClientID,
					tenantId:   "test-tenant-id",
					vaultName:  "test-vault",
					certName:   "test-cert",
					secretName: "kv-gw-cert-test-gateway-https",
					cloud:      "AzurePublicCloud",
				},
			},
			verifyModifyOwner: true,
			wantCertificateRef: &gatewayv1.SecretObjectReference{
				Group:     util.ToPtr(gatewayv1.Group(corev1.GroupName)),
				Kind:      util.ToPtr(gatewayv1.Kind("Secret")),
				Name:      "kv-gw-cert-test-gateway-https",
				Namespace: util.ToPtr(gatewayv1.Namespace("test-ns")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			var gotOpts []spcOpts
			var gotErrs []error

			for opts, err := range gatewayToSpcOpts(context.Background(), client, tt.conf, tt.gateway) {
				if err != nil {
					gotErrs = append(gotErrs, err)
				} else {
					gotOpts = append(gotOpts, opts)
				}
			}

			if tt.wantErr {
				require.NotEmpty(t, gotErrs)
				for _, err := range gotErrs {
					assert.Contains(t, err.Error(), tt.wantErrStr)
				}
				return
			}

			require.Empty(t, gotErrs)
			if tt.wantSpcOpts == nil {
				assert.Empty(t, gotOpts)
				return
			}

			require.Equal(t, len(tt.wantSpcOpts), len(gotOpts))
			for i, want := range tt.wantSpcOpts {
				got := gotOpts[i]

				if tt.verifyModifyOwner && got.modifyOwner != nil {
					// Create a copy of the gateway to test modifyOwner
					gatewayCopy := tt.gateway.DeepCopy()
					err := got.modifyOwner(gatewayCopy)
					require.NoError(t, err)

					// Verify certificate references were updated correctly
					listener := gatewayCopy.Spec.Listeners[i]
					require.NotNil(t, listener.TLS)
					require.NotEmpty(t, listener.TLS.CertificateRefs)
					assert.Equal(t, *tt.wantCertificateRef, listener.TLS.CertificateRefs[0])
				}

				// Clear modifyOwner for comparison
				hasModifyOwner := got.modifyOwner != nil
				got.modifyOwner = nil
				want.modifyOwner = nil
				assert.Equal(t, want, got)

				// If this was a reconcile action and TLS was configured, verify modifyOwner was set
				if want.action == actionReconcile && tt.gateway.Spec.Listeners[i].TLS != nil {
					assert.True(t, hasModifyOwner)
				}
			}
		})
	}
}

func TestIsManagedGateway(t *testing.T) {
	tests := []struct {
		name    string
		gateway *gatewayv1.Gateway
		want    bool
	}{
		{
			name: "managed gateway",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: istioGatewayClassName,
				},
			},
			want: true,
		},
		{
			name: "unmanaged gateway",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "some-other-class",
				},
			},
			want: false,
		},
		{
			name:    "nil gateway",
			gateway: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsManagedGateway(tt.gateway)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetGatewayListenerSpcName(t *testing.T) {
	tests := []struct {
		name         string
		gwName       string
		listenerName string
		want         string
	}{
		{
			name:         "short names",
			gwName:       "my-gateway",
			listenerName: "https",
			want:         "kv-gw-cert-my-gateway-https",
		},
		{
			name:         "long names",
			gwName:       strings.Repeat("a", 300),
			listenerName: strings.Repeat("a", 300),
			want:         "kv-gw-cert-" + strings.Repeat("a", 253-len("kv-gw-cert-")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetGatewayListenerSpcName(tt.gwName, tt.listenerName)
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), 253, "name should not exceed k8s name length limit")
		})
	}
}

func TestListenerIsKvEnabled(t *testing.T) {
	tests := []struct {
		name     string
		listener gatewayv1.Listener
		want     bool
	}{
		{
			name: "enabled with cert URI",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						certUriTLSOption: "https://vault.azure.net/secrets/cert",
					},
				},
			},
			want: true,
		},
		{
			name: "disabled without cert URI",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
				},
			},
			want: false,
		},
		{
			name:     "disabled without TLS",
			listener: gatewayv1.Listener{},
			want:     false,
		},
		{
			name: "disabled with nil options",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListenerIsKvEnabled(tt.listener)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestServiceAccountFromListener(t *testing.T) {
	tests := []struct {
		name     string
		listener gatewayv1.Listener
		want     string
	}{
		{
			name: "service account specified",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						"kubernetes.azure.com/tls-cert-service-account": "test-sa",
					},
				},
			},
			want: "test-sa",
		},
		{
			name: "no service account",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
				},
			},
			want: "",
		},
		{
			name:     "no TLS config",
			listener: gatewayv1.Listener{},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ServiceAccountFromListener(tt.listener)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetServiceAccountClientId(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name         string
		sa           *corev1.ServiceAccount
		wantErr      bool
		wantErrStr   string
		wantClientID string
	}{
		{
			name: "valid service account",
			sa: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"azure.workload.identity/client-id": testClientID,
					},
				},
			},
			wantClientID: testClientID,
		},
		{
			name:       "missing service account",
			sa:         nil,
			wantErr:    true,
			wantErrStr: "serviceaccounts \"test-sa\" not found",
		},
		{
			name: "missing annotation",
			sa: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "test-ns",
				},
			},
			wantErr:    true,
			wantErrStr: "user-specified service account does not contain WI annotation",
		},
		{
			name: "empty annotation",
			sa: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"azure.workload.identity/client-id": "",
					},
				},
			},
			wantErr:    true,
			wantErrStr: "user-specified service account does not contain WI annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			if tt.sa != nil {
				require.NoError(t, client.Create(context.Background(), tt.sa))
			}

			got, err := getServiceAccountClientId(context.Background(), client, "test-sa", "test-ns")
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantClientID, got)
		})
	}
}

func TestClientIdFromListener(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	validServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"azure.workload.identity/client-id": testClientID,
			},
		},
	}

	tests := []struct {
		name         string
		listener     gatewayv1.Listener
		objects      []client.Object
		wantErr      bool
		wantErrStr   string
		wantClientID string
	}{
		{
			name: "valid configuration",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						certUriTLSOption:             "https://test-vault.vault.azure.net/secrets/test-cert",
						util.ServiceAccountTLSOption: "test-sa",
					},
				},
			},
			objects:      []client.Object{validServiceAccount},
			wantClientID: testClientID,
		},
		{
			name: "cert URI without service account",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						certUriTLSOption: "https://test-vault.vault.azure.net/secrets/test-cert",
					},
				},
			},
			wantErr:    true,
			wantErrStr: "user specified cert URI but no ServiceAccount in a listener",
		},
		{
			name: "service account without cert URI",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						util.ServiceAccountTLSOption: "test-sa",
					},
				},
			},
			objects:    []client.Object{validServiceAccount},
			wantErr:    true,
			wantErrStr: "user specified ServiceAccount but no cert URI in a listener",
		},
		{
			name: "missing both cert URI and service account",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{},
				},
			},
			wantErr:    true,
			wantErrStr: "none of the required TLS options were specified",
		},
		{
			name: "non-existent service account",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						certUriTLSOption:             "https://test-vault.vault.azure.net/secrets/test-cert",
						util.ServiceAccountTLSOption: "non-existent-sa",
					},
				},
			},
			wantErr:    true,
			wantErrStr: "serviceaccounts \"non-existent-sa\" not found",
		},
		{
			name: "service account without client ID annotation",
			listener: gatewayv1.Listener{
				TLS: &gatewayv1.GatewayTLSConfig{
					Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
						certUriTLSOption:             "https://test-vault.vault.azure.net/secrets/test-cert",
						util.ServiceAccountTLSOption: "test-sa",
					},
				},
			},
			objects: []client.Object{
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-sa",
						Namespace: "test-ns",
					},
				},
			},
			wantErr:    true,
			wantErrStr: "user-specified service account does not contain WI annotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			got, err := clientIdFromListener(context.Background(), client, "test-ns", tt.listener)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrStr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantClientID, got)
		})
	}
}
