package nginxingress

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileResources(t *testing.T) {
	t.Run("nil nic", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		_, err := n.ReconcileResource(context.Background(), nil, nil)
		require.ErrorContains(t, err, "nginxIngressController cannot be nil")
	})

	t.Run("nil resources", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		_, err := n.ReconcileResource(context.Background(), &approutingv1alpha1.NginxIngressController{}, nil)
		require.Error(t, err, "resources cannot be nil")
	})

	t.Run("valid resources", func(t *testing.T) {
		cl := fake.NewFakeClient()
		events := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			conf: &config.Config{
				NS: "default",
			},
			client: cl,
			events: events,
		}

		nic := &approutingv1alpha1.NginxIngressController{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nic",
			},
			Spec: approutingv1alpha1.NginxIngressControllerSpec{
				IngressClassName:     "ingressClassName",
				ControllerNamePrefix: "prefix",
			},
		}
		res := n.ManagedResources(nic)

		managed, err := n.ReconcileResource(context.Background(), nic, res)
		require.NoError(t, err)
		require.True(t, len(managed) == len(res.Objects())-1, "expected all resources to be returned as managed except the namespace")

		// prove objects were created
		for _, obj := range res.Objects() {
			require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj))
		}

		// no events
		select {
		case <-events.Events:
			require.Fail(t, "expected no events")
		default:
		}
	})

	t.Run("valid resources with defaultSSLCertificate Secret", func(t *testing.T) {
		cl := fake.NewFakeClient()
		events := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			conf: &config.Config{
				NS: "default",
			},
			client: cl,
			events: events,
		}

		nic := &approutingv1alpha1.NginxIngressController{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nic",
			},
			Spec: approutingv1alpha1.NginxIngressControllerSpec{
				IngressClassName:      "ingressClassName",
				ControllerNamePrefix:  "prefix",
				DefaultSSLCertificate: &approutingv1alpha1.DefaultSSLCertificate{Secret: &approutingv1alpha1.Secret{Name: "test-name", Namespace: "test-namespace"}},
			},
		}
		res := n.ManagedResources(nic)

		managed, err := n.ReconcileResource(context.Background(), nic, res)
		require.NoError(t, err)
		require.True(t, len(managed) == len(res.Objects())-1, "expected all resources to be returned as managed except the namespace")

		// prove objects were created
		for _, obj := range res.Objects() {
			require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj))
		}

		// no events
		select {
		case <-events.Events:
			require.Fail(t, "expected no events")
		default:
		}
	})

	t.Run("valid resources with defaultSSLCertificate key vault URI", func(t *testing.T) {
		cl := fake.NewFakeClient()
		events := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			conf: &config.Config{
				NS: "default",
			},
			client: cl,
			events: events,
		}
		kvUri := "https://testvault.vault.azure.net/certificates/testcert/f8982febc6894c0697b884f946fb1a34"
		nic := &approutingv1alpha1.NginxIngressController{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nic",
			},
			Spec: approutingv1alpha1.NginxIngressControllerSpec{
				IngressClassName:      "ingressClassName",
				ControllerNamePrefix:  "prefix",
				DefaultSSLCertificate: &approutingv1alpha1.DefaultSSLCertificate{KeyVaultURI: &kvUri},
			},
		}
		res := n.ManagedResources(nic)

		managed, err := n.ReconcileResource(context.Background(), nic, res)
		require.NoError(t, err)
		require.True(t, len(managed) == len(res.Objects())-1, "expected all resources to be returned as managed except the namespace")

		// prove objects were created
		for _, obj := range res.Objects() {
			require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(obj), obj))
		}

		// no events
		select {
		case <-events.Events:
			require.Fail(t, "expected no events")
		default:
		}
	})

	t.Run("invalid resources", func(t *testing.T) {
		cl := fake.NewFakeClient()
		events := record.NewFakeRecorder(10)
		n := &nginxIngressControllerReconciler{
			conf: &config.Config{
				NS: "otherNamespaceThatDoesntExistYet",
			},
			client: cl,
			events: events,
		}

		nic := &approutingv1alpha1.NginxIngressController{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nic",
			},
			Spec: approutingv1alpha1.NginxIngressControllerSpec{
				IngressClassName:     "ingressClassName",
				ControllerNamePrefix: "prefix",
			},
		}
		res := n.ManagedResources(nic)
		res.Deployment = &appsv1.Deployment{} // invalid deployment

		_, err := n.ReconcileResource(context.Background(), nic, res)
		require.ErrorContains(t, err, "upserting object: ")

		// prove event was created
		e := <-events.Events
		require.Equal(t, "Warning EnsuringManagedResourcesFailed Failed to ensure managed resource  /:  \"\" is invalid: metadata.name: Required value: name is required", e)
	})
}

func TestManagedResources(t *testing.T) {
	t.Run("nil nic", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{}
		require.Nil(t, n.ManagedResources(nil))
	})

	t.Run("standard nic", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{
			conf: &config.Config{
				NS: "default",
			},
		}
		nic := &approutingv1alpha1.NginxIngressController{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nic",
			},
			Spec: approutingv1alpha1.NginxIngressControllerSpec{
				IngressClassName:     "ingressClassName",
				ControllerNamePrefix: "prefix",
			},
		}

		resources := n.ManagedResources(nic)
		require.NotNil(t, resources)
		require.Equal(t, nic.Spec.IngressClassName, resources.IngressClass.Name)
		require.True(t, strings.HasPrefix(resources.Deployment.Name, nic.Spec.ControllerNamePrefix))

		// check that we are only putting owner references on managed resources
		ownerRef := manifests.GetOwnerRefs(nic, true)
		require.Equal(t, ownerRef, resources.Deployment.OwnerReferences)
		require.NotEqual(t, ownerRef, resources.Namespace.OwnerReferences)
	})

	t.Run("nic with load balancer annotations", func(t *testing.T) {
		n := &nginxIngressControllerReconciler{
			conf: &config.Config{
				NS: "default",
			},
		}
		nic := &approutingv1alpha1.NginxIngressController{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nic",
			},
			Spec: approutingv1alpha1.NginxIngressControllerSpec{
				IngressClassName:     "ingressClassName",
				ControllerNamePrefix: "prefix",
				LoadBalancerAnnotations: map[string]string{
					"foo": "bar",
				},
			},
		}

		resources := n.ManagedResources(nic)
		require.NotNil(t, resources)
		require.Equal(t, nic.Spec.IngressClassName, resources.IngressClass.Name)
		require.True(t, strings.HasPrefix(resources.Deployment.Name, nic.Spec.ControllerNamePrefix))

		// check that we are only putting owner references on managed resources
		ownerRef := manifests.GetOwnerRefs(nic, true)
		require.Equal(t, ownerRef, resources.Deployment.OwnerReferences)
		require.NotEqual(t, ownerRef, resources.Namespace.OwnerReferences)

		// verify load balancer annotations
		for k, v := range nic.Spec.LoadBalancerAnnotations {
			require.Equal(t, v, resources.Service.Annotations[k])
		}
	})

	t.Run("default nic", func(t *testing.T) {
		defaultNicControllerClass := "defaultNicControllerClass"
		n := &nginxIngressControllerReconciler{
			conf: &config.Config{
				NS: "default",
			},
			defaultNicControllerClass: defaultNicControllerClass,
		}
		nic := &approutingv1alpha1.NginxIngressController{
			ObjectMeta: metav1.ObjectMeta{
				Name: DefaultNicName,
			},
			Spec: approutingv1alpha1.NginxIngressControllerSpec{
				IngressClassName:     DefaultIcName,
				ControllerNamePrefix: "nginx",
			},
		}

		resources := n.ManagedResources(nic)
		require.NotNil(t, resources)
		require.Equal(t, nic.Spec.IngressClassName, resources.IngressClass.Name)
		require.Equal(t, defaultNicControllerClass, resources.IngressClass.Spec.Controller)
		require.Equal(t, nic.Spec.ControllerNamePrefix, resources.Deployment.Name)

		// check that we are only putting owner references on managed resources
		ownerRef := manifests.GetOwnerRefs(nic, true)
		require.Equal(t, ownerRef, resources.Deployment.OwnerReferences)
		require.NotEqual(t, ownerRef, resources.Namespace.OwnerReferences)
	})
}

func TestGetCollisionCount(t *testing.T) {
	ctx := context.Background()
	cl := fake.NewClientBuilder().Build()

	n := &nginxIngressControllerReconciler{
		client: cl,
		conf: &config.Config{
			NS: "default",
		},
	}

	// standard collisions
	nic := &approutingv1alpha1.NginxIngressController{
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			ControllerNamePrefix: "sameControllerNamePrefixForAll",
		},
	}
	for i := 0; i <= approutingv1alpha1.MaxCollisions; i++ {
		copy := nic.DeepCopy()
		copy.Name = fmt.Sprintf("nic%d", i)
		copy.Spec.IngressClassName = fmt.Sprintf("ingressClassName%d", i)

		count, err := n.GetCollisionCount(ctx, copy)

		if i == approutingv1alpha1.MaxCollisions {
			require.Equal(t, maxCollisionsErr, err)
			continue
		}

		require.NoError(t, err)
		require.Equal(t, int32(i), count)

		copy.Status.CollisionCount = count
		for _, obj := range n.ManagedResources(copy).Objects() {
			cl.Create(ctx, obj)
		}
	}

	// ic collision
	collisionIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "collisionIngressClassName",
		},
	}
	require.NoError(t, cl.Create(ctx, collisionIc))
	nic2 := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "collisionNic",
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName:     collisionIc.Name,
			ControllerNamePrefix: "controllerNameUnique",
		},
	}
	_, err := n.GetCollisionCount(ctx, nic2)
	require.Equal(t, icCollisionErr, err)
}

func TestCollides(t *testing.T) {
	ctx := context.Background()
	cl := fake.NewClientBuilder().Build()

	collisionNic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "collision",
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName:     "ingressClassName",
			ControllerNamePrefix: "controllerName",
		},
	}

	n := &nginxIngressControllerReconciler{
		client: cl,
		conf:   &config.Config{NS: "default"},
	}
	for _, obj := range n.ManagedResources(collisionNic).Objects() {
		cl.Create(ctx, obj)
	}

	// ic collision
	collisionIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "collisionIngressClassName",
		},
	}
	require.NoError(t, cl.Create(ctx, collisionIc))
	nic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nic1",
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName:     collisionIc.Name,
			ControllerNamePrefix: "controllerName1",
		},
	}
	got, err := n.collides(ctx, nic)
	require.NoError(t, err)
	require.Equal(t, collisionIngressClass, got)

	// non ic collision
	nic2 := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nic12",
		},
		Spec: collisionNic.Spec,
	}
	nic2.Spec.IngressClassName = "anotherIngressClassName"
	got, err = n.collides(ctx, nic2)
	require.NoError(t, err)
	require.Equal(t, collisionOther, got)

	// no collision
	nic3 := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nic3",
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName:     "anotherIngressClassName",
			ControllerNamePrefix: "controllerName3",
		},
	}
	got, err = n.collides(ctx, nic3)
	require.NoError(t, err)
	require.Equal(t, collisionNone, got)

	// prove that we check ownership references for collisions and there will be no collision if  ownership references match
	for _, obj := range n.ManagedResources(collisionNic).Objects() {
		cl.Create(ctx, obj)
	}

	got, err = n.collides(ctx, nic3)
	require.NoError(t, err)
	require.Equal(t, collisionNone, got)

	// no collision for default nic
	defaultNic := &approutingv1alpha1.NginxIngressController{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNicName,
		},
		Spec: approutingv1alpha1.NginxIngressControllerSpec{
			IngressClassName: DefaultIcName,
		},
	}
	defaultIc := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultNic.Spec.IngressClassName,
		},
	}
	require.NoError(t, cl.Create(ctx, defaultIc))
	defaultSa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNicResourceName,
		},
	}
	require.NoError(t, cl.Create(ctx, defaultSa))
	got, err = n.collides(ctx, defaultNic)
	require.NoError(t, err)
	require.Equal(t, collisionNone, got)
}

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

func TestToNginxIngressConfig(t *testing.T) {
	defaultCc := "defaultControllerClass"
	FakeDefaultSSLCert := getFakeDefaultSSLCert("fake", "fakenamespace")
	FakeDefaultSSLCertNoName := getFakeDefaultSSLCert("", "fakenamespace")
	FakeDefaultSSLCertNoNamespace := getFakeDefaultSSLCert("fake", "")
	FakeDefaultBackend := "fakenamespace/fakename"
	cases := []struct {
		name string
		nic  *approutingv1alpha1.NginxIngressController
		want manifests.NginxIngressConfig
	}{
		{
			name: "default controller class",
			nic:  util.ToPtr(GetDefaultNginxIngressController()),
			want: manifests.NginxIngressConfig{
				ControllerClass: defaultCc,
				ResourceName:    DefaultNicResourceName,
				IcName:          DefaultIcName,
				ServiceConfig:   &manifests.ServiceConfig{},
			},
		},
		{
			name: "custom fields",
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nicName",
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName:     "ingressClassName",
					ControllerNamePrefix: "controllerNamePrefix",
				},
			},
			want: manifests.NginxIngressConfig{
				ControllerClass: "approuting.kubernetes.azure.com/nicName",
				ResourceName:    "controllerNamePrefix-0",
				ServiceConfig:   &manifests.ServiceConfig{},
				IcName:          "ingressClassName",
			},
		},
		{
			name: "custom fields with annotations",
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nicName",
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName:     "ingressClassName",
					ControllerNamePrefix: "controllerNamePrefix",
					LoadBalancerAnnotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			want: manifests.NginxIngressConfig{
				ControllerClass: "approuting.kubernetes.azure.com/nicName",
				ResourceName:    "controllerNamePrefix-0",
				ServiceConfig: &manifests.ServiceConfig{
					map[string]string{
						"foo": "bar",
					},
				},
				IcName: "ingressClassName",
			},
		},
		{
			name: "custom fields with long name",
			nic: &approutingv1alpha1.NginxIngressController{
				ObjectMeta: metav1.ObjectMeta{
					Name: strings.Repeat("a", 1000),
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					IngressClassName:     "ingressClassName",
					ControllerNamePrefix: "controllerNamePrefix",
				},
			},
			want: manifests.NginxIngressConfig{
				ControllerClass: ("approuting.kubernetes.azure.com/" + strings.Repeat("a", 1000))[:controllerClassMaxLen],
				ResourceName:    "controllerNamePrefix-0",
				ServiceConfig:   &manifests.ServiceConfig{},
				IcName:          "ingressClassName",
			},
		},
		{
			name: "default controller class with DefaultSSLCertificate",
			nic: &approutingv1alpha1.NginxIngressController{
				TypeMeta: metav1.TypeMeta{
					APIVersion: approutingv1alpha1.GroupVersion.String(),
					Kind:       "NginxIngressController",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultNicName,
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					ControllerNamePrefix:  DefaultNicResourceName,
					IngressClassName:      DefaultIcName,
					DefaultSSLCertificate: FakeDefaultSSLCert,
				},
			},
			want: manifests.NginxIngressConfig{
				ControllerClass:       defaultCc,
				ResourceName:          DefaultNicResourceName,
				IcName:                DefaultIcName,
				ServiceConfig:         &manifests.ServiceConfig{},
				DefaultSSLCertificate: FakeDefaultSSLCert.Secret.Namespace + "/" + FakeDefaultSSLCert.Secret.Name,
			},
		},
		{
			name: "default controller class with DefaultSSLCertificate with no name",
			nic: &approutingv1alpha1.NginxIngressController{
				TypeMeta: metav1.TypeMeta{
					APIVersion: approutingv1alpha1.GroupVersion.String(),
					Kind:       "NginxIngressController",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultNicName,
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					ControllerNamePrefix:  DefaultNicResourceName,
					IngressClassName:      DefaultIcName,
					DefaultSSLCertificate: FakeDefaultSSLCertNoName,
				},
			},
			want: manifests.NginxIngressConfig{
				ControllerClass: defaultCc,
				ResourceName:    DefaultNicResourceName,
				IcName:          DefaultIcName,
				ServiceConfig:   &manifests.ServiceConfig{},
			},
		},
		{
			name: "default controller class with DefaultSSLCertificate with no namespace",
			nic: &approutingv1alpha1.NginxIngressController{
				TypeMeta: metav1.TypeMeta{
					APIVersion: approutingv1alpha1.GroupVersion.String(),
					Kind:       "NginxIngressController",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultNicName,
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					ControllerNamePrefix:  DefaultNicResourceName,
					IngressClassName:      DefaultIcName,
					DefaultSSLCertificate: FakeDefaultSSLCertNoNamespace,
				},
			},
			want: manifests.NginxIngressConfig{
				ControllerClass: defaultCc,
				ResourceName:    DefaultNicResourceName,
				IcName:          DefaultIcName,
				ServiceConfig:   &manifests.ServiceConfig{},
			},
		},
		{
			name: "default controller class with DefaultBackendService",
			nic: &approutingv1alpha1.NginxIngressController{
				TypeMeta: metav1.TypeMeta{
					APIVersion: approutingv1alpha1.GroupVersion.String(),
					Kind:       "NginxIngressController",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultNicName,
				},
				Spec: approutingv1alpha1.NginxIngressControllerSpec{
					ControllerNamePrefix:  DefaultNicResourceName,
					IngressClassName:      DefaultIcName,
					DefaultBackendService: &FakeDefaultBackend,
				},
			},
			want: manifests.NginxIngressConfig{
				ControllerClass:       defaultCc,
				ResourceName:          DefaultNicResourceName,
				IcName:                DefaultIcName,
				ServiceConfig:         &manifests.ServiceConfig{},
				DefaultBackendService: FakeDefaultBackend,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ToNginxIngressConfig(c.nic, defaultCc)
			require.Equal(t, c.want, *got)
		})
	}
}

func getFakeDefaultSSLCert(name, namespace string) *approutingv1alpha1.DefaultSSLCertificate {
	fakecert := &approutingv1alpha1.DefaultSSLCertificate{
		Secret: &approutingv1alpha1.Secret{
			Name:      name,
			Namespace: namespace,
		},
	}
	return fakecert
}
