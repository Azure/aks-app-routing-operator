package nginxingress

import (
	"errors"
	"fmt"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

func TestUpdateStatusManagedResourceRefs(t *testing.T) {
	t.Run("nil managed resource refs", func(t *testing.T) {
		nic := &approutingv1alpha1.NginxIngressController{}
		n := &nginxIngressControllerReconciler{}
		n.updateStatusManagedResourceRefs(nic, nil)

		require.Nil(t, nic.Status.ManagedResourceRefs)
	})

	t.Run("managed resource refs", func(t *testing.T) {
		nic := &approutingv1alpha1.NginxIngressController{}
		n := &nginxIngressControllerReconciler{}
		refs := []approutingv1alpha1.ManagedObjectReference{
			{
				Name:      "name",
				Namespace: "namespace",
				Kind:      "kind",
				APIGroup:  "group",
			},
		}
		n.updateStatusManagedResourceRefs(nic, refs)
		require.Equal(t, refs, nic.Status.ManagedResourceRefs)

	})
}

func TestUpdateStatusIngressClass(t *testing.T) {
	t.Run("nil ingress class", func(t *testing.T) {
		nic := &approutingv1alpha1.NginxIngressController{}
		n := &nginxIngressControllerReconciler{}
		n.updateStatusIngressClass(nic, nil)

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeIngressClassReady)
		require.NotNil(t, got)
		require.Equal(t, metav1.ConditionUnknown, got.Status)
	})

	t.Run(("ingress clas with no creation timestamp"), func(t *testing.T) {
		nic := &approutingv1alpha1.NginxIngressController{}
		n := &nginxIngressControllerReconciler{}
		n.updateStatusIngressClass(nic, &netv1.IngressClass{})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeIngressClassReady)
		require.NotNil(t, got)
		require.Equal(t, metav1.ConditionUnknown, got.Status)
	})

	t.Run("ingress class with creation timestamp", func(t *testing.T) {
		nic := &approutingv1alpha1.NginxIngressController{}
		n := &nginxIngressControllerReconciler{}
		n.updateStatusIngressClass(nic, &netv1.IngressClass{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Now(),
			},
			Spec: netv1.IngressClassSpec{},
		})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeIngressClassReady)
		require.NotNil(t, got)
		require.Equal(t, metav1.ConditionTrue, got.Status)

	})
}

func TestUpdateStatusNilDeployment(t *testing.T) {
	nic := &approutingv1alpha1.NginxIngressController{}
	n := &nginxIngressControllerReconciler{}
	n.updateStatusNilDeployment(nic)

	controllerAvailable := nic.GetCondition(approutingv1alpha1.ConditionTypeControllerAvailable)
	require.NotNil(t, controllerAvailable)
	require.Equal(t, metav1.ConditionUnknown, controllerAvailable.Status)

	controllerProgressing := nic.GetCondition(approutingv1alpha1.ConditionTypeProgressing)
	require.NotNil(t, controllerProgressing)
	require.Equal(t, metav1.ConditionUnknown, controllerProgressing.Status)
}

func TestUpdateStatusControllerReplicas(t *testing.T) {
	t.Run("nil deployment", func(t *testing.T) {
		nic := &approutingv1alpha1.NginxIngressController{}
		n := &nginxIngressControllerReconciler{}
		n.updateStatusControllerReplicas(nic, nil)
		require.Equal(t, int32(0), nic.Status.ControllerReplicas)
		require.Equal(t, int32(0), nic.Status.ControllerReadyReplicas)
		require.Equal(t, int32(0), nic.Status.ControllerAvailableReplicas)
		require.Equal(t, int32(0), nic.Status.ControllerUnavailableReplicas)
	})

	t.Run("deployment with replicas", func(t *testing.T) {
		nic := &approutingv1alpha1.NginxIngressController{}
		n := &nginxIngressControllerReconciler{}
		deployment := &appsv1.Deployment{
			Status: appsv1.DeploymentStatus{
				Replicas:            1,
				ReadyReplicas:       2,
				AvailableReplicas:   1,
				UnavailableReplicas: 5,
			},
		}
		n.updateStatusControllerReplicas(nic, deployment)
		require.Equal(t, int32(1), nic.Status.ControllerReplicas)
		require.Equal(t, int32(2), nic.Status.ControllerReadyReplicas)
		require.Equal(t, int32(1), nic.Status.ControllerAvailableReplicas)
		require.Equal(t, int32(5), nic.Status.ControllerUnavailableReplicas)
	})
}

func TestUpdateStatusAvailable(t *testing.T) {
	cases := []struct {
		name                string
		controllerAvailable *metav1.Condition
		icAvailable         *metav1.Condition
		expected            *metav1.Condition
	}{
		{
			name: "controller available, ic available",
			controllerAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeControllerAvailable,
				Status: metav1.ConditionTrue,
			},
			icAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeIngressClassReady,
				Status: metav1.ConditionTrue,
			},
			expected: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeAvailable,
				Status: metav1.ConditionTrue,
			},
		},
		{
			name: "controller not available, ic available",
			controllerAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeControllerAvailable,
				Status: metav1.ConditionFalse,
			},
			icAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeIngressClassReady,
				Status: metav1.ConditionTrue,
			},
			expected: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeAvailable,
				Status: metav1.ConditionFalse,
			},
		},
		{
			name: "controller available, ic not available",
			controllerAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeControllerAvailable,
				Status: metav1.ConditionTrue,
			},
			icAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeIngressClassReady,
				Status: metav1.ConditionFalse,
			},
			expected: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeAvailable,
				Status: metav1.ConditionFalse,
			},
		},
		{
			name: "controller not available, ic not available",
			controllerAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeControllerAvailable,
				Status: metav1.ConditionFalse,
			},
			icAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeIngressClassReady,
				Status: metav1.ConditionFalse,
			},
			expected: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeAvailable,
				Status: metav1.ConditionFalse,
			},
		},
		{
			name:                "nil controller condition, ic available",
			controllerAvailable: nil,
			icAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeIngressClassReady,
				Status: metav1.ConditionTrue,
			},
			expected: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeAvailable,
				Status: metav1.ConditionFalse,
			},
		},
		{
			name: "nil ic condition, controller available",
			controllerAvailable: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeControllerAvailable,
				Status: metav1.ConditionTrue,
			},
			icAvailable: nil,
			expected: &metav1.Condition{
				Type:   approutingv1alpha1.ConditionTypeAvailable,
				Status: metav1.ConditionFalse,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			nic := &approutingv1alpha1.NginxIngressController{
				Status: approutingv1alpha1.NginxIngressControllerStatus{
					Conditions: []metav1.Condition{},
				},
			}

			if c.controllerAvailable != nil {
				nic.Status.Conditions = append(nic.Status.Conditions, *c.controllerAvailable)
			}
			if c.icAvailable != nil {
				nic.Status.Conditions = append(nic.Status.Conditions, *c.icAvailable)
			}

			n := &nginxIngressControllerReconciler{}
			n.updateStatusAvailable(nic)

			got := nic.GetCondition(approutingv1alpha1.ConditionTypeAvailable)
			if got == nil && c.expected == nil {
				return
			}

			require.Equal(t, c.expected.Status, got.Status)
		})
	}
}

func TestUpdateStatusFromError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		recorder := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			events: recorder,
		}
		nic := &approutingv1alpha1.NginxIngressController{}
		n.updateStatusFromError(nic, nil)
		require.True(t, len(nic.Status.Conditions) == 0)
		select {
		case <-recorder.Events:
			require.Fail(t, "unexpected event")
		default:
			// no events, we're good
		}
	})

	t.Run("non nil, non handled error", func(t *testing.T) {
		recorder := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			events: recorder,
		}
		nic := &approutingv1alpha1.NginxIngressController{}
		n.updateStatusFromError(nic, errors.New("test error"))
		require.True(t, len(nic.Status.Conditions) == 0)
		select {
		case <-recorder.Events:
			require.Fail(t, "unexpected event")
		default:
			// no events, we're good
		}
	})

	t.Run("ingressClass collision error", func(t *testing.T) {
		recorder := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			events: recorder,
		}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 1
		n.updateStatusFromError(nic, icCollisionErr)
		got := nic.GetCondition(approutingv1alpha1.ConditionTypeProgressing)
		require.True(t, got.Status == metav1.ConditionFalse)
		require.True(t, got.ObservedGeneration == nic.Generation)

		event := <-recorder.Events
		require.Equal(t, event, "Warning IngressClassCollision IngressClass already exists and is not owned by this controller. Change the spec.IngressClassName to an unused IngressClass name in a new NginxIngressController.")
	})

	t.Run("max collision error", func(t *testing.T) {
		recorder := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			events: recorder,
		}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 1
		n.updateStatusFromError(nic, maxCollisionsErr)
		got := nic.GetCondition(approutingv1alpha1.ConditionTypeProgressing)
		require.True(t, got.Status == metav1.ConditionFalse)
		require.True(t, got.ObservedGeneration == nic.Generation)

		event := <-recorder.Events
		require.Equal(t, event, "Warning TooManyCollisions Too many collisions with existing resources. Change the spec.ControllerNamePrefix to something more unique in a new NginxIngressController.")
	})
}

func TestUpdateStatusControllerAvailable(t *testing.T) {
	t.Run("non deployment available condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		n.updateStatusControllerAvailable(nic, appsv1.DeploymentCondition{})
		require.True(t, len(nic.Status.Conditions) == 0)
	})

	t.Run("deployment available true condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 1
		n.updateStatusControllerAvailable(nic, appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentAvailable,
			Status: corev1.ConditionTrue,
		})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeControllerAvailable)
		require.True(t, got.Status == metav1.ConditionTrue)
		require.True(t, got.ObservedGeneration == nic.Generation)
	})

	t.Run("deployment available false condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 2
		n.updateStatusControllerAvailable(nic, appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentAvailable,
			Status: corev1.ConditionFalse,
		})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeControllerAvailable)
		require.True(t, got.Status == metav1.ConditionFalse)
		require.True(t, got.ObservedGeneration == nic.Generation)
	})

	t.Run("deployment available unknown condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 3
		n.updateStatusControllerAvailable(nic, appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentAvailable,
			Status: corev1.ConditionUnknown,
		})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeControllerAvailable)
		require.True(t, got.Status == metav1.ConditionUnknown)
		require.True(t, got.ObservedGeneration == nic.Generation)
	})
}

func TestUpdateStatusControllerProgressing(t *testing.T) {
	t.Run("non deployment progressing condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		n.updateStatusControllerProgressing(nic, appsv1.DeploymentCondition{})
		require.True(t, len(nic.Status.Conditions) == 0)
	})

	t.Run("deployment progressing true condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 1
		n.updateStatusControllerProgressing(nic, appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionTrue,
		})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeProgressing)
		require.True(t, got.Status == metav1.ConditionTrue)
		require.True(t, got.ObservedGeneration == nic.Generation)
	})

	t.Run("deployment progressing false condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 2
		n.updateStatusControllerProgressing(nic, appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionFalse,
		})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeProgressing)
		require.True(t, got.Status == metav1.ConditionFalse)
		require.True(t, got.ObservedGeneration == nic.Generation)
	})

	t.Run("deployment progressing unknown condition", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		nic := &approutingv1alpha1.NginxIngressController{}
		nic.Generation = 3
		n.updateStatusControllerProgressing(nic, appsv1.DeploymentCondition{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionUnknown,
		})

		got := nic.GetCondition(approutingv1alpha1.ConditionTypeProgressing)
		require.True(t, got.Status == metav1.ConditionUnknown)
		require.True(t, got.ObservedGeneration == nic.Generation)
	})
}

func TestIsUnreconcilableError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "non unreconcilable error",
			err:  errors.New("some error"),
			want: false,
		},
		{
			name: "unreconcilable error",
			err:  icCollisionErr,
			want: true,
		},
		{
			name: "another unreconcilable error",
			err:  maxCollisionsErr,
			want: true,
		},
		{
			name: "wrapped unreconcilable error",
			err:  fmt.Errorf("wrapped: %w", icCollisionErr),
			want: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isUnreconcilableError(c.err)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
