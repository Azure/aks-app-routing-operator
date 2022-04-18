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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	suite.Client, err = client.New(rc, client.Options{})
	if err != nil {
		panic(err)
	}

	controllerUsername := "aks-app-routing-operator-e2e-user"
	if err := e2eutil.SetupRBAC(suite.Clientset, "../rbac.yaml", controllerUsername); err != nil {
		panic(err)
	}

	// Start controllers
	impersonateRC := rest.CopyConfig(rc)
	impersonateRC.Impersonate.UserName = controllerUsername
	manager, err := controller.NewManagerForRestConfig(config.Flags, impersonateRC)
	if err != nil {
		panic(err)
	}
	go manager.Start(context.Background())

	// Run tests
	suite.Purge()
	os.Exit(m.Run())
}

// TestBasicService is the most common user scenario - add annotations to a service, get back working
// ingress with TLS termination and e2e encryption using OSM.
func TestBasicService(t *testing.T) {
	suite.StartTestCase(t).
		WithResources(
			fixtures.NewClientDeployment(t, conf.CertHostname, conf.TestNamservers),
			fixtures.NewGoDeployment(t, "server"),
			fixtures.NewService("server", conf.CertHostname, conf.CertID, 8080))
}

// TestBasicServiceVersionlessCert proves that users can remove the version hash from a Keyvault cert URI.
func TestBasicServiceVersionlessCert(t *testing.T) {
	suite.StartTestCase(t).
		WithResources(
			fixtures.NewClientDeployment(t, conf.CertHostname, conf.TestNamservers),
			fixtures.NewGoDeployment(t, "server"),
			fixtures.NewService("server", conf.CertHostname, conf.CertVersionlessID, 8080))
}

// TestBasicServiceNoOSM is identical to TestBasicService but disables OSM.
func TestBasicServiceNoOSM(t *testing.T) {
	svc := fixtures.NewService("server", conf.CertHostname, conf.CertID, 8080)
	svc.Annotations["kubernetes.azure.com/insecure-disable-osm"] = "true"

	svr := fixtures.NewGoDeployment(t, "server")
	svr.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"

	suite.StartTestCase(t).
		WithResources(
			fixtures.NewClientDeployment(t, conf.CertHostname, conf.TestNamservers),
			svr, svc)
}
