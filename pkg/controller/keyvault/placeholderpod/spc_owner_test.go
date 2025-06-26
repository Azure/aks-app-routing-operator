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
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

const (
	spcTestNamespace         = "test-ns"
	spcTestKind              = "TestKind"
	spcTestGroup             = "testgroup"
	spcTestVersion           = "v1"
	spcTestName              = "test-owner"
	testNicName              = "test-nic"
	testGatewayName          = "test-gateway"
	testListenerName         = "test-listener"
	testIngress              = "test-ingress"
	testServiceAccount       = "test-sa"
	testClientID             = "test-client-id"
	testKVUri                = "https://test-kv.vault.azure.net/secrets/test-cert"
	testIstioGatewayClass    = "istio"
	testTLSServiceAccountKey = "kubernetes.azure.com/tls-cert-service-account"
	testClientIdAnnotation   = "azure.workload.identity/client-id"
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
							Kind: spcTestKind,
							Name: spcTestName,
						},
					},
				},
			},
			kind: spcTestKind,
			want: true,
		},
		{
			name: "no matching owner",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "OtherKind",
							Name: spcTestName,
						},
					},
				},
			},
			kind: spcTestKind,
			want: false,
		},
		{
			name: "empty owner references",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{},
			},
			kind: spcTestKind,
			want: false,
		},
		{
			name: "nil spc",
			spc:  nil,
			kind: spcTestKind,
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
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group:   spcTestGroup,
		Version: spcTestVersion,
		Kind:    spcTestKind,
	}, &testOwner{})
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
							Kind: spcTestKind,
							Name: spcTestName,
						},
					},
				},
			},
			objects: []client.Object{
				&testOwner{
					ObjectMeta: metav1.ObjectMeta{
						Name:      spcTestName,
						Namespace: spcTestNamespace,
					},
				},
			},
			want: &testOwner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      spcTestName,
					Namespace: spcTestNamespace,
				},
			},
		},
		{
			name: "object not found",
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: spcTestKind,
							Name: "missing-owner",
						},
					},
				},
			},
			wantError: spcOwnerNotFoundErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := spcOwnerStruct[*testOwner]{
				kind:      spcTestKind,
				namespace: func(obj *testOwner) string { return spcTestNamespace },
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
					Name: testNicName,
				},
				Spec: v1alpha1.NginxIngressControllerSpec{
					DefaultSSLCertificate: &v1alpha1.DefaultSSLCertificate{
						KeyVaultURI: util.ToPtr(testKVUri),
					},
				},
			},
			wantReconcile: true,
		},
		{
			name: "should not reconcile - no cert",
			nic: &v1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNicName,
				},
			},
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
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name             string
		gateway          *gatewayv1.Gateway
		spc              *secv1.SecretProviderClass
		serviceAccount   *corev1.ServiceAccount
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
					Name: testGatewayName,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "webapprouting.kubernetes.azure.com/gateway-controller-azure-alb-istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: testListenerName,
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
					Name:      testGatewayName,
					Namespace: spcTestNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: testIstioGatewayClass,
					Listeners: []gatewayv1.Listener{
						{
							Name: testListenerName,
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									testTLSServiceAccountKey: testServiceAccount,
								},
							},
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kv-gw-cert-" + testGatewayName + "-" + testListenerName,
				},
			},
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testServiceAccount,
					Namespace: spcTestNamespace,
					Annotations: map[string]string{
						testClientIdAnnotation: testClientID,
					},
				},
			},
			wantReconcile:   true,
			wantServiceAcct: testServiceAccount,
		},
		{
			name: "valid service account and listener, unmanaged gateway",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testGatewayName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "notistio",
					Listeners: []gatewayv1.Listener{
						{
							Name: testListenerName,
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": testServiceAccount,
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
					Name:      testGatewayName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "webapprouting.kubernetes.azure.com/gateway-controller-azure-alb-istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: testListenerName,
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
		{
			name: "multiple listeners - matching listener has service account",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testGatewayName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "http",
						},
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": testServiceAccount,
								},
							},
						},
						{
							Name: "grpc",
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kv-gw-cert-test-gateway-https",
				},
			},
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testServiceAccount,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"azure.workload.identity/client-id": testClientID,
					},
				},
			},
			wantReconcile:   true,
			wantServiceAcct: testServiceAccount,
		},
		{
			name: "multiple listeners - non-matching listener names",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testGatewayName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "http",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": testServiceAccount,
								},
							},
						},
						{
							Name: "grpc",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": testServiceAccount,
								},
							},
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kv-gw-cert-test-gateway-https",
				},
			},
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testServiceAccount,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"azure.workload.identity/client-id": testClientID,
					},
				},
			},
			wantServiceError: true,
		},
		{
			name: "multiple listeners - different service accounts",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testGatewayName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "https-1",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "sa-1",
								},
							},
						},
						{
							Name: "https-2",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": "sa-2",
								},
							},
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kv-gw-cert-test-gateway-https-1",
				},
			},
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sa-1",
					Namespace: testNamespace,
					Annotations: map[string]string{
						"azure.workload.identity/client-id": testClientID,
					},
				},
			},
			wantReconcile:   true,
			wantServiceAcct: "sa-1",
		},
		{
			name: "multiple listeners with mixed TLS configuration",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testGatewayName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "istio",
					Listeners: []gatewayv1.Listener{
						{
							Name: "http",
						},
						{
							Name: "https",
							TLS: &gatewayv1.GatewayTLSConfig{
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									"kubernetes.azure.com/tls-cert-service-account": testServiceAccount,
									"kubernetes.azure.com/tls-cert-keyvault-uri":    "https://kv.vault.azure.net/secrets/cert",
								},
							},
						},
						{
							Name: "mtls",
							TLS:  &gatewayv1.GatewayTLSConfig{},
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kv-gw-cert-test-gateway-https",
				},
			},
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testServiceAccount,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"azure.workload.identity/client-id": testClientID,
					},
				},
			},
			wantReconcile:   false,
			wantServiceAcct: testServiceAccount,
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
			if !tt.wantReconcile {
				return
			}

			sa, err := gatewaySpcOwner.GetServiceAccountName(context.Background(), client, tt.spc, tt.gateway)
			if tt.wantServiceError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantServiceAcct, sa)
			assert.Equal(t, gatewaySpcOwner.kind, "Gateway")
			assert.Equal(t, gatewaySpcOwner.ownerNameAnnotation, "kubernetes.azure.com/gateway-owner")
			assert.Equal(t, gatewaySpcOwner.namespace(tt.gateway), tt.gateway.Namespace)
		})
	}
}

func TestGetIngressSpcOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, netv1.AddToScheme(scheme))

	tests := []struct {
		name          string
		ingress       *netv1.Ingress
		spc           *secv1.SecretProviderClass
		isManaged     bool
		isManagedErr  error
		wantReconcile bool
		wantError     bool
	}{
		{
			name: "managed ingress, no keyvault annotations",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngress,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test": "true",
					},
				},
			},
			isManaged:     true,
			wantReconcile: false,
		},
		{
			name: "managed ingress, keyvault annotations",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngress,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"kubernetes.azure.com/tls-cert-keyvault-uri": "https://kv.vault.azure.net/secrets/cert",
					},
				},
			},
			isManaged:     true,
			wantReconcile: true,
		},
		{
			name: "should not reconcile unmanaged ingress",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngress,
					Namespace: testNamespace,
				},
			},
			wantReconcile: false,
		},
		{
			name: "error when checking if ingress is managed",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testIngress,
					Namespace: testNamespace,
				},
			},
			isManagedErr:  errors.New("test error"),
			wantError:     true,
			wantReconcile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := getIngressSpcOwner(util.NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
				return tt.isManaged, tt.isManagedErr
			}))
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
			assert.Equal(t, owner.kind, "Ingress")
			assert.Equal(t, owner.ownerNameAnnotation, ingressOwnerAnnotation)
			assert.Equal(t, owner.namespace(tt.ingress), tt.ingress.Namespace)
		})
	}
}

func TestSpcOwnerStructGetOwnerAnnotation(t *testing.T) {
	tests := []struct {
		name      string
		ownerType spcOwnerStruct[*testOwner]
		wantAnnot string
	}{
		{
			name: "returns configured annotation",
			ownerType: spcOwnerStruct[*testOwner]{
				kind:                spcTestKind,
				ownerNameAnnotation: "test.company.com/owner-annotation",
			},
			wantAnnot: "test.company.com/owner-annotation",
		},
		{
			name: "returns empty string when not configured",
			ownerType: spcOwnerStruct[*testOwner]{
				kind: spcTestKind,
			},
			wantAnnot: "",
		},
		{
			name: "returns kubernetes prefixed annotation",
			ownerType: spcOwnerStruct[*testOwner]{
				kind:                spcTestKind,
				ownerNameAnnotation: "kubernetes.azure.com/test-owner",
			},
			wantAnnot: "kubernetes.azure.com/test-owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ownerType.GetOwnerAnnotation()
			assert.Equal(t, tt.wantAnnot, got)
		})
	}
}
