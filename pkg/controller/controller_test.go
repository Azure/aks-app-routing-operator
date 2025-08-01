// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/Azure/aks-app-routing-operator/pkg/store"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	controllerruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

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
		conf := &config.Config{NS: "app-routing-system", OperatorDeployment: "app-routing-operator"}

		ns := &corev1.Namespace{}
		ns.Name = conf.NS
		deploy := &appsv1.Deployment{}
		deploy.Name = conf.OperatorDeployment
		deploy.Namespace = ns.Name

		kcs.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
		kcs.AppsV1().Deployments(deploy.Namespace).Create(context.Background(), deploy, metav1.CreateOptions{})

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

func minimalTestConfig() *config.Config {
	return &config.Config{
		DefaultController:        config.Standard,
		NS:                       "test-namespace",
		Registry:                 "test-registry",
		MSIClientID:              "test-msi-client-id",
		TenantID:                 "test-tenant-id",
		Cloud:                    "test-cloud",
		Location:                 "test-location",
		ConcurrencyWatchdogThres: 101,
		ConcurrencyWatchdogVotes: 2,
		ClusterUid:               "test-cluster-uid",
		OperatorDeployment:       "app-routing-operator",
		CrdPath:                  validCrdPath,
	}
}

func TestSetup(t *testing.T) {
	testConfig := minimalTestConfig()
	testenv := testutils.NewTestEnvironment()

	testRestConfig, err := testenv.Start()
	require.NoError(t, err)

	s := testutils.NewTestScheme()

	mgr, err := manager.New(testRestConfig, manager.Options{
		Scheme: s,
	})
	require.NoError(t, err, "failed to create manager")

	store, err := store.New(logr.Discard(), context.Background())
	require.NoError(t, err, "failed to create store")

	require.NoError(t, setupIndexers(mgr, logr.Discard(), testConfig))
	require.NoError(t, setupControllers(mgr, testConfig, logr.Discard(), controllerruntimefake.NewFakeClient(), store))
	require.NoError(t, setupProbes(testConfig, mgr, logr.Discard()))
}
