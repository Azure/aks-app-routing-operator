// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

func TestGetSelfDeploy(t *testing.T) {
	t.Run("deploy exists", func(t *testing.T) {
		kcs := fake.NewSimpleClientset()
		conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}

		ns := &corev1.Namespace{}
		ns.Name = conf.NS
		deploy := &appsv1.Deployment{}
		deploy.Name = conf.OperatorDeployment
		deploy.Namespace = conf.NS

		kcs.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
		kcs.AppsV1().Deployments(conf.NS).Create(context.Background(), deploy, metav1.CreateOptions{})

		self, err := getSelfDeploy(kcs, conf, logr.Discard())
		require.NoError(t, err)
		require.NotNil(t, self)
		assert.Equal(t, self.Name, deploy.Name)
	})

	t.Run("deploy missing", func(t *testing.T) {
		kcs := fake.NewSimpleClientset()
		conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator"}

		self, err := getSelfDeploy(kcs, conf, logr.Discard())
		require.NoError(t, err)
		require.Nil(t, self)
	})
}
