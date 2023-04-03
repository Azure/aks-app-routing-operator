// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

//go:build e2e

package e2e

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
	CertID, CertVersionlessID string
	DNSZoneDomain             string
	PromClientImage           string
}

func TestMain(m *testing.M) {
	// Load configuration
	rawConf := os.Getenv("E2E_JSON_CONTENTS")
	if rawConf == "" {
		panic(errors.New("failed to get e2e contents from env"))
	}
	if err := json.Unmarshal([]byte(rawConf), conf); err != nil {
		panic(err)
	}

	promClientImage := strings.TrimSpace(os.Getenv("PROM_CLIENT_IMAGE"))
	if promClientImage == "" {
		panic(errors.New("failed to get prometheus client image from env"))
	}
	conf.PromClientImage = promClientImage

	// attempt to load in-cluster config
	rc, err := rest.InClusterConfig()
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
		fixtures.NewGoDeployment(t, fixtures.Server),
		fixtures.NewService(fixtures.Server.String(), hostname, conf.CertID, 8080))
}

// TestBasicServiceVersionlessCert proves that users can remove the version hash from a Keyvault cert URI.
func TestBasicServiceVersionlessCert(t *testing.T) {
	t.Parallel()
	tc := suite.StartTestCase(t)
	hostname := tc.Hostname(conf.DNSZoneDomain)
	tc.WithResources(
		fixtures.NewClientDeployment(t, hostname, conf.TestNameservers),
		fixtures.NewGoDeployment(t, fixtures.Server),
		fixtures.NewService(fixtures.Server.String(), hostname, conf.CertVersionlessID, 8080))
}

// TestBasicServiceNoOSM is identical to TestBasicService but disables OSM.
func TestBasicServiceNoOSM(t *testing.T) {
	t.Parallel()
	tc := suite.StartTestCase(t)
	hostname := tc.Hostname(conf.DNSZoneDomain)

	svc := fixtures.NewService(fixtures.Server.String(), hostname, conf.CertID, 8080)
	svc.Annotations["kubernetes.azure.com/insecure-disable-osm"] = "true"

	svr := fixtures.NewGoDeployment(t, fixtures.Server)
	svr.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"

	tc.WithResources(
		fixtures.NewClientDeployment(t, hostname, conf.TestNameservers),
		svr, svc)
}

// TestPrometheus proves that users can consume Prometheus metrics emitted by our controllers
func TestPrometheus(t *testing.T) {
	t.Parallel()
	tc := suite.StartTestCase(t)
	ns := tc.NS()

	tc.WithResources(append(fixtures.NewPrometheus(ns), fixtures.NewPrometheusClient(ns, conf.PromClientImage))...)
}
