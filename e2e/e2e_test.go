// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

//adfgo:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"strings"
	"testing"

	"github.com/Azure/aks-app-routing-operator/e2e/e2eutil"
	"github.com/Azure/aks-app-routing-operator/e2e/fixtures"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"sigs.k8s.io/e2e-framework/pkg/env"
)

var (
	suite   e2eutil.Suite
	conf    = &testConfig{}
	testEnv env.Environment
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

	testEnv = env.NewInClusterConfig()

	testEnv.Setup(
		e2eutil.Purge)

	util.UseServerSideApply()

	testEnv.BeforeEachTest(func(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
		return e2eutil.CreateNSForTest(ctx, cfg, t)
	})
	testEnv.AfterEachTest(func(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
		return e2eutil.DeleteNSForTest(ctx, cfg, t)
	})

	// Run tests
	os.Exit(testEnv.Run(m))
}

// TestBasicService is the most common user scenario - add annotations to a service, get back working
// ingress with TLS termination and e2e encryption using OSM.
func TestBasicService(t *testing.T) {
	t.Parallel()
	tc := suite.StartTestCase(t)

	hostname := tc.Hostname(testEnv, conf.DNSZoneDomain)

	basicFeature := features.New("basic").
		Setup(func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			namespace := ctx.Value(e2eutil.GetNamespaceKey(t)).(string)

			clientDeployment := fixtures.NewClientDeployment(t, hostname, conf.TestNameservers, namespace)
			// Create Deployments and Service
			if err := client.Resources().Create(ctx); err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Assess("client deployment", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {

			return ctx
		}).Feature()

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
