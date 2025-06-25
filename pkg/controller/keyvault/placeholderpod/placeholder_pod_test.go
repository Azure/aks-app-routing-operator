// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

type mockSpcOwner struct {
	isOwner             bool
	ownerAnnotation     string
	object              client.Object
	shouldReconcile     bool
	serviceAccountName  string
	getObjectError      error
	serviceAccountError error
}

func (m *mockSpcOwner) IsOwner(_ *secv1.SecretProviderClass) bool {
	return m.isOwner
}

func (m *mockSpcOwner) GetOwnerAnnotation() string {
	return m.ownerAnnotation
}

func (m *mockSpcOwner) GetObject(_ context.Context, _ client.Client, _ *secv1.SecretProviderClass) (client.Object, error) {
	if m.getObjectError != nil {
		return nil, m.getObjectError
	}
	return m.object, nil
}

func (m *mockSpcOwner) ShouldReconcile(_ *secv1.SecretProviderClass, _ client.Object) (bool, error) {
	return m.shouldReconcile, nil
}

func (m *mockSpcOwner) GetServiceAccountName(_ context.Context, _ client.Client, _ *secv1.SecretProviderClass, _ client.Object) (string, error) {
	if m.serviceAccountError != nil {
		return "", m.serviceAccountError
	}
	return m.serviceAccountName, nil
}

func TestPlaceholderPodControllerReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, secv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name           string
		objects        []client.Object
		mockOwner      *mockSpcOwner
		config         *config.Config
		wantResult     ctrl.Result
		wantError      bool
		wantDeployment bool
		verifyFunc     func(t *testing.T, dep *appsv1.Deployment)
	}{
		{
			name: "spc not found returns success",
		},
		{
			name: "no owner found skips reconciliation",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner: false,
			},
		},
		{
			name: "owner found but should not reconcile",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:         true,
				shouldReconcile: false,
				object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
		},
		{
			name: "owner found but get object returns error",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:        true,
				getObjectError: noSpcOwnerErr,
			},
		},
		{
			name: "creates deployment",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:         true,
				shouldReconcile: true,
				ownerAnnotation: "test-annotation",
				object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantDeployment: true,
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				container := dep.Spec.Template.Spec.Containers[0]
				require.Equal(t, "placeholder", container.Name)
			},
		},
		{
			name: "creates deployment with correct security context",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:         true,
				shouldReconcile: true,
				ownerAnnotation: "test-annotation",
				object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantDeployment: true,
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				container := dep.Spec.Template.Spec.Containers[0]
				require.NotNil(t, container.SecurityContext)
				assert.False(t, *container.SecurityContext.Privileged)
				assert.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
				assert.True(t, *container.SecurityContext.RunAsNonRoot)
				assert.Equal(t, int64(65535), *container.SecurityContext.RunAsUser)
				assert.Equal(t, int64(65535), *container.SecurityContext.RunAsGroup)
				assert.True(t, *container.SecurityContext.ReadOnlyRootFilesystem)
				assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, container.SecurityContext.SeccompProfile.Type)
				require.NotNil(t, container.SecurityContext.Capabilities)
				assert.Contains(t, container.SecurityContext.Capabilities.Drop, corev1.Capability("ALL"))
			},
		},
		{
			name: "creates deployment with correct volume configuration",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:         true,
				shouldReconcile: true,
				ownerAnnotation: "test-annotation",
				object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantDeployment: true,
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				// Verify volume mounts
				container := dep.Spec.Template.Spec.Containers[0]
				require.Len(t, container.VolumeMounts, 1)
				volumeMount := container.VolumeMounts[0]
				assert.Equal(t, "secrets", volumeMount.Name)
				assert.Equal(t, "/mnt/secrets", volumeMount.MountPath)
				assert.True(t, volumeMount.ReadOnly)

				// Verify volumes
				require.Len(t, dep.Spec.Template.Spec.Volumes, 1)
				volume := dep.Spec.Template.Spec.Volumes[0]
				assert.Equal(t, "secrets", volume.Name)
				require.NotNil(t, volume.CSI)
				assert.Equal(t, "secrets-store.csi.k8s.io", volume.CSI.Driver)
				assert.True(t, *volume.CSI.ReadOnly)
				assert.Equal(t, "test-spc", volume.CSI.VolumeAttributes["secretProviderClass"])
			},
		},
		{
			name: "preserves existing labels on update",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
						Labels: map[string]string{
							"new-label": "value",
						},
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
						Labels: map[string]string{
							"existing-label": "value",
						},
					},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"existing-selector": "value",
							},
						},
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:         true,
				shouldReconcile: true,
				ownerAnnotation: "test-annotation",
				object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantDeployment: true,
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				// Original selector labels should be preserved
				assert.Equal(t, "value", dep.Spec.Selector.MatchLabels["existing-selector"])
			},
		},
		{
			name: "owner found but service account error",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-spc",
						Namespace: "test-ns",
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:             true,
				shouldReconcile:     true,
				ownerAnnotation:     "test-annotation",
				serviceAccountError: util.NewUserError(errors.New("error"), "service account error"),
				object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantError: true,
		},
		{
			name: "handles generation change",
			objects: []client.Object{
				&secv1.SecretProviderClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-spc",
						Namespace:  "test-ns",
						Generation: 2,
					},
				},
			},
			mockOwner: &mockSpcOwner{
				isOwner:         true,
				shouldReconcile: true,
				ownerAnnotation: "test-annotation",
				object: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-owner",
						Namespace: "test-ns",
					},
				},
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantDeployment: true,
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				assert.Equal(t, "2", dep.Spec.Template.Annotations["kubernetes.azure.com/observed-generation"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objects...).
				Build()

			controller := &PlaceholderPodController{
				client:        client,
				events:        &record.FakeRecorder{},
				config:        tt.config,
				spcOwnerTypes: []spcOwnerType{tt.mockOwner},
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-spc",
					Namespace: "test-ns",
				},
			}

			ctx := logr.NewContext(context.Background(), logr.Discard())
			result, err := controller.Reconcile(ctx, req)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, result)

			if tt.wantDeployment {
				dep := &appsv1.Deployment{}
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      "test-spc",
					Namespace: "test-ns",
				}, dep)
				require.NoError(t, err)

				// Verify deployment spec
				assert.Equal(t, int32(1), *dep.Spec.Replicas)
				assert.Equal(t, int32(2), *dep.Spec.RevisionHistoryLimit)
				assert.NotNil(t, dep.Spec.Selector)
				assert.NotEmpty(t, dep.Spec.Template.Labels)
				assert.NotEmpty(t, dep.Spec.Template.Annotations)
				assert.Equal(t, tt.mockOwner.object.GetName(), dep.Spec.Template.Annotations[tt.mockOwner.ownerAnnotation])

				// Verify container spec
				require.Len(t, dep.Spec.Template.Spec.Containers, 1)
				container := dep.Spec.Template.Spec.Containers[0]
				assert.Equal(t, "placeholder", container.Name)
				assert.Equal(t, tt.config.Registry+"/oss/kubernetes/pause:3.10", container.Image)

				// Run additional verifications if provided
				if tt.verifyFunc != nil {
					tt.verifyFunc(t, dep)
				}
			}
		})
	}
}

func TestGetCurrentDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	tests := []struct {
		name       string
		deployment *appsv1.Deployment
		wantNil    bool
		wantError  bool
	}{
		{
			name: "deployment exists",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
			},
		},
		{
			name:    "deployment does not exist",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{}
			if tt.deployment != nil {
				objects = append(objects, tt.deployment)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			controller := &PlaceholderPodController{
				client: client,
			}

			dep, err := controller.getCurrentDeployment(context.Background(), types.NamespacedName{
				Name:      "test-deployment",
				Namespace: "test-ns",
			})

			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, dep)
			} else {
				assert.NotNil(t, dep)
				assert.Equal(t, tt.deployment.Name, dep.Name)
				assert.Equal(t, tt.deployment.Namespace, dep.Namespace)
			}
		})
	}
}

func TestBuildDeploymentSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, secv1.AddToScheme(scheme))

	tests := []struct {
		name            string
		deployment      *appsv1.Deployment
		spc             *secv1.SecretProviderClass
		existingDep     *appsv1.Deployment
		owner           client.Object
		mockOwner       *mockSpcOwner
		config          *config.Config
		wantError       bool
		wantGeneration  string
		wantServiceAcct string
		verifyFunc      func(t *testing.T, dep *appsv1.Deployment)
	}{
		{
			name: "builds new deployment with minimal config",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-spc",
					Namespace:  "test-ns",
					Generation: 1,
				},
			},
			owner: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-owner",
				},
			},
			mockOwner: &mockSpcOwner{
				ownerAnnotation: "test-annotation",
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantGeneration: "1",
		},
		{
			name: "preserves existing labels and adds new ones",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
			},
			existingDep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"existing": "label",
						},
					},
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-spc",
					Namespace:  "test-ns",
					Generation: 2,
				},
			},
			owner: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-owner",
				},
			},
			mockOwner: &mockSpcOwner{
				ownerAnnotation: "test-annotation",
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantGeneration: "2",
		},
		{
			name: "configures service account correctly",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-spc",
					Namespace: "test-ns",
				},
			},
			owner: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-owner",
				},
			},
			mockOwner: &mockSpcOwner{
				ownerAnnotation:    "test-annotation",
				serviceAccountName: "test-sa",
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantServiceAcct: "test-sa",
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				assert.True(t, *dep.Spec.Template.Spec.AutomountServiceAccountToken)
				assert.Equal(t, "test-sa", dep.Spec.Template.Spec.ServiceAccountName)
			},
		},
		{
			name: "handles service account error",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-spc",
					Namespace: "test-ns",
				},
			},
			owner: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-owner",
				},
			},
			mockOwner: &mockSpcOwner{
				ownerAnnotation:     "test-annotation",
				serviceAccountError: util.NewUserError(nil, "service account error"),
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			wantError: true,
		},
		{
			name: "configures required annotations",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-spc",
					Namespace:  "test-ns",
					Generation: 3,
				},
			},
			owner: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-owner",
				},
			},
			mockOwner: &mockSpcOwner{
				ownerAnnotation: "test-annotation",
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				annotations := dep.Spec.Template.Annotations
				assert.Equal(t, "3", annotations["kubernetes.azure.com/observed-generation"])
				assert.Equal(t, "hold CSI mount to enable keyvault-to-k8s secret mirroring", annotations["kubernetes.azure.com/purpose"])
				assert.Equal(t, "test-owner", annotations["test-annotation"])
				assert.Equal(t, "disabled", annotations["openservicemesh.io/sidecar-injection"])
			},
		},
		{
			name: "sets security context and resource limits correctly",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-ns",
				},
			},
			spc: &secv1.SecretProviderClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-spc",
					Namespace: "test-ns",
				},
			},
			owner: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-owner",
				},
			},
			mockOwner: &mockSpcOwner{
				ownerAnnotation: "test-annotation",
			},
			config: &config.Config{
				Registry: "test.azurecr.io",
			},
			verifyFunc: func(t *testing.T, dep *appsv1.Deployment) {
				container := dep.Spec.Template.Spec.Containers[0]

				// Verify security context
				sc := container.SecurityContext
				require.NotNil(t, sc)
				assert.False(t, *sc.Privileged)
				assert.False(t, *sc.AllowPrivilegeEscalation)
				assert.True(t, *sc.RunAsNonRoot)
				assert.Equal(t, int64(65535), *sc.RunAsUser)
				assert.Equal(t, int64(65535), *sc.RunAsGroup)
				assert.True(t, *sc.ReadOnlyRootFilesystem)
				assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)
				require.NotNil(t, sc.Capabilities)
				assert.Contains(t, sc.Capabilities.Drop, corev1.Capability("ALL"))

				// Verify resource limits
				require.NotNil(t, container.Resources.Limits)
				cpu := container.Resources.Limits[corev1.ResourceCPU]
				memory := container.Resources.Limits[corev1.ResourceMemory]
				assert.Equal(t, "20m", cpu.String())
				assert.Equal(t, "24Mi", memory.String())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{}
			if tt.existingDep != nil {
				objects = append(objects, tt.existingDep)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			controller := &PlaceholderPodController{
				client: client,
				config: tt.config,
			}

			err := controller.buildDeploymentSpec(context.Background(), tt.deployment, tt.spc, tt.owner, tt.mockOwner)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify deployment spec basics
			assert.Equal(t, int32(1), *tt.deployment.Spec.Replicas)
			assert.Equal(t, int32(2), *tt.deployment.Spec.RevisionHistoryLimit)
			assert.NotNil(t, tt.deployment.Spec.Selector)
			assert.NotEmpty(t, tt.deployment.Spec.Template.Labels)

			// Verify generation annotation if specified
			if tt.wantGeneration != "" {
				assert.Equal(t, tt.wantGeneration, tt.deployment.Spec.Template.Annotations["kubernetes.azure.com/observed-generation"])
			}

			// Verify service account if specified
			if tt.wantServiceAcct != "" {
				assert.True(t, *tt.deployment.Spec.Template.Spec.AutomountServiceAccountToken)
				assert.Equal(t, tt.wantServiceAcct, tt.deployment.Spec.Template.Spec.ServiceAccountName)
			}

			// Run additional verifications if provided
			if tt.verifyFunc != nil {
				tt.verifyFunc(t, tt.deployment)
			}
		})
	}
}
