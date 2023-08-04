// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

func TestEventMirrorHappyPath(t *testing.T) {
	owner1 := &netv1.Ingress{}
	owner1.APIVersion = "networking.k8s.io"
	owner1.Kind = "Ingress"
	owner1.Name = "owner1"
	owner1.Namespace = "testns"

	owner2 := &corev1.Pod{}
	owner2.Name = "keyvault-owner2"
	owner2.Namespace = owner1.Namespace
	owner2.Annotations = map[string]string{"kubernetes.azure.com/ingress-owner": owner1.Name}

	event := &corev1.Event{}
	event.Name = "testevent"
	event.Namespace = owner1.Namespace
	event.Reason = "FailedMount"
	event.Message = "test keyvault event"
	event.InvolvedObject.Namespace = owner2.Namespace
	event.InvolvedObject.Name = owner2.Name
	event.InvolvedObject.Kind = "Pod"
	event.InvolvedObject.APIVersion = "v1"

	recorder := record.NewFakeRecorder(10)
	recorder.IncludeObject = true
	c := fake.NewClientBuilder().WithObjects(owner1, owner2, event).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: event.Namespace, Name: event.Name}}

	e := &EventMirror{
		client: c,
		events: recorder,
	}

	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, "Warning FailedMount test keyvault event involvedObject{kind=Ingress,apiVersion=networking.k8s.io/v1}", <-recorder.Events)
}

func TestEventMirrorServiceOwnerHappyPath(t *testing.T) {
	owner0 := &corev1.Service{}
	owner0.Kind = "Service"
	owner0.Name = "owner0"
	owner0.Namespace = "testns"

	owner1 := &netv1.Ingress{}
	owner1.APIVersion = "networking.k8s.io"
	owner1.Kind = "Ingress"
	owner1.Name = "owner1"
	owner1.Namespace = owner0.Namespace
	owner1.OwnerReferences = []metav1.OwnerReference{{
		Kind: owner0.Kind,
		Name: owner0.Name,
	}}

	owner2 := &corev1.Pod{}
	owner2.Name = "keyvault-owner2"
	owner2.Namespace = owner1.Namespace
	owner2.Annotations = map[string]string{"kubernetes.azure.com/ingress-owner": owner1.Name}

	event := &corev1.Event{}
	event.Name = "testevent"
	event.Namespace = owner1.Namespace
	event.Reason = "FailedMount"
	event.Message = "test keyvault event"
	event.InvolvedObject.Namespace = owner2.Namespace
	event.InvolvedObject.Name = owner2.Name
	event.InvolvedObject.Kind = "Pod"
	event.InvolvedObject.APIVersion = "v1"

	recorder := record.NewFakeRecorder(10)
	recorder.IncludeObject = true
	c := fake.NewClientBuilder().WithObjects(owner0, owner1, owner2, event).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: event.Namespace, Name: event.Name}}

	e := &EventMirror{
		client: c,
		events: recorder,
	}

	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, "Warning FailedMount test keyvault event involvedObject{kind=Service,apiVersion=v1}", <-recorder.Events)
	assert.Equal(t, "Warning FailedMount test keyvault event involvedObject{kind=Ingress,apiVersion=networking.k8s.io/v1}", <-recorder.Events)
}

func TestNewEventMirror(t *testing.T) {
	m := getManager()
	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}
	err := NewEventMirror(m, conf)
	require.NoError(t, err, "should not error")
}

func getManager() manager.Manager {
	testenv := &envtest.Environment{}
	cfg, _ := testenv.Start()
	m, _ := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	return m
}
