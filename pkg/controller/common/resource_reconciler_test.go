package common

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestResourceReconcilerEmpty(t *testing.T) {
	c := fake.NewClientBuilder().Build()

	rr := &resourceReconciler{
		name:      "test-name",
		client:    c,
		logger:    logr.Discard(),
		resources: []client.Object{},
	}
	require.NoError(t, rr.tick(context.Background()))
}

func TestResourceReconcilerIntegration(t *testing.T) {
	c := fake.NewClientBuilder().Build()

	obj := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	rr := &resourceReconciler{
		name:      "test-name",
		client:    c,
		logger:    logr.Discard(),
		resources: []client.Object{obj},
	}

	// prove the resource doesn't exist
	actual := &corev1.Namespace{}
	require.True(t,
		errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)),
		"expected not found error")

	// create resource
	require.NoError(t, rr.tick(context.Background()))
	require.NoError(t,
		c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual),
		"expected resource to exist")

	// delete the resource
	require.NoError(t, c.Delete(context.Background(), obj))
	require.True(t,
		errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)),
		"expected not found error")

	// prove the resource is recreated
	require.NoError(t, rr.tick(context.Background()))
	require.NoError(t,
		c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual),
		"expected resource to exist")
}

func TestResourceReconcilerLeaderElection(t *testing.T) {
	var ler manager.LeaderElectionRunnable = &resourceReconciler{}
	require.True(t, ler.NeedLeaderElection(), "should need leader election")
}
