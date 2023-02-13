// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func TestPlaceholderPodControllerIntegration(t *testing.T) {
	ing := &netv1.Ingress{}
	ing.Name = "test-ing"
	ing.Namespace = "default"
	ingressClass := "webapprouting.kubernetes.azure.com"
	ing.Spec.IngressClassName = &ingressClass

	spc := &secv1.SecretProviderClass{}
	spc.Name = "test-spc"
	spc.Namespace = ing.Namespace
	spc.Generation = 123
	spc.OwnerReferences = []metav1.OwnerReference{{
		Kind: "Ingress",
		Name: ing.Name,
	}}

	c := fake.NewClientBuilder().WithObjects(spc, ing).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))
	p := &PlaceholderPodController{
		client:     c,
		config:     &config.Config{Registry: "test-registry"},
		ingConfigs: []*manifests.NginxIngressConfig{{IcName: ingressClass}},
	}

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())

	// Create placeholder pod deployment
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: spc.Namespace, Name: spc.Name}}
	_, err := p.Reconcile(ctx, req)
	require.NoError(t, err)

	dep := &appsv1.Deployment{}
	dep.Name = spc.Name
	dep.Namespace = spc.Namespace
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))

	replicas := int32(1)
	historyLimit := int32(2)
	expected := appsv1.DeploymentSpec{
		Replicas:             &replicas,
		RevisionHistoryLimit: &historyLimit,
		Selector:             &metav1.LabelSelector{MatchLabels: map[string]string{"app": spc.Name}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": spc.Name},
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
	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)

	// Update the secret class generation
	spc.Generation = 234
	expected.Template.Annotations["kubernetes.azure.com/observed-generation"] = "234"
	require.NoError(t, c.Update(ctx, spc))

	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove the generation annotation was updated
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dep), dep))
	assert.Equal(t, expected, dep.Spec)

	// Change the ingress resource's class
	ing.Spec.IngressClassName = nil
	require.NoError(t, c.Update(ctx, ing))

	_, err = p.Reconcile(ctx, req)
	require.NoError(t, err)

	// Prove the deployment was deleted
	require.True(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))

	// Prove idempotence
	require.True(t, errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(dep), dep)))
}
