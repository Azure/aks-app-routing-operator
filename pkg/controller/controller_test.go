// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/go-logr/logr"
	cfgv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controllerruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
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

func TestSetup(t *testing.T) {
	testConfig := &config.Config{
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
	testenv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "gatewaycrd"),
		},
	}

	testRestConfig, err := testenv.Start()
	require.NoError(t, err)

	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(secv1.Install(s))
	utilruntime.Must(cfgv1alpha2.AddToScheme(s))
	utilruntime.Must(policyv1alpha1.AddToScheme(s))
	utilruntime.Must(approutingv1alpha1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	utilruntime.Must(gatewayv1.Install(s))

	mgr, err := manager.New(testRestConfig, manager.Options{
		Scheme: s,
	})
	require.NoError(t, err)

	require.NoError(t, setupIndexers(mgr, logr.Discard(), testConfig))
	require.NoError(t, setupControllers(mgr, testConfig, logr.Discard(), controllerruntimefake.NewFakeClient()))
	require.NoError(t, setupProbes(testConfig, mgr, logr.Discard()))
}
