// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

//go:build e2e

package e2e

import (
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
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var (
	suite e2eutil.Suite
	conf  = &testConfig{}
)

type testConfig struct {
	TestNameservers           []string
	Kubeconfig                string
	CertID, CertVersionlessID string
	DNSZoneDomain             string
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

	// attempt to load in-cluster config first
	rc, err := rest.InClusterConfig()
	if err != nil {
		rc, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: conf.Kubeconfig},
			&clientcmd.ConfigOverrides{}).ClientConfig()
		if err != nil {
			panic(err)
		}
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

	// Run tests
	suite.Purge()
	os.Exit(m.Run())
}

// TestBasicService is the most common user scenario - add annotations to a service, get back working
// ingress with TLS termination and e2e encryption using OSM.
func TestBasicService(t *testing.T) {
	t.Parallel()
	tc := suite.StartTestCase(t)
	hostname := tc.Hostname(conf.DNSZoneDomain)
	tc.WithResources(
		fixtures.NewClientDeployment(t, hostname, conf.TestNameservers),
		fixtures.NewGoDeployment(t, "server"),
		fixtures.NewService("server", hostname, conf.CertID, 8080))
}

// TestBasicServiceVersionlessCert proves that users can remove the version hash from a Keyvault cert URI.
func TestBasicServiceVersionlessCert(t *testing.T) {
	t.Parallel()
	tc := suite.StartTestCase(t)
	hostname := tc.Hostname(conf.DNSZoneDomain)
	tc.WithResources(
		fixtures.NewClientDeployment(t, hostname, conf.TestNameservers),
		fixtures.NewGoDeployment(t, "server"),
		fixtures.NewService("server", hostname, conf.CertVersionlessID, 8080))
}

// TestBasicServiceNoOSM is identical to TestBasicService but disables OSM.
func TestBasicServiceNoOSM(t *testing.T) {
	t.Parallel()
	tc := suite.StartTestCase(t)
	hostname := tc.Hostname(conf.DNSZoneDomain)

	svc := fixtures.NewService("server", hostname, conf.CertID, 8080)
	svc.Annotations["kubernetes.azure.com/insecure-disable-osm"] = "true"

	svr := fixtures.NewGoDeployment(t, "server")
	svr.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"

	tc.WithResources(
		fixtures.NewClientDeployment(t, hostname, conf.TestNameservers),
		svr, svc)
}
