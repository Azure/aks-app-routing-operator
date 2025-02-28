// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	placeholderPodControllerNameElements = []string{"keyvault", "placeholder", "pod"}
	placeholderTestIngClassName          = "webapprouting.kubernetes.azure.com"
	placeholderTestIng                   = &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ing",
			Namespace: "default",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "v1",
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &placeholderTestIngClassName,
		},
	}
	placeholderTestUri          = "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34"
	placeholderTestNginxIngress = &v1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-nic",
		},
		TypeMeta: metav1.TypeMeta{Kind: "NginxIngressController"},
		Spec: v1alpha1.NginxIngressControllerSpec{
			IngressClassName: spcTestNginxIngressClassName,
			DefaultSSLCertificate: &v1alpha1.DefaultSSLCertificate{
				KeyVaultURI: &placeholderTestUri,
			},
		},
	}
	placeholderSpc = &secv1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-spc",
			Namespace:  placeholderTestIng.Namespace,
			Generation: 123,
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Ingress",
				Name: placeholderTestIng.Name,
			}},
		},
	}
	placeholderDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
	}
)

func ReconcileAndTestSuccessMetrics(t *testing.T, ctx context.Context, reconciler reconcile.Reconciler, req ctrl.Request, controllerNameElements []string) {
	controllerName := controllername.New(controllerNameElements[0], controllerNameElements[1:]...)
	beforeErrCount := testutils.GetErrMetricCount(t, controllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, controllerName, metrics.LabelSuccess)
	_, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, controllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, controllerName, metrics.LabelSuccess), beforeReconcileCount)
}

func TestPlaceholderPodControllerIntegrationWithIng(t *testing.T) {
	ing := placeholderTestIng.DeepCopy()
	spc := placeholderSpc.DeepCopy()
	spc.Labels = manifests.GetTopLevelLabels()

	c := fake.NewClientBuilder().WithObjects(spc, ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	p := &PlaceholderPodController{
		client: c,
		config: &config.Config{Registry: "test-registry"},
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if ing == nil {
				return false, nil
			}

			if ing.Spec.IngressClassName == nil {
				return false, nil
			}

			if *ing.Spec.IngressClassName == placeholderTestIngClassName {
				return true, nil
			}

			return false, nil
		}),
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create placeholder pod deployment
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: spc.Namespace, Name: spc.Name}}
	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spc.Name,
			Namespace: spc.Namespace,
		},
	}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))

	replicas := int32(1)
	historyLimit := int32(2)

	expectedLabels := map[string]string{"app": spc.Name}
	expected := appsv1.DeploymentSpec{
		Replicas:             &replicas,
		RevisionHistoryLimit: &historyLimit,
		Selector:             &metav1.LabelSelector{MatchLabels: expectedLabels},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: expectedLabels,
				Annotations: map[string]string{
					"kubernetes.azure.com/observed-generation": "123",
					"kubernetes.azure.com/purpose":             "hold CSI mount to enable keyvault-to-k8s secret mirroring",
					"kubernetes.azure.com/ingress-owner":       ing.Name,
					"openservicemesh.io/sidecar-injection":     "disabled",
				},
			},
			Spec: *manifests.WithPreferSystemNodes(&corev1.PodSpec{
				AutomountServiceAccountToken: util.ToPtr(false),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: "test-registry/oss/kubernetes/pause:3.10",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "secrets",
						MountPath: "/mnt/secrets",
						ReadOnly:  true,
					}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("20m"),
							corev1.ResourceMemory: resource.MustParse("24Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged:               util.ToPtr(false),
						AllowPrivilegeEscalation: util.ToPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsNonRoot:           util.ToPtr(true),
						RunAsUser:              util.Int64Ptr(65535),
						RunAsGroup:             util.Int64Ptr(65535),
						ReadOnlyRootFilesystem: util.ToPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				}},
				Volumes: []corev1.Volume{{
					Name: "secrets",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver:           "secrets-store.csi.k8s.io",
							ReadOnly:         util.ToPtr(true),
							VolumeAttributes: map[string]string{"secretProviderClass": spc.Name},
						},
					},
				}},
			}),
		},
	}
	assert.Equal(t, expected, dep.Spec)

	// Prove idempotence
	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	// Update the secret class generation
	spc.Generation = 234
	expected.Template.Annotations["kubernetes.azure.com/observed-generation"] = "234"
	require.NoError(t, c.Update(ctx, spc))

	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	// Prove the generation annotation was updated
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))
	assert.Equal(t, expected, dep.Spec)

	// Change the ingress resource's class
	ing.Spec.IngressClassName = nil
	require.NoError(t, c.Update(ctx, ing))

	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	// Prove the deployment was deleted
	require.True(t, k8serrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))

	// Prove idempotence
	require.True(t, k8serrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))

	// Prove that placeholder deployment retains immutable fields during updates
	oldPlaceholder := &appsv1.Deployment{}
	labels := map[string]string{"foo": "bar", "fizz": "buzz"}
	oldPlaceholder.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	oldPlaceholder.Name = "immutable-test"
	require.NoError(t, c.Create(ctx, oldPlaceholder), "failed to create old placeholder deployment")

	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	updatedPlaceholder := &appsv1.Deployment{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(oldPlaceholder), updatedPlaceholder), "failed to get updated placeholder deployment")
	assert.Equal(t, labels, updatedPlaceholder.Spec.Selector.MatchLabels, "selector labels should have been retained")
}

func TestPlaceholderPodControllerIntegrationWithNic(t *testing.T) {
	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	nic := placeholderTestNginxIngress.DeepCopy()
	spc := getDefaultNginxSpc(nic)
	spc.SetOwnerReferences(manifests.GetOwnerRefs(nic, true))

	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	require.NoError(t, c.Create(ctx, nic))
	require.NoError(t, c.Create(ctx, spc))

	p := &PlaceholderPodController{
		client:         c,
		config:         &config.Config{Registry: "test-registry"},
		ingressManager: nil,
		events:         recorder,
	}

	// Create placeholder pod deployment
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: spc.Namespace, Name: spc.Name}}
	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spc.Name,
			Namespace: spc.Namespace,
		},
	}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))

	replicas := int32(1)
	historyLimit := int32(2)

	expectedLabels := map[string]string{"app": spc.Name}
	expected := appsv1.DeploymentSpec{
		Replicas:             &replicas,
		RevisionHistoryLimit: &historyLimit,
		Selector:             &metav1.LabelSelector{MatchLabels: expectedLabels},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: expectedLabels,
				Annotations: map[string]string{
					"kubernetes.azure.com/observed-generation":            "123",
					"kubernetes.azure.com/purpose":                        "hold CSI mount to enable keyvault-to-k8s secret mirroring",
					"kubernetes.azure.com/nginx-ingress-controller-owner": nic.Name,
					"openservicemesh.io/sidecar-injection":                "disabled",
				},
			},
			Spec: *manifests.WithPreferSystemNodes(&corev1.PodSpec{
				AutomountServiceAccountToken: util.ToPtr(false),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: "test-registry/oss/kubernetes/pause:3.10",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "secrets",
						MountPath: "/mnt/secrets",
						ReadOnly:  true,
					}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("20m"),
							corev1.ResourceMemory: resource.MustParse("24Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged:               util.ToPtr(false),
						AllowPrivilegeEscalation: util.ToPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsNonRoot:           util.ToPtr(true),
						RunAsUser:              util.Int64Ptr(65535),
						RunAsGroup:             util.Int64Ptr(65535),
						ReadOnlyRootFilesystem: util.ToPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				}},
				Volumes: []corev1.Volume{{
					Name: "secrets",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver:           "secrets-store.csi.k8s.io",
							ReadOnly:         util.ToPtr(true),
							VolumeAttributes: map[string]string{"secretProviderClass": spc.Name},
						},
					},
				}},
			}),
		},
	}
	assert.Equal(t, expected, dep.Spec)

	// Prove idempotence
	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	// Update the secret class generation
	spc.Generation = 234
	expected.Template.Annotations["kubernetes.azure.com/observed-generation"] = "234"
	require.NoError(t, c.Update(ctx, spc))

	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	// Prove the generation annotation was updated
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))
	assert.Equal(t, expected, dep.Spec)

	// Remove Key Vault URI
	nic.Spec.DefaultSSLCertificate.KeyVaultURI = nil
	require.NoError(t, c.Update(ctx, nic))

	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	// Prove the deployment was deleted
	require.True(t, k8serrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))

	// Prove idempotence
	require.True(t, k8serrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))

	// Prove that placeholder deployment retains immutable fields during updates
	oldPlaceholder := &appsv1.Deployment{}
	labels := map[string]string{"foo": "bar", "fizz": "buzz"}
	oldPlaceholder.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	oldPlaceholder.Name = "immutable-test"
	require.NoError(t, c.Create(ctx, oldPlaceholder), "failed to create old placeholder deployment")

	ReconcileAndTestSuccessMetrics(t, ctx, p, req, placeholderPodControllerNameElements)

	updatedPlaceholder := &appsv1.Deployment{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(oldPlaceholder), updatedPlaceholder), "failed to get updated placeholder deployment")
	assert.Equal(t, labels, updatedPlaceholder.Spec.Selector.MatchLabels, "selector labels should have been retained")
}

func TestPlaceholderPodControllerIntegrationWithGw(t *testing.T) {
	recorder := record.NewFakeRecorder(1)
	gw := gatewayWithTwoServiceAccounts.DeepCopy()

	saspc := serviceAccountTwoSpc.DeepCopy()
	saspc.Generation = 124

	c := testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, corev1.AddToScheme, appsv1.AddToScheme).WithObjects(saspc, gw, annotatedServiceAccount, annotatedServiceAccountTwo).Build()
	p := &PlaceholderPodController{
		client: c,
		config: &config.Config{Registry: "test-registry"},
		events: recorder,
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create placeholder pod deployment for serviceaccount listener
	saReq := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: saspc.Namespace, Name: saspc.Name}}
	ReconcileAndTestSuccessMetrics(t, ctx, p, saReq, placeholderPodControllerNameElements)
	require.Equal(t, 0, len(recorder.Events))

	replicas := int32(1)
	historyLimit := int32(2)

	saDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saspc.Name,
			Namespace: saspc.Namespace,
		},
	}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(saDep), saDep))

	expectedSaLabels := map[string]string{"app": saspc.Name}
	expectedSaDep := appsv1.DeploymentSpec{
		Replicas:             &replicas,
		RevisionHistoryLimit: &historyLimit,
		Selector:             &metav1.LabelSelector{MatchLabels: expectedSaLabels},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: expectedSaLabels,
				Annotations: map[string]string{
					"kubernetes.azure.com/observed-generation": "124",
					"kubernetes.azure.com/purpose":             "hold CSI mount to enable keyvault-to-k8s secret mirroring",
					"kubernetes.azure.com/gateway-owner":       gw.Name,
					"openservicemesh.io/sidecar-injection":     "disabled",
				},
			},
			Spec: *manifests.WithPreferSystemNodes(&corev1.PodSpec{
				ServiceAccountName:           "test-sa-2",
				AutomountServiceAccountToken: util.ToPtr(true),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: "test-registry/oss/kubernetes/pause:3.10",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "secrets",
						MountPath: "/mnt/secrets",
						ReadOnly:  true,
					}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("20m"),
							corev1.ResourceMemory: resource.MustParse("24Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged:               util.ToPtr(false),
						AllowPrivilegeEscalation: util.ToPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsNonRoot:           util.ToPtr(true),
						RunAsUser:              util.Int64Ptr(65535),
						RunAsGroup:             util.Int64Ptr(65535),
						ReadOnlyRootFilesystem: util.ToPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				}},
				Volumes: []corev1.Volume{{
					Name: "secrets",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver:           "secrets-store.csi.k8s.io",
							ReadOnly:         util.ToPtr(true),
							VolumeAttributes: map[string]string{"secretProviderClass": saspc.Name},
						},
					},
				}},
			}),
		},
	}
	assert.Equal(t, expectedSaDep, saDep.Spec)

	// Prove idempotence for the SA deployment
	ReconcileAndTestSuccessMetrics(t, ctx, p, saReq, placeholderPodControllerNameElements)

	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(saDep), saDep))
	require.Equal(t, 0, len(recorder.Events))
	require.Equal(t, expectedSaDep, saDep.Spec)

	// Update the secret class generation
	saspc.Generation = 234
	expectedSaDep.Template.Annotations["kubernetes.azure.com/observed-generation"] = "234"
	require.NoError(t, c.Update(ctx, saspc))

	ReconcileAndTestSuccessMetrics(t, ctx, p, saReq, placeholderPodControllerNameElements)
	require.Equal(t, 0, len(recorder.Events))

	// Prove the generation annotation was updated
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(saDep), saDep))
	require.Equal(t, expectedSaDep, saDep.Spec)

	// Change the gw resource's GatewayClass
	gw.Spec.GatewayClassName = "notistio"
	require.NoError(t, c.Update(ctx, gw))

	ReconcileAndTestSuccessMetrics(t, ctx, p, saReq, placeholderPodControllerNameElements)
	// Prove the sa deployment was deleted
	require.True(t, k8serrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(saDep), saDep)))
}

func TestVerifyServiceAccount(t *testing.T) {
	tcs := []struct {
		name                   string
		spc                    *secv1.SecretProviderClass
		obj                    client.Object
		existingObjects        []client.Object
		expectedServiceAccount string
		expectedError          error
	}{
		{
			name:                   "happy path with input serviceaccount",
			spc:                    serviceAccountTwoSpc,
			obj:                    gatewayWithTwoServiceAccounts,
			existingObjects:        []client.Object{annotatedServiceAccount, annotatedServiceAccountTwo, serviceAccountTwoSpc},
			expectedServiceAccount: "test-sa-2",
		},
		{
			name:            "no matching listeners",
			spc:             serviceAccountTwoSpc,
			obj:             modifyGateway(gatewayWithTwoServiceAccounts, func(gw *gatewayv1.Gateway) { gw.Spec.Listeners[1].Name = "test-listener-3" }),
			existingObjects: []client.Object{gatewayWithTwoServiceAccounts, annotatedServiceAccount},
			expectedError:   newUserError(errors.New("failed to locate listener for SPC kv-gw-cert-test-gw-test-listener-2 on user's gateway resource"), "gateway listener for spc %s doesn't exist or doesn't contain required TLS options"),
		},
		{
			name: "listener matches but doesn't contain service account option",
			spc:  serviceAccountTwoSpc,
			obj: modifyGateway(gatewayWithTwoServiceAccounts, func(gw *gatewayv1.Gateway) {
				gw.Spec.Listeners[1].TLS.Options = map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{"not-service-account": "test-value"}
			}),
			existingObjects: []client.Object{gatewayWithTwoServiceAccounts, serviceAccountTwoSpc, annotatedServiceAccount},
			expectedError:   newUserError(errors.New("failed to locate listener for SPC kv-gw-cert-test-gw-test-listener-2 on user's gateway resource"), "gateway listener for spc %s doesn't exist or doesn't contain required TLS options"),
		},
		{
			name: "nonexistent service account referenced",
			spc:  serviceAccountTwoSpc,
			obj: modifyGateway(gatewayWithTwoServiceAccounts, func(gw *gatewayv1.Gateway) {
				gw.Spec.Listeners[1].TLS.Options = map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{"kubernetes.azure.com/tls-cert-service-account": "fake-sa"}
			}),
			existingObjects: []client.Object{serviceAccountTwoSpc, gatewayWithTwoServiceAccounts, annotatedServiceAccount},
			expectedError:   newUserError(errors.New("serviceaccounts \"fake-sa\" not found"), "gateway listener for spc %s doesn't exist or doesn't contain required TLS options"),
		},
		{
			name: "service account without required annotation referenced",
			spc:  serviceAccountTwoSpc,
			obj:  gatewayWithTwoServiceAccounts,
			existingObjects: []client.Object{
				serviceAccountTwoSpc,
				gatewayWithTwoServiceAccounts,
				&corev1.ServiceAccount{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ServiceAccount",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   gatewayWithTwoServiceAccounts.Namespace,
						Name:        "test-sa-2",
						Annotations: map[string]string{"foo": "bar"},
					},
				},
			},
			expectedError: newUserError(errors.New("user-specified service account does not contain WI annotation"), "serviceAccount test-sa was specified in Gateway but does not include necessary annotation for workload identity"),
		},
		{
			name: "incorrect object type",
			spc:  serviceAccountTwoSpc,
			obj:  &netv1.Ingress{},
		},
	}

	for _, tc := range tcs {
		t.Logf("starting case %s", tc.name)
		c := testutils.RegisterSchemes(t, fake.NewClientBuilder(), secv1.AddToScheme, gatewayv1.Install, corev1.AddToScheme).WithObjects(tc.existingObjects...).Build()
		p := PlaceholderPodController{
			client: c,
		}

		serviceAccount, err := p.verifyServiceAccount(context.Background(), tc.spc, tc.obj, logr.Discard())

		if tc.expectedError != nil {
			require.NotNil(t, err)
			require.Equal(t, tc.expectedError.Error(), err.Error())
		} else {
			require.Nil(t, err)
		}
		require.Equal(t, tc.expectedServiceAccount, serviceAccount)
	}
}

func TestPlaceholderPodControllerNoManagedByLabels(t *testing.T) {
	ing := placeholderTestIng.DeepCopy()
	spc := placeholderSpc.DeepCopy()
	spc.Labels = map[string]string{}

	c := fake.NewClientBuilder().WithObjects(spc, ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	p := &PlaceholderPodController{
		client: c,
		config: &config.Config{Registry: "test-registry"},
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if ing == nil {
				return false, nil
			}

			if ing.Spec.IngressClassName == nil {
				return false, nil
			}

			if *ing.Spec.IngressClassName == placeholderTestIngClassName {
				return true, nil
			}

			return false, nil
		}),
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create placeholder pod deployment
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: spc.Namespace, Name: spc.Name}}
	beforeErrCount := testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err := p.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spc.Name,
			Namespace: spc.Namespace,
		},
	}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))

	replicas := int32(1)
	historyLimit := int32(2)

	expectedLabels := map[string]string{"app": spc.Name}
	expected := appsv1.DeploymentSpec{
		Replicas:             &replicas,
		RevisionHistoryLimit: &historyLimit,
		Selector:             &metav1.LabelSelector{MatchLabels: expectedLabels},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: expectedLabels,
				Annotations: map[string]string{
					"kubernetes.azure.com/observed-generation": "123",
					"kubernetes.azure.com/purpose":             "hold CSI mount to enable keyvault-to-k8s secret mirroring",
					"kubernetes.azure.com/ingress-owner":       ing.Name,
					"openservicemesh.io/sidecar-injection":     "disabled",
				},
			},
			Spec: *manifests.WithPreferSystemNodes(&corev1.PodSpec{
				AutomountServiceAccountToken: util.ToPtr(false),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: "test-registry/oss/kubernetes/pause:3.10",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "secrets",
						MountPath: "/mnt/secrets",
						ReadOnly:  true,
					}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("20m"),
							corev1.ResourceMemory: resource.MustParse("24Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged:               util.ToPtr(false),
						AllowPrivilegeEscalation: util.ToPtr(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsNonRoot:           util.ToPtr(true),
						RunAsUser:              util.Int64Ptr(65535),
						RunAsGroup:             util.Int64Ptr(65535),
						ReadOnlyRootFilesystem: util.ToPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				}},
				Volumes: []corev1.Volume{{
					Name: "secrets",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver:           "secrets-store.csi.k8s.io",
							ReadOnly:         util.ToPtr(true),
							VolumeAttributes: map[string]string{"secretProviderClass": spc.Name},
						},
					},
				}},
			}),
		},
	}
	assert.Equal(t, expected, dep.Spec)

	// Prove idempotence
	beforeErrCount = testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Change the ingress resource's class
	ing.Spec.IngressClassName = nil
	require.NoError(t, c.Update(ctx, ing))

	beforeErrCount = testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Prove the deployment was not deleted
	require.False(t, k8serrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))
}

func TestPlaceholderPodControllerUnmanagedDeploymentUnmanagedSPC(t *testing.T) {
	commonName := "name"
	ing := placeholderTestIng.DeepCopy()
	spc := &secv1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      commonName,
			Namespace: ing.Namespace,
		},
	}
	spc.SetOwnerReferences(manifests.GetOwnerRefs(ing, true))
	cl := fake.NewClientBuilder().WithObjects(ing, spc).Build()
	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	unmanagedDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      commonName,
			Namespace: spc.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(3),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": commonName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"label": "label"},
					Annotations: map[string]string{"annotation": "annotation"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  commonName,
							Image: "test/image:latest",
						},
					},
				},
			},
		},
	}
	require.NoError(t, cl.Create(ctx, unmanagedDeployment), "creating unmanaged deployment")

	p := &PlaceholderPodController{
		client:         cl,
		config:         &config.Config{Registry: "test-registry"},
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) { return false, nil }),
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: spc.Namespace, Name: spc.Name}}
	beforeErrCount := testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err := p.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

	afterDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      unmanagedDeployment.Name,
			Namespace: unmanagedDeployment.Namespace,
		},
	}
	require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(afterDeployment), afterDeployment))
	assert.Equal(t, unmanagedDeployment.Spec, afterDeployment.Spec)
}

func TestNewPlaceholderPodController(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)

	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}

	ingressManager := NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
		if ing == nil {
			return false, nil
		}

		if ing.Spec.IngressClassName == nil {
			return false, nil
		}

		if *ing.Spec.IngressClassName == "webapprouting.kubernetes.azure" {
			return true, nil
		}

		return false, nil
	})

	err = NewPlaceholderPodController(m, conf, ingressManager)
	require.NoError(t, err)

	// test nil ingress manager for nginx ingress controller
	nilIm, err := manager.New(restConfig, manager.Options{Controller: ctrlconfig.Controller{SkipNameValidation: to.Ptr(true)}, Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)
	err = NewPlaceholderPodController(nilIm, conf, nil)
	require.NoError(t, err)
}

func TestGetCurrentDeploymentWithIng(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
	}
	c := fake.NewFakeClient(dep)
	p := &PlaceholderPodController{client: c}

	// can find existing deployment
	dep, err := p.getCurrentDeployment(context.Background(), client.ObjectKeyFromObject(dep))
	require.NoError(t, err)
	require.NotNil(t, dep)

	// returns nil if deployment does not exist
	dep, err = p.getCurrentDeployment(context.Background(), client.ObjectKey{Name: "does-not-exist", Namespace: "test-namespace"})
	require.NoError(t, err)
	require.Nil(t, dep)
}

func TestPlaceholderPodCleanCheck(t *testing.T) {
	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))

	nic := placeholderTestNginxIngress.DeepCopy()
	ing := placeholderTestIng.DeepCopy()
	ing.TypeMeta.Kind = "Ingress"
	unmanagedIngClassName := "unmanagedClassName"
	errorIngClassName := "errorClassName"

	gw := gatewayWithTwoServiceAccounts.DeepCopy()
	// not sure why this happens but otherwise resourceversion is 999 after deepcopy
	gw.ObjectMeta.ResourceVersion = ""

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	require.NoError(t, c.Create(ctx, nic))
	require.NoError(t, c.Create(ctx, ing))
	require.NoError(t, c.Create(ctx, gw))
	p := &PlaceholderPodController{
		client: c,
		config: &config.Config{Registry: "test-registry"},
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			if ing.Spec.IngressClassName != nil {
				if *ing.Spec.IngressClassName == unmanagedIngClassName {
					return false, nil
				}

				if *ing.Spec.IngressClassName == errorIngClassName {
					return false, fmt.Errorf("an error has occured checking if ingress is managed")
				}
			}
			return true, nil
		}),
	}

	// Default scenarios for ingress and nginxingresscontroller. Does not clean when spc required fields are there
	cleanPod, err := p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, nic)
	require.NoError(t, err)
	require.Equal(t, false, cleanPod)

	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, ing)
	require.NoError(t, err)
	require.Equal(t, false, cleanPod)

	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "kv-gw-cert-test-gw-test-listener"}}, gw)
	require.NoError(t, err)
	require.Equal(t, false, cleanPod)

	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "kv-gw-cert-test-gw-test-listener-2"}}, gw)
	require.NoError(t, err)
	require.Equal(t, false, cleanPod)

	// Clean Placeholder Pod scenarios

	// nic without key vault uri
	nic.Spec.DefaultSSLCertificate.KeyVaultURI = nil
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, nic)
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	// nic without DefaultSSLCertificate
	nic.Spec.DefaultSSLCertificate = nil
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, nic)
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	// ingress without IngressClassName
	ing.Spec.IngressClassName = nil
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, ing)
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	ing.Spec.IngressClassName = placeholderTestIng.Spec.IngressClassName // returning value to IngressClassName to test individual fields triggering clean

	// ingress with empty Name field
	ing.Name = ""
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, ing)
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	// unmanaged ingress
	ing.Spec.IngressClassName = &unmanagedIngClassName
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, ing)
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	// ingress that hits an error while checking if it's managed
	ing.Spec.IngressClassName = &errorIngClassName
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{}, ing)
	require.Error(t, err, "determining if ingress is managed: an error has occured checking if ingress is managed")
	require.Equal(t, false, cleanPod)

	// gw with no matching listeners
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "kv-gw-cert-test-gw-test-listener-3"}}, gw)
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	// gw with listener that doesn't have a cert uri
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "kv-gw-cert-test-gw-test-listener"}},
		modifyGateway(gw, func(gw *gatewayv1.Gateway) {
			gw.Spec.Listeners[0].TLS.Options["kubernetes.azure.com/tls-cert-keyvault-uri"] = ""
		}))
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	// gw with listener that doesn't have TLS
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "kv-gw-cert-test-gw-test-listener"}},
		modifyGateway(gw, func(gw *gatewayv1.Gateway) {
			gw.Spec.Listeners[0].TLS = nil
		}))
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)

	// gw with non-istio gatewayclass
	cleanPod, err = p.placeholderPodCleanCheck(&secv1.SecretProviderClass{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "kv-gw-cert-test-gw-test-listener"}},
		modifyGateway(gw, func(gw *gatewayv1.Gateway) {
			gw.Spec.GatewayClassName = "not-istio"
		}))
	require.NoError(t, err)
	require.Equal(t, true, cleanPod)
}

func TestBuildDeployment(t *testing.T) {
	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, gatewayv1.Install(scheme))

	spc := placeholderSpc.DeepCopy()

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := &PlaceholderPodController{
		client: c,
		config: &config.Config{Registry: "test-registry"},
		ingressManager: NewIngressManagerFromFn(func(ing *netv1.Ingress) (bool, error) {
			return true, nil
		}),
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spc.Name,
			Namespace: spc.Namespace,
		},
	}

	var emptyObj client.Object

	err := p.buildDeployment(ctx, dep, spc, emptyObj)
	require.EqualError(t, err, "failed to build deployment: object type not ingress, nginxingresscontroller, or gateway")

	// test gateway owner annotation
	gwSpc := &secv1.SecretProviderClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-spc",
			Namespace: "test-ns",
		},
	}

	gwObj := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gw",
			Namespace: "test-ns",
		},
	}

	err = p.buildDeployment(ctx, dep, gwSpc, gwObj)
	require.Equal(t, nil, err)
	require.Equal(t, dep.Spec.Template.Annotations["kubernetes.azure.com/gateway-owner"], "test-gw")
}

func getDefaultNginxSpc(nic *v1alpha1.NginxIngressController) *secv1.SecretProviderClass {
	spc := &secv1.SecretProviderClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "secrets-store.csi.x-k8s.io/v1",
			Kind:       "SecretProviderClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       DefaultNginxCertName(nic),
			Namespace:  config.DefaultNs,
			Labels:     manifests.GetTopLevelLabels(),
			Generation: 123,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: nic.APIVersion,
				Controller: util.ToPtr(true),
				Kind:       "NginxIngressController",
				Name:       nic.Name,
				UID:        nic.UID,
			}},
		},
	}

	return spc
}
