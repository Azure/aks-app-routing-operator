// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	restConfig *rest.Config
	err        error
)

func TestMain(m *testing.M) {
	restConfig, err = testutils.StartTestingEnv()
	if err != nil {
		panic(err)
	}

	code := m.Run()
	testutils.CleanupTestingEnv()

	os.Exit(code)
}

func TestLogger(t *testing.T) {
	t.Run("logs are json structured", func(t *testing.T) {
		logOut := new(bytes.Buffer)
		logger := getLogger(zap.WriteTo(logOut))

		logger.Info("test info log", "key", "value", "key2", "value2")
		logger.Error(errors.New("test error log"), "msg", "key3", "values3")

		out := logOut.Bytes()
		checked := 0
		for _, line := range bytes.SplitAfter(out, []byte("}")) {
			if bytes.TrimSpace(line) == nil {
				continue
			}

			assert.True(t, json.Valid(line), "line is not valid json", string(line))
			assert.True(t, strings.Contains(string(line), "\"caller\":\"controller/controller_test.go"))
			checked++
		}

		assert.True(t, checked > 0, "no logs validated")
	})
}

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

func TestNewManagerForRestConfig(t *testing.T) {
	conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "operator-test", MetricsAddr: "0"}
	_, err := NewManagerForRestConfig(conf, restConfig)
	require.NoError(t, err)
}
