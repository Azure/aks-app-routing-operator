// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package keyvault

import (
	"context"
	"fmt"
	"os"
	"testing"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	keyVaultMountingErrorEvent = &corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			Name: "keyvault-test",
			Kind: "Pod",
		},
		Reason:  "FailedMount",
		Message: "test keyvault event",
	}
	keyVaultMountRotationFailedEvent = func() *corev1.Event {
		e := keyVaultMountingErrorEvent.DeepCopy()
		e.Reason = "MountRotationFailed"
		return e
	}()
	keyVaultSecretRotationFailedEvent = func() *corev1.Event {
		e := keyVaultMountingErrorEvent.DeepCopy()
		e.Reason = "SecretRotationFailed"
		return e
	}()
	keyVaultFailedToCreateSecretEvent = func() *corev1.Event {
		e := keyVaultMountingErrorEvent.DeepCopy()
		e.Reason = "FailedToCreateSecret"
		return e
	}()
	nonKeyVaultMountingErrorEventInvolvedObjectKind = func() *corev1.Event {
		e := keyVaultMountingErrorEvent.DeepCopy()
		e.InvolvedObject.Kind = "Service"
		return e
	}()
	nonKeyVaultMountingErrorEventInvolvedObjectName = func() *corev1.Event {
		e := keyVaultMountingErrorEvent.DeepCopy()
		e.InvolvedObject.Name = "test"
		return e
	}()
	nonKeyVaultMountingErrorEventReason = func() *corev1.Event {
		e := keyVaultMountingErrorEvent.DeepCopy()
		e.Reason = "FailedCreate"
		return e
	}()
	nonKeyVaultMountingErrorEventMessage = func() *corev1.Event {
		e := keyVaultMountingErrorEvent.DeepCopy()
		e.Message = "another vault message"
		return e
	}()
)

var (
	err        error
	restConfig *rest.Config
	env        *envtest.Environment
)

func TestMain(m *testing.M) {
	restConfig, env, err = testutils.StartTestingEnv()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testutils.CleanupTestingEnv(env)

	os.Exit(code)
}

func TestEventMirrorHappyPath(t *testing.T) {
	owner1 := &netv1.Ingress{}
	owner1.APIVersion = "networking.k8s.io/v1"
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

	beforeErrCount := testutils.GetErrMetricCount(t, eventMirrorControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess)
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	require.Equal(t, testutils.GetErrMetricCount(t, eventMirrorControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess), beforeReconcileCount)

	assert.Equal(t, "Warning FailedMount test keyvault event involvedObject{kind=Ingress,apiVersion=networking.k8s.io/v1}", <-recorder.Events)
}

func TestEventMirrorServiceOwnerHappyPath(t *testing.T) {
	owner0 := &corev1.Service{}
	owner0.Kind = "Service"
	owner0.APIVersion = "v1"
	owner0.Name = "owner0"
	owner0.Namespace = "testns"

	owner1 := &netv1.Ingress{}
	owner1.APIVersion = "networking.k8s.io/v1"
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

	beforeErrCount := testutils.GetErrMetricCount(t, eventMirrorControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess)
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	require.Equal(t, testutils.GetErrMetricCount(t, eventMirrorControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess), beforeReconcileCount)

	assert.Equal(t, "Warning FailedMount test keyvault event involvedObject{kind=Service,apiVersion=v1}", <-recorder.Events)
	assert.Equal(t, "Warning FailedMount test keyvault event involvedObject{kind=Ingress,apiVersion=networking.k8s.io/v1}", <-recorder.Events)
}

// newReconcileEventTest is a helper that runs a single reconcile for an event with the given
// reason and asserts it is mirrored to the owning ingress with that same reason.
func newReconcileEventTest(t *testing.T, reason string) {
	t.Helper()

	owner1 := &netv1.Ingress{}
	owner1.APIVersion = "networking.k8s.io/v1"
	owner1.Kind = "Ingress"
	owner1.Name = "owner1"
	owner1.Namespace = "testns"

	owner2 := &corev1.Pod{}
	owner2.Name = "keyvault-owner2"
	owner2.Namespace = owner1.Namespace
	owner2.Annotations = map[string]string{"kubernetes.azure.com/ingress-owner": owner1.Name}

	ev := &corev1.Event{}
	ev.Name = "testevent"
	ev.Namespace = owner1.Namespace
	ev.Reason = reason
	ev.Message = "test keyvault event"
	ev.InvolvedObject.Namespace = owner2.Namespace
	ev.InvolvedObject.Name = owner2.Name
	ev.InvolvedObject.Kind = "Pod"
	ev.InvolvedObject.APIVersion = "v1"

	recorder := record.NewFakeRecorder(10)
	recorder.IncludeObject = true
	c := fake.NewClientBuilder().WithObjects(owner1, owner2, ev).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))

	ctx := logr.NewContext(context.Background(), logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ev.Namespace, Name: ev.Name}}

	e := &EventMirror{client: c, events: recorder}
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, fmt.Sprintf("Warning %s test keyvault event involvedObject{kind=Ingress,apiVersion=networking.k8s.io/v1}", reason), <-recorder.Events)
}

func TestEventMirrorMountRotationFailed(t *testing.T) {
	newReconcileEventTest(t, "MountRotationFailed")
}

func TestEventMirrorSecretRotationFailed(t *testing.T) {
	newReconcileEventTest(t, "SecretRotationFailed")
}

func TestEventMirrorFailedToCreateSecret(t *testing.T) {
	newReconcileEventTest(t, "FailedToCreateSecret")
}

func TestIgnoreNotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithObjects().Build()
	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	e := &EventMirror{
		client: c,
	}
	_, err := e.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "nonexist", Name: "nonexist"}})
	require.NoError(t, err, "Expected ignored not found error")
}

func TestNoLoggerFoundInContext(t *testing.T) {
	e := &EventMirror{}
	_, err := e.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "nonexist", Name: "nonexist"}})
	require.ErrorContains(t, err, "no logr.Logger was present")
}

func TestEventMirrorServiceOwnerMissingOwner(t *testing.T) {
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

	event := &corev1.Event{}
	event.Name = "testevent"
	event.Namespace = owner1.Namespace
	event.Reason = "FailedMount"
	event.Message = "test keyvault event"
	event.InvolvedObject.Namespace = "nonexist"
	event.InvolvedObject.Name = "nonexist"
	event.InvolvedObject.Kind = "Pod"
	event.InvolvedObject.APIVersion = "v1"

	recorder := record.NewFakeRecorder(10)
	recorder.IncludeObject = true
	c := fake.NewClientBuilder().WithObjects(owner0, owner1, event).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: event.Namespace, Name: event.Name}}

	e := &EventMirror{
		client: c,
		events: recorder,
	}

	beforeErrCount := testutils.GetErrMetricCount(t, eventMirrorControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess)
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	require.Equal(t, testutils.GetErrMetricCount(t, eventMirrorControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess), beforeReconcileCount)
}

func TestEventMirrorServiceOwnerIngressNotFound(t *testing.T) {
	owner0 := &corev1.Service{}
	owner0.Kind = "Service"
	owner0.Name = "owner0"
	owner0.Namespace = "testns"

	owner2 := &corev1.Pod{}
	owner2.Name = "keyvault-owner2"
	owner2.Namespace = owner0.Namespace
	owner2.Annotations = map[string]string{"kubernetes.azure.com/ingress-owner": owner0.Name}

	event := &corev1.Event{}
	event.Name = "testevent"
	event.Namespace = owner0.Namespace
	event.Reason = "FailedMount"
	event.Message = "test keyvault event"
	event.InvolvedObject.Namespace = owner2.Namespace
	event.InvolvedObject.Name = owner2.Name
	event.InvolvedObject.Kind = "Pod"
	event.InvolvedObject.APIVersion = "v1"

	recorder := record.NewFakeRecorder(10)
	recorder.IncludeObject = true
	c := fake.NewClientBuilder().WithObjects(owner0, owner2, event).Build()
	require.NoError(t, secv1.AddToScheme(c.Scheme()))

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: event.Namespace, Name: event.Name}}

	e := &EventMirror{
		client: c,
		events: recorder,
	}

	beforeErrCount := testutils.GetErrMetricCount(t, eventMirrorControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess)
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	require.Equal(t, testutils.GetErrMetricCount(t, eventMirrorControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess), beforeReconcileCount)
}

func TestEventMirrorNonKeyVaultEvent(t *testing.T) {
	ev := nonKeyVaultMountingErrorEventReason.DeepCopy()
	ev.Name = "testevent"
	ev.Namespace = "testns"

	recorder := record.NewFakeRecorder(10)
	c := fake.NewClientBuilder().WithObjects(ev).Build()

	ctx := context.Background()
	ctx = logr.NewContext(ctx, logr.Discard())
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ev.Namespace, Name: ev.Name}}

	e := &EventMirror{client: c, events: recorder}

	beforeErrCount := testutils.GetErrMetricCount(t, eventMirrorControllerName)
	beforeReconcileCount := testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess)
	_, err := e.Reconcile(ctx, req)
	require.NoError(t, err)

	// Nothing should have been published to the recorder
	require.Empty(t, recorder.Events)
	require.Equal(t, testutils.GetErrMetricCount(t, eventMirrorControllerName), beforeErrCount)
	require.Greater(t, testutils.GetReconcileMetricCount(t, eventMirrorControllerName, metrics.LabelSuccess), beforeReconcileCount)
}

func TestNewEventMirror(t *testing.T) {
	m, err := manager.New(restConfig, manager.Options{Metrics: metricsserver.Options{BindAddress: ":0"}})
	require.NoError(t, err)
	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}
	err = NewEventMirror(m, conf)
	require.NoError(t, err, "should not error")
}

func TestNewPredicates(t *testing.T) {
	e := &EventMirror{}

	predicates := e.newPredicates()

	// CreateFunc should pass through only keyvault mounting error events
	require.True(t, predicates.Create(event.CreateEvent{Object: keyVaultMountingErrorEvent}))
	require.True(t, predicates.Create(event.CreateEvent{Object: keyVaultMountRotationFailedEvent}))
	require.False(t, predicates.Create(event.CreateEvent{Object: nonKeyVaultMountingErrorEventReason}))
	require.False(t, predicates.Create(event.CreateEvent{Object: nonKeyVaultMountingErrorEventInvolvedObjectKind}))
	require.False(t, predicates.Create(event.CreateEvent{}))

	require.False(t, predicates.Update(event.UpdateEvent{}))
	require.False(t, predicates.Delete(event.DeleteEvent{}))
	require.False(t, predicates.Generic(event.GenericEvent{}))
}

func TestIsKeyVaultMountingError(t *testing.T) {
	cases := []struct {
		name     string
		event    *corev1.Event
		expected bool
	}{
		{
			name:     "FailedMount keyvault pod event",
			event:    keyVaultMountingErrorEvent,
			expected: true,
		},
		{
			name:     "MountRotationFailed keyvault pod event",
			event:    keyVaultMountRotationFailedEvent,
			expected: true,
		},
		{
			name:     "SecretRotationFailed keyvault pod event",
			event:    keyVaultSecretRotationFailedEvent,
			expected: true,
		},
		{
			name:     "FailedToCreateSecret keyvault pod event",
			event:    keyVaultFailedToCreateSecretEvent,
			expected: true,
		},
		{
			name:     "non-pod involved object kind",
			event:    nonKeyVaultMountingErrorEventInvolvedObjectKind,
			expected: false,
		},
		{
			name:     "non-keyvault pod name prefix",
			event:    nonKeyVaultMountingErrorEventInvolvedObjectName,
			expected: false,
		},
		{
			name:     "unrecognised reason",
			event:    nonKeyVaultMountingErrorEventReason,
			expected: false,
		},
		{
			name:     "non-keyvault message",
			event:    nonKeyVaultMountingErrorEventMessage,
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isKeyVaultMountingError(tc.event))
		})
	}
}
