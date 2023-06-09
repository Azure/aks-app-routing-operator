// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

//asdfgo:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/e2e/e2eutil"
	"github.com/Azure/aks-app-routing-operator/e2e/fixtures"
	"github.com/Azure/aks-app-routing-operator/pkg/util"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	conf    = &testConfig{}
	testEnv env.Environment
)

type zoneConfig struct {
	ZoneType                  string
	NameServer                string
	CertID, CertVersionlessID string
	DNSZoneId                 string
	Id                        string
}

var zoneConfigs []*zoneConfig

type testConfig struct {
	PublicNameservers map[string][]string
	PrivateNameserver string

	PublicCertIDs, PublicCertVersionlessIDs   map[string]string
	PrivateCertIDs, PrivateCertVersionlessIDs map[string]string

	CertID, CertVersionlessID           string
	PrivateDNSZoneIDs, PublicDNSZoneIDs []string
	PromClientImage                     string
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

	// Load zone configs
	zoneConfigs = generateZoneConfigs(conf)

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

	var clientDeployment, serverDeployment *appsv1.Deployment
	var service *corev1.Service

	genBasicFeatures := func(zoneconfig zoneConfig) features.Feature {
		return features.New("basic").
			Setup(func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
				client, err := config.NewClient()
				if err != nil {
					t.Fatal(err)
				}
				namespace := ctx.Value(e2eutil.GetNamespaceKey(t)).(string)

				clientDeployment, serverDeployment, service = generateTestingObjects(t, namespace, zoneconfig.CertID, zoneconfig)
				deployObjects(t, ctx, client, []k8s.Object{clientDeployment, serverDeployment, service})
				return ctx
			}).
			Assess("client deployment available", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
				client, err := config.NewClient()
				if err != nil {
					t.Fatal(err)
				}

				// Wait for client deployment to be ready
				if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(clientDeployment, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(5*time.Minute)); err != nil {
					t.Fatal(err)
				}

				return ctx
			}).Feature()
	}

	for _, config := range zoneConfigs {
		testEnv.Test(t, genBasicFeatures(*config))
	}
}

// TestBasicServiceVersionlessCert proves that users can remove the version hash from a Keyvault cert URI.
func TestBasicServiceVersionlessCert(t *testing.T) {
	t.Parallel()

	var (
		clientDeployment, serverDeployment *appsv1.Deployment
		service                            *corev1.Service
	)

	versionlessFeature := features.New("versionlessCert").
		Setup(func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			namespace := ctx.Value(e2eutil.GetNamespaceKey(t)).(string)

			clientDeployment, serverDeployment, service = generateTestingObjects(t, conf.CertVersionlessID, namespace)
			deployObjects(t, ctx, client, []k8s.Object{clientDeployment, serverDeployment, service})
			return ctx
		}).
		Assess("client deployment available", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}

			// Wait for client deployment to be ready
			if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(clientDeployment, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(5*time.Minute)); err != nil {
				t.Fatal(err)
			}

			return ctx
		}).Feature()

	testEnv.Test(t, versionlessFeature)
}

// TestBasicServiceNoOSM is identical to TestBasicService but disables OSM.
func TestBasicServiceNoOSM(t *testing.T) {
	t.Parallel()

	var (
		clientDeployment, svr *appsv1.Deployment
		svc                   *corev1.Service
	)

	noOSMFeature := features.New("noOSM").
		Setup(func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			namespace := ctx.Value(e2eutil.GetNamespaceKey(t)).(string)
			clientDeployment, svr, svc = generateTestingObjects(t, conf.CertID, namespace)

			// disable OSM
			svc.Annotations["kubernetes.azure.com/insecure-disable-osm"] = "true"
			svr.Spec.Template.Annotations["openservicemesh.io/sidecar-injection"] = "disabled"

			deployObjects(t, ctx, client, []k8s.Object{clientDeployment, svr, svc})
			return ctx
		}).
		Assess("client deployment available", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}

			// Wait for client deployment to be ready
			if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(clientDeployment, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(5*time.Minute)); err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Feature()

	testEnv.Test(t, noOSMFeature)
}

// TestPrometheus proves that users can consume Prometheus metrics emitted by our controllers
func TestPrometheus(t *testing.T) {
	t.Parallel()

	var promClient *appsv1.Deployment
	var namespace string

	prometheus := features.New("prometheus").
		Setup(func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}

			// Deploy Prometheus
			namespace = ctx.Value(e2eutil.GetNamespaceKey(t)).(string)
			promClient = fixtures.NewPrometheusClient(namespace, conf.PromClientImage)
			deployObjects(t, ctx, client, append(fixtures.NewPrometheus(namespace), promClient))

			return ctx
		}).
		Assess("prometheus metrics available", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}

			serverDep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: fixtures.PromServer, Namespace: namespace},
			}
			// Wait for prometheus server to be available
			if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(serverDep, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(5*time.Minute)); err != nil {
				t.Fatal(err)
			}

			// Wait for prometheus client to be available
			if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(promClient, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(5*time.Minute)); err != nil {
				t.Fatal(err)
			}
			return ctx
		}).
		Feature()

	testEnv.Test(t, prometheus)

}

func generateTestingObjects(t *testing.T, namespace, keyvaultURI string, config zoneConfig) (clientDeployment *appsv1.Deployment, serverDeployment *appsv1.Deployment, service *corev1.Service) {
	hostname := e2eutil.GetHostname(namespace, config.DNSZoneId)
	clientDeployment = fixtures.NewClientDeployment(t, hostname, config.NameServer, namespace, config.Id)
	serverDeployment = fixtures.NewGoDeployment(t, fixtures.Server, namespace, config.Id)
	service = fixtures.NewService(fixtures.Server.String()+config.Id, hostname, keyvaultURI, 8080, namespace)

	return clientDeployment, serverDeployment, service
}

func deployObjects(t *testing.T, ctx context.Context, client klient.Client, objs []k8s.Object) {
	for _, obj := range objs {
		if err := client.Resources().Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
	}
}

func generateZoneConfigs(conf *testConfig) []*zoneConfig {
	ret := []*zoneConfig{}

	// generate private zone config
	for i, privateZoneId := range conf.PrivateDNSZoneIDs {
		ret = append(ret, &zoneConfig{
			DNSZoneId:         privateZoneId,
			ZoneType:          "private",
			NameServer:        conf.PrivateNameserver,
			CertID:            conf.PrivateCertIDs[privateZoneId],
			CertVersionlessID: conf.PrivateCertVersionlessIDs[privateZoneId],
			Id:                fmt.Sprintf("-private-%d", i),
		})
	}

	// generate public zone config
	for i, publicZoneId := range conf.PublicDNSZoneIDs {
		ret = append(ret, &zoneConfig{
			DNSZoneId:         publicZoneId,
			ZoneType:          "public",
			NameServer:        conf.PublicNameservers[publicZoneId][0],
			CertID:            conf.PublicCertIDs[publicZoneId],
			CertVersionlessID: conf.PublicCertVersionlessIDs[publicZoneId],
			Id:                fmt.Sprintf("-public-%d", i),
		})
	}

	return ret
}
