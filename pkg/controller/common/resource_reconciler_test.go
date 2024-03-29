package common

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestResourceReconcilerEmpty(t *testing.T) {
	c := fake.NewClientBuilder().Build()

	rr := &resourceReconciler{
		name:      controllername.New("test", "resource", "reconciler"),
		client:    c,
		logger:    logr.Discard(),
		resources: []client.Object{},
	}
	beforeErrCount := testutils.GetErrMetricCount(t, rr.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess)
	require.NoError(t, rr.tick(context.Background()))

	require.Equal(t, testutils.GetErrMetricCount(t, rr.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess), beforeReconcileCount)
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
		name:      controllername.New("test"),
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
	beforeErrCount := testutils.GetErrMetricCount(t, rr.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess)
	require.NoError(t, rr.tick(context.Background()))

	require.Equal(t, testutils.GetErrMetricCount(t, rr.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess), beforeReconcileCount)
	require.NoError(t,
		c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual),
		"expected resource to exist")

	// delete the resource
	require.NoError(t, c.Delete(context.Background(), obj))
	require.True(t,
		errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)),
		"expected not found error")

	// prove the resource is recreated
	beforeErrCount = testutils.GetErrMetricCount(t, rr.name)
	beforeReconcileCount = testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess)
	require.NoError(t, rr.tick(context.Background()))

	require.Equal(t, testutils.GetErrMetricCount(t, rr.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess), beforeReconcileCount)
	require.NoError(t,
		c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual),
		"expected resource to exist")
}

func TestResourceReconcilerLeaderElection(t *testing.T) {
	var ler manager.LeaderElectionRunnable = &resourceReconciler{}
	require.True(t, ler.NeedLeaderElection(), "should need leader election")
}

func TestNewResourceReconciler(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)
	err = NewResourceReconciler(m, controllername.New("test"), nil, 1*time.Nanosecond)
	require.NoError(t, err)
}

func TestResourceReconciler_DeletionTimestamp(t *testing.T) {
	deletionTimeStamp := metav1.Time{time.Now().Add(1 * time.Second)}
	obj := &corev1.Namespace{

		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			DeletionTimestamp: &deletionTimeStamp,
			Finalizers:        []string{"finalizer"},
		},
	}

	c := fake.NewClientBuilder().WithObjects(obj).Build()

	rr := &resourceReconciler{
		name:      controllername.New("test", "name"),
		client:    c,
		logger:    logr.Discard(),
		resources: []client.Object{obj},
	}

	obj.SetFinalizers([]string{})
	require.NoError(t, c.Update(context.Background(), obj))

	beforeErrCount := testutils.GetErrMetricCount(t, rr.name)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess)
	require.NoError(t, rr.tick(context.Background()))

	require.Equal(t, testutils.GetErrMetricCount(t, rr.name), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, rr.name, metrics.LabelSuccess), beforeReconcileCount)

	// prove object got deleted
	actual := &corev1.Namespace{}
	require.True(t,
		errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)),
		"expected not found error")
}
