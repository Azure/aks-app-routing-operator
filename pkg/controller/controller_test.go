// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

func TestCheckNamespace(t *testing.T) {
	t.Run("namespace exists", func(t *testing.T) {
		kcs := fake.NewSimpleClientset()
		conf := &config.Config{NS: "app-routing-system"}

		ns := &corev1.Namespace{}
		ns.Name = conf.NS
		kcs.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})

		require.NoError(t, checkNamespace(kcs, conf))
		assert.Equal(t, "app-routing-system", conf.NS)
	})

	t.Run("namespace deleting", func(t *testing.T) {
		kcs := fake.NewSimpleClientset()
		conf := &config.Config{NS: "app-routing-system"}

		ns := &corev1.Namespace{}
		ns.Name = conf.NS
		ns.DeletionTimestamp = &metav1.Time{}
		kcs.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})

		require.NoError(t, checkNamespace(kcs, conf))
		assert.Equal(t, "kube-system", conf.NS)
	})

	t.Run("namespace missing", func(t *testing.T) {
		kcs := fake.NewSimpleClientset()
		conf := &config.Config{NS: "app-routing-system"}

		require.NoError(t, checkNamespace(kcs, conf))
		assert.Equal(t, "kube-system", conf.NS)
	})

	t.Run("kube-system", func(t *testing.T) {
		kcs := fake.NewSimpleClientset()

		conf := &config.Config{NS: "kube-system"}
		require.NoError(t, checkNamespace(kcs, conf))
		assert.Equal(t, "kube-system", conf.NS)
	})
}
