// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"

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
	placeholderTestIngClassName = "webapprouting.kubernetes.azure.com"
	placeholderTestIng          = &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ing",
			Namespace: "default",
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &placeholderTestIngClassName,
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
)

func TestPlaceholderPodControllerIntegration(t *testing.T) {
	ing := placeholderTestIng.DeepCopy()
	spc := placeholderSpc.DeepCopy()
	spc.Labels = manifests.GetTopLevelLabels()

	c := fake.NewClientBuilder().WithObjects(spc, ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	p := &PlaceholderPodController{
		client:         c,
		config:         &config.Config{Registry: "test-registry"},
		ingressManager: NewIngressManager(map[string]struct{}{placeholderTestIngClassName: {}}),
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
				AutomountServiceAccountToken: util.BoolPtr(false),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: "test-registry/oss/kubernetes/pause:3.6-hotfix.20220114",
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
				}},
				Volumes: []corev1.Volume{{
					Name: "secrets",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver:           "secrets-store.csi.k8s.io",
							ReadOnly:         util.BoolPtr(true),
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

	// Update the secret class generation
	spc.Generation = 234
	expected.Template.Annotations["kubernetes.azure.com/observed-generation"] = "234"
	require.NoError(t, c.Update(ctx, spc))

	beforeErrCount = testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Prove the generation annotation was updated
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))
	assert.Equal(t, expected, dep.Spec)

	// Change the ingress resource's class
	ing.Spec.IngressClassName = nil
	require.NoError(t, c.Update(ctx, ing))

	beforeErrCount = testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

	// Prove the deployment was deleted
	require.True(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))

	// Prove idempotence
	require.True(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))
}

func TestPlaceholderPodControllerNoManagedByLabels(t *testing.T) {
	ing := placeholderTestIng.DeepCopy()
	spc := placeholderSpc.DeepCopy()
	spc.Labels = map[string]string{}

	c := fake.NewClientBuilder().WithObjects(spc, ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	p := &PlaceholderPodController{
		client:         c,
		config:         &config.Config{Registry: "test-registry"},
		ingressManager: NewIngressManager(map[string]struct{}{placeholderTestIngClassName: {}}),
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
				AutomountServiceAccountToken: util.BoolPtr(false),
				Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: "test-registry/oss/kubernetes/pause:3.6-hotfix.20220114",
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
				}},
				Volumes: []corev1.Volume{{
					Name: "secrets",
					VolumeSource: corev1.VolumeSource{
						CSI: &corev1.CSIVolumeSource{
							Driver:           "secrets-store.csi.k8s.io",
							ReadOnly:         util.BoolPtr(true),
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
	require.False(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))
}

func TestNewPlaceholderPodController(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)

	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}
	ingressManager := NewIngressManager(map[string]struct{}{"webapprouting.kubernetes.azure.com": {}})
	err = NewPlaceholderPodController(m, conf, ingressManager)
	require.NoError(t, err)
}
