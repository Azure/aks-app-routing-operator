// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

type testOwner struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (t *testOwner) GetObjectKind() schema.ObjectKind { return t }
func (t *testOwner) DeepCopyObject() runtime.Object   { return t.DeepCopy() }
func (t *testOwner) DeepCopy() *testOwner {
	return &testOwner{
		TypeMeta:   t.TypeMeta,
		ObjectMeta: *t.ObjectMeta.DeepCopy(),
	}
}

func TestSpcOwnerStructIsOwner(t *testing.T) {
	tests := []struct {
		name string
		spc  *secv1.SecretProviderClass
		kind string
		want bool
	}{
		{
			name: "matching owner",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "TestKind",
							Name: "test-owner",
						},
					},
				},
			},
			kind: "TestKind",
			want: true,
		},
		{
			name: "no matching owner",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "OtherKind",
							Name: "test-owner",
						},
					},
				},
			},
			kind: "TestKind",
			want: false,
		},
		{
			name: "empty owner references",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{},
			},
			kind: "TestKind",
			want: false,
		},
		{
			name: "nil spc",
			spc:  nil,
			kind: "TestKind",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := spcOwnerStruct[*testOwner]{
				kind: tt.kind,
			}
			got := owner.IsOwner(tt.spc)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSpcOwnerStructGetObject(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, secv1.AddToScheme(scheme))

	tests := []struct {
		name      string
		spc       *secv1.SecretProviderClass
		objects   []client.Object
		want      *testOwner
		wantError error
	}{
		{
			name: "object exists",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "TestKind",
							Name: "test-owner",
						},
					},
				},
			},
			objects: []client.Object{
				&testOwner{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
			want: &testOwner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-owner",
					Namespace: "test-ns",
				},
			},
		},
		{
			name: "object not found",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "TestKind",
							Name: "missing-owner",
						},
					},
				},
			},
			wantError: noSpcOwnerErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := spcOwnerStruct[*testOwner]{
				kind:      "TestKind",
				namespace: func(obj *testOwner) string { return "test-ns" },
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			got, err := owner.GetObject(context.Background(), client, tt.spc)
			if tt.wantError != nil {
				assert.True(t, errors.Is(err, tt.wantError))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.GetName(), got.GetName())
			assert.Equal(t, tt.want.GetNamespace(), got.GetNamespace())
		})
	}
}

func TestNicSpcOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	tests := []struct {
		name             string
		nic              *v1alpha1.NginxIngressController
		spc              *secv1.SecretProviderClass
		wantReconcile    bool
		wantServiceAcct  string
		wantServiceError bool
	}{
		{
			name: "should reconcile - valid config",
			nic: &v1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nic",
				},
				Spec: v1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: &v1alpha1.DefaultSSLCertificate{
						KeyVaultURI: util.ToPtr("https://test-kv.vault.azure.net/secrets/test-cert"),
					},
				},
			},
			wantReconcile: true,
		},
		{
			name: "should not reconcile - no cert",
			nic: &v1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-nic",
				},
			},
			wantReconcile: false,
		},
		{
			name:          "nil nic",
			nic:           nil,
			wantReconcile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.nic).
				Build()

			reconcile, err := nicSpcOwner.ShouldReconcile(&secv1.SecretProviderClass{}, tt.nic)
			require.NoError(t, err)
			assert.Equal(t, tt.wantReconcile, reconcile)

			sa, err := nicSpcOwner.GetServiceAccountName(context.Background(), client, &secv1.SecretProviderClass{}, tt.nic)
			if tt.wantServiceError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantServiceAcct, sa)
			assert.Equal(t, nicSpcOwner.kind, "NginxIngressController")
			assert.Equal(t, nicSpcOwner.namespace(tt.nic), "")
			assert.Equal(t, nicSpcOwner.ownerNameAnnotation, "kubernetes.azure.com/nginx-ingress-controller-owner")
		})
	}
}

func TestGatewaySpcOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, gatewayv1.Install(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	tests := []struct {
		name             string
		gateway          *gatewayv1.Gateway
		spc              *secv1.SecretProviderClass
		serviceAccount   *testOwner
		wantReconcile    bool
		wantServiceAcct  string
		wantServiceError bool
	}{
		{
			name: "should not reconcile - unmanaged gateway",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "other-class",
				},
			},
			wantReconcile: false,
		},
		{
			name: "should not reconcile - kv not enabled listener",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-gateway",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "webapprouting.kubernetes.azure.com/gateway-controller-azure-alb-istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
						},
					},
				},
			},
			wantReconcile: false,
		},
		{
			name: "valid service account and listener, managed gateway",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kv-gw-cert-test-gateway-test-listener",
				},
			},
			serviceAccount: &testOwner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sa",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"azure.workload.identity/client-id": "test-client-id",
					},
				},
			},
			wantReconcile:   true,
			wantServiceAcct: "test-sa",
		},
		{
			name: "valid service account and listener, unmanaged gateway",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "notistio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "test-sa",
								},
							},
						},
					},
				},
			},
			wantReconcile: false,
		},
		{
			name: "missing service account",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "webapprouting.kubernetes.azure.com/gateway-controller-azure-alb-istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "test-listener",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "missing-sa",
								},
							},
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kv-gw-cert-test-gateway-test-listener",
				},
			},
			wantServiceError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{tt.gateway}
			if tt.serviceAccount != nil {
				objects = append(objects, tt.serviceAccount)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			reconcile, err := gatewaySpcOwner.ShouldReconcile(tt.spc, tt.gateway)
			require.NoError(t, err)
			assert.Equal(t, tt.wantReconcile, reconcile)

			sa, err := gatewaySpcOwner.GetServiceAccountName(context.Background(), client, tt.spc, tt.gateway)
			if tt.wantServiceError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantServiceAcct, sa)
		})
	}
}

func TestGetIngressSpcOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, netv1.AddToScheme(scheme))

	mockIngressManager := &util.MockIngressManager{
		IsManagedFunc: func(ing *netv1.Ingress) (bool, error) {
			return ing.Annotations != nil && ing.Annotations["test"] == "true", nil
		},
	}

	tests := []struct {
		name          string
		ingress       *netv1.Ingress
		spc           *secv1.SecretProviderClass
		wantReconcile bool
		wantError     bool
	}{
		{
			name: "should reconcile managed ingress",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"test": "true",
					},
				},
			},
			wantReconcile: true,
		},
		{
			name: "should not reconcile unmanaged ingress",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress",
					Namespace: "test-ns",
				},
			},
			wantReconcile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := getIngressSpcOwner(mockIngressManager)
			reconcile, err := owner.ShouldReconcile(tt.spc, tt.ingress)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReconcile, reconcile)

			// Test that namespace function works
			assert.Equal(t, tt.ingress.Namespace, owner.namespace(tt.ingress))

			// Test service account name is empty (not implemented yet)
			sa, err := owner.GetServiceAccountName(context.Background(), nil, tt.spc, tt.ingress)
			require.NoError(t, err)
			assert.Empty(t, sa)
		})
	}
}
