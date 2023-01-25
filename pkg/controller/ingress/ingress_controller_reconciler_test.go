// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/informer"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	fakecgo "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIngressControllerReconcilerEmpty(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	cs := fakecgo.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(cs, time.Duration(0))
	ingClassInformer, err := informer.NewIngressClass(f)
	require.NoError(t, err)

	i := &IngressControllerReconciler{
		client:           c,
		resources:        []client.Object{},
		logger:           logr.Discard(),
		ingClassInformer: ingClassInformer,
	}
	require.NoError(t, i.tick(context.Background()))
}

func TestIngressControllerReconcilerIntegration(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	cs := fakecgo.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(cs, time.Duration(0))
	ingClassInformer, err := informer.NewIngressClass(f)
	require.NoError(t, err)

	obj := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	// Create resource
	controller := "webapprouting.kubernetes.azure.com/controller"
	i := &IngressControllerReconciler{
		client:           c,
		resources:        []client.Object{obj},
		logger:           logr.Discard(),
		ingClassInformer: ingClassInformer,
		controller:       controller,
	}
	require.NoError(t, i.tick(context.Background()))

	// Prove the resource doesn't exist yet
	actual := &corev1.Namespace{}
	require.True(t, errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)), "expected not found error")

	// Add a non-consuming ingressclass
	nonConsumingIngC := &netv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: "nonconsuming"}}
	f.Networking().V1().IngressClasses().Informer().GetIndexer().Add(nonConsumingIngC.DeepCopyObject()) // use a deep copy because we update later
	require.NoError(t, i.tick(context.Background()))

	// Prove that the resource still doesn't exist yet
	actual = &corev1.Namespace{}
	require.True(t, errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)), "expected not found error")

	// Add a consuming ingressclass
	ingC := &netv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{Name: "consuming"},
		Spec:       netv1.IngressClassSpec{Controller: controller},
	}
	f.Networking().V1().IngressClasses().Informer().GetIndexer().Add(ingC)
	require.NoError(t, i.tick(context.Background()))

	// Prove that the resource exists
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual))

	// Delete consuming ingressclass
	f.Networking().V1().IngressClasses().Informer().GetIndexer().Delete(ingC)
	require.NoError(t, i.tick(context.Background()))

	// Prove that the resource exists
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual))

	// Delete resource
	obj.DeletionTimestamp = &metav1.Time{}
	require.NoError(t, i.tick(context.Background()))

	// Prove the resource doesn't exist
	require.True(t, errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)), "expected not found error")

	// Update the non-consuming ingressclass to consume
	obj.DeletionTimestamp = nil
	nonConsumingIngC.Spec.Controller = controller
	f.Networking().V1().IngressClasses().Informer().GetIndexer().Update(nonConsumingIngC)
	require.NoError(t, i.tick(context.Background()))

	// Prove that the resource exists
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual))
}
