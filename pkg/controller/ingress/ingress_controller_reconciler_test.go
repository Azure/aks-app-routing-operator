// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

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
)

func TestIngressControllerReconcilerEmpty(t *testing.T) {
	c := fake.NewClientBuilder().Build()
	i := &IngressControllerReconciler{
		client:    c,
		resources: []client.Object{},
		logger:    logr.Discard(),
	}
	require.NoError(t, i.tick(context.Background()))
}

func TestIngressControllerReconcilerIntegration(t *testing.T) {
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

	// Create resource
	i := &IngressControllerReconciler{
		client:    c,
		resources: []client.Object{obj},
		logger:    logr.Discard(),
	}
	require.NoError(t, i.tick(context.Background()))

	// Prove the resource exists
	actual := &corev1.Namespace{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual))

	// Delete resource
	obj.DeletionTimestamp = &metav1.Time{}
	require.NoError(t, i.tick(context.Background()))

	// Prove the resource doesn't exist
	require.True(t, errors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(obj), actual)))
}
