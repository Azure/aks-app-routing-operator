// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/Azure/aks-app-routing-operator/e2e/e2eutil"
	"github.com/Azure/aks-app-routing-operator/e2e/fixtures"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	suite e2eutil.Suite
	conf  = &testConfig{Config: config.Flags}
)

type testConfig struct {
	*config.Config
	CertHostname              string
	TestNamservers            []string
	Kubeconfig                string
	CertID, CertVersionlessID string
}

func TestMain(m *testing.M) {
	// Load configuration
	rawConf, err := ioutil.ReadFile("../devenv/state/e2e.json")
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(rawConf, conf); err != nil {
		panic(err)
	}
	if err := conf.Validate(); err != nil {
		panic(err)
	}
	rc, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: conf.Kubeconfig},
		&clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		panic(err)
	}
	util.UseServerSideApply()
	suite.Clientset, err = kubernetes.NewForConfig(rc)
	if err != nil {
		panic(err)
	}

	// Start controllers
	manager, err := controller.NewManagerForRestConfig(config.Flags, rc)
	if err != nil {
		panic(err)
	}
	suite.Client = manager.GetClient()
	go manager.Start(context.Background())

	// Run tests
	suite.Purge()
	os.Exit(m.Run())
}

func TestBasicNginx(t *testing.T) {
	suite.StartTestCase(t).
		WithResources(
			fixtures.NewClientDeployment(t, conf.CertHostname, conf.TestNamservers),
			fixtures.NewGoDeployment(t, "server"),
			fixtures.NewService("server", conf.CertHostname, conf.CertID, 8080))
}

func TestBasicNginxNoOSM(t *testing.T) {
	svc := fixtures.NewService("server", conf.CertHostname, conf.CertID, 8080)
	svc.Annotations["kubernetes.azure.com/insecure-disable-osm"] = "true"

	svr := fixtures.NewGoDeployment(t, "server")
	svr.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"

	suite.StartTestCase(t).
		WithResources(
			fixtures.NewClientDeployment(t, conf.CertHostname, conf.TestNamservers),
			svr, svc)
}
