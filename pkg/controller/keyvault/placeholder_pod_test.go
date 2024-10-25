// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

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
	placeholderTestIngClassName = "webapprouting.kubernetes.azure.com"
	placeholderTestIng          = &netv1.Ingress{
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
					Image: "test-registry/oss/kubernetes/pause:3.9-hotfix-20230808",
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

	// Prove that placeholder deployment retains immutable fields during updates
	oldPlaceholder := &appsv1.Deployment{}
	labels := map[string]string{"foo": "bar", "fizz": "buzz"}
	oldPlaceholder.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	oldPlaceholder.Name = "immutable-test"
	require.NoError(t, c.Create(ctx, oldPlaceholder), "failed to create old placeholder deployment")
	beforeErrCount = testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err = p.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(oldPlaceholder)})
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

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
					Image: "test-registry/oss/kubernetes/pause:3.9-hotfix-20230808",
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

	// Remove Key Vault URI
	nic.Spec.DefaultSSLCertificate.KeyVaultURI = nil
	require.NoError(t, c.Update(ctx, nic))

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

	// Prove that placeholder deployment retains immutable fields during updates
	oldPlaceholder := &appsv1.Deployment{}
	labels := map[string]string{"foo": "bar", "fizz": "buzz"}
	oldPlaceholder.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	oldPlaceholder.Name = "immutable-test"
	require.NoError(t, c.Create(ctx, oldPlaceholder), "failed to create old placeholder deployment")
	beforeErrCount = testutils.GetErrMetricCount(t, placeholderPodControllerName)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess)
	_, err = p.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(oldPlaceholder)})
	require.NoError(t, err)
	require.Equal(t, testutils.GetErrMetricCount(t, placeholderPodControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, placeholderPodControllerName, metrics.LabelSuccess), beforeReconcileCount)

	updatedPlaceholder := &appsv1.Deployment{}
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(oldPlaceholder), updatedPlaceholder), "failed to get updated placeholder deployment")
	assert.Equal(t, labels, updatedPlaceholder.Spec.Selector.MatchLabels, "selector labels should have been retained")
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
					Image: "test-registry/oss/kubernetes/pause:3.9-hotfix-20230808",
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
	require.False(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))
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

	nic := placeholderTestNginxIngress.DeepCopy()
	ing := placeholderTestIng.DeepCopy()
	ing.TypeMeta.Kind = "Ingress"
	unmanagedIngClassName := "unmanagedClassName"
	errorIngClassName := "errorClassName"

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	require.NoError(t, c.Create(ctx, nic))
	require.NoError(t, c.Create(ctx, ing))
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
}

func TestBuildDeployment(t *testing.T) {
	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, netv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

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
