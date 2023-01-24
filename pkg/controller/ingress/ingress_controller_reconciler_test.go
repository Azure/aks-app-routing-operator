// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/informer"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
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
	ingInformer, err := informer.NewIngress(f)
	require.NoError(t, err)

	i := &IngressControllerReconciler{
		client:      c,
		resources:   []client.Object{},
		logger:      logr.Discard(),
		ingInformer: ingInformer,
	}
	require.NoError(t, i.tick(context.Background()))
}

func TestIngressControllerReconcilerIntegration(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	cs := fakecgo.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(cs, time.Duration(0))
	ingInformer, err := informer.NewIngress(f)
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
	ingressCn := "testing.webapprouting.kubernetes.azure.com"
	i := &IngressControllerReconciler{
		client:      c,
		resources:   []client.Object{obj},
		logger:      logr.Discard(),
		ingInformer: ingInformer,
		className:   ingressCn,
	}
	require.NoError(t, i.tick(context.Background()))

	// Prove the resource doesn't exist yet
	actual := &corev1.Namespace{}
	require.True(t, errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)))

	// Add a non-consuming ingress
	nonConsumingIng := &netv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "nonconsuming"}}
	f.Networking().V1().Ingresses().Informer().GetIndexer().Add(nonConsumingIng)
	require.NoError(t, i.tick(context.Background()))

	// Prove that the resource still doesn't exist yet
	actual = &corev1.Namespace{}
	require.True(t, errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)))

	// Add a consuming ingress
	ing := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "consuming"},
		Spec:       netv1.IngressSpec{IngressClassName: util.StringPtr(ingressCn)},
	}
	f.Networking().V1().Ingresses().Informer().GetIndexer().Add(ing)
	require.NoError(t, i.tick(context.Background()))

	// Prove that the resource exists
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual))

	// Delete resource
	obj.DeletionTimestamp = &metav1.Time{}
	require.NoError(t, i.tick(context.Background()))

	// Prove the resource doesn't exist
	require.True(t, errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)))
}
