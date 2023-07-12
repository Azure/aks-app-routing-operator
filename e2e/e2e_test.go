// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

//go:build e2e

package e2e

import (
	"bufio"
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
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/go-autorest/autorest/azure"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const (
	// must match value in /devenv/kustomize/operator-deployment/operator.yaml
	operatorName = "app-routing-operator"
	operatorNs   = "kube-system"
)

var (
	conf    = &testConfig{}
	testEnv = env.NewInClusterConfig()
)

type zoneConfig struct {
	ZoneType                  string
	NameServer                string
	CertID, CertVersionlessID string
	DNSZoneId                 string
	Id                        string
}

type testConfig struct {
	RandomPrefix      string
	PublicNameservers map[string][]string
	PrivateNameserver string

	PublicCertIDs, PublicCertVersionlessIDs   map[string]string
	PrivateCertIDs, PrivateCertVersionlessIDs map[string]string

	PrivateDNSZoneIDs, PublicDNSZoneIDs []string
	Zones                               []*zoneConfig
	PromClientImage                     string
}

func (testConfig *testConfig) Validate() error {
	if len(testConfig.PrivateDNSZoneIDs) == 0 {
		return errors.New("missing private dns zone ids")
	}

	if len(testConfig.PublicDNSZoneIDs) == 0 {
		return errors.New("missing public dns zone ids")
	}

	return nil
}

func init() {
	// Load configuration
	rawConf := os.Getenv("E2E_JSON_CONTENTS")
	if rawConf == "" {
		panic(errors.New("failed to get e2e contents from env"))
	}
	if err := json.Unmarshal([]byte(rawConf), conf); err != nil {
		panic(err)
	}

	// Load config
	conf.Zones = generateZoneConfigs(conf)
	promClientImage := strings.TrimSpace(os.Getenv("PROM_CLIENT_IMAGE"))
	if promClientImage == "" {
		panic(errors.New("failed to get prometheus client image from env"))
	}
	conf.PromClientImage = promClientImage

	util.UseServerSideApply()

	if err := conf.Validate(); err != nil {
		panic(fmt.Errorf("validating config: %w", err))
	}
}

func TestMain(m *testing.M) {
	testEnv.Setup(e2eutil.Purge)
	testEnv.BeforeEachTest(func(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
		return e2eutil.CreateNSForTest(ctx, cfg, t)
	})
	testEnv.AfterEachTest(func(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
		return e2eutil.DeleteNSForTest(ctx, cfg, t)
	})

	// Run tests
	os.Exit(testEnv.Run(m))
}

// TestOperatorLogging ensures that the operator logs are being emitted correctly.
func TestOperatorLogging(t *testing.T) {
	testEnv.Test(t, features.New("operator-logging").
		Assess("operator logs", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(err)
			}

			operator := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operatorName,
					Namespace: operatorNs,
				},
			}
			if err := wait.For(conditions.New(client.Resources()).ResourceMatch(operator, func(object k8s.Object) bool {
				d := object.(*appsv1.Deployment)
				return d.Status.ReadyReplicas > 0
			}), wait.WithTimeout(2*time.Minute)); err != nil {
				t.Fatal(err)
			}

			if err := client.Resources().Get(context.Background(), operatorName, operatorNs, operator); err != nil {
				t.Fatal(err)
			}
			selector := operator.Spec.Selector

			clientset, err := kubernetes.NewForConfig(client.RESTConfig())
			if err != nil {
				t.Fatal(err)
			}

			// need to use retry because pods may restart and need time to start again
			if err := e2eutil.RetryBackoff(3, time.Second*10, func() error {
				pods, err := clientset.CoreV1().Pods(operatorNs).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
				if err != nil {
					t.Fatal(err)
				}

				checkedLines := 0
				for _, pod := range pods.Items {
					if pod.Status.Phase != corev1.PodRunning {
						continue
					}

					req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{})
					logs, err := req.Stream(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					defer logs.Close()

					scanner := bufio.NewScanner(logs)
					for scanner.Scan() {
						if !json.Valid(scanner.Bytes()) {
							return errors.New("operator pod logs are not valid json: " + scanner.Text())
						}

						checkedLines++
					}
				}

				if checkedLines == 0 {
					return errors.New("no logs found")
				}

				return nil
			}); err != nil {
				t.Fatal(err)
			}

			return ctx
		}).Feature(),
	)
}

// TestBasicService is the most common user scenario - add annotations to a service, get back working
// ingress with TLS termination and e2e encryption using OSM.
func TestBasicService(t *testing.T) {
	t.Parallel()

	genBasicFeature := func(zoneconfig *zoneConfig) features.Feature {
		var clientDeployment, serverDeployment *appsv1.Deployment
		var service *corev1.Service

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
				if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(clientDeployment, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(8*time.Minute)); err != nil {
					t.Logf("failed to wait for client deployment %s to be ready: %s", clientDeployment.Name, err)
					t.Fatal(err)
				}

				return ctx
			}).Feature()
	}

	for _, zone := range conf.Zones {
		testEnv.Test(t, genBasicFeature(zone))
	}
}

// TestBasicServiceVersionlessCert proves that users can remove the version hash from a Keyvault cert URI.
func TestBasicServiceVersionlessCert(t *testing.T) {
	t.Parallel()

	genVersionlessFeature := func(zoneConfig *zoneConfig) features.Feature {
		var clientDeployment, serverDeployment *appsv1.Deployment
		var service *corev1.Service

		return features.New("versionlessCert").
			Setup(func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
				client, err := config.NewClient()
				if err != nil {
					t.Fatal(err)
				}
				namespace := ctx.Value(e2eutil.GetNamespaceKey(t)).(string)

				clientDeployment, serverDeployment, service = generateTestingObjects(t, namespace, zoneConfig.CertVersionlessID, zoneConfig)
				deployObjects(t, ctx, client, []k8s.Object{clientDeployment, serverDeployment, service})
				return ctx
			}).
			Assess("client deployment available", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
				client, err := config.NewClient()
				if err != nil {
					t.Fatal(err)
				}

				// Wait for client deployment to be ready
				if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(clientDeployment, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(8*time.Minute)); err != nil {
					t.Logf("failed to wait for client deployment %s to be ready: %s", clientDeployment.Name, err)
					t.Fatal(err)
				}

				return ctx
			}).Feature()
	}

	for _, zone := range conf.Zones {
		testEnv.Test(t, genVersionlessFeature(zone))
	}
}

// TestBasicServiceNoOSM is identical to TestBasicService but disables OSM.
func TestBasicServiceNoOSM(t *testing.T) {
	t.Parallel()

	genNoOSMFeature := func(zoneConfig *zoneConfig) features.Feature {
		var clientDeployment, svr *appsv1.Deployment
		var svc *corev1.Service

		return features.New("noOSM").
			Setup(func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
				client, err := config.NewClient()
				if err != nil {
					t.Fatal(err)
				}
				namespace := ctx.Value(e2eutil.GetNamespaceKey(t)).(string)
				clientDeployment, svr, svc = generateTestingObjects(t, namespace, zoneConfig.CertID, zoneConfig)

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
				if err := wait.For(conditions.New(client.Resources()).DeploymentConditionMatch(clientDeployment, appsv1.DeploymentAvailable, corev1.ConditionTrue), wait.WithTimeout(8*time.Minute)); err != nil {
					t.Logf("failed to wait for client deployment %s to be ready: %s", clientDeployment.Name, err)
					t.Fatal(err)
				}

				return ctx
			}).
			Feature()
	}

	for _, zone := range conf.Zones {
		testEnv.Test(t, genNoOSMFeature(zone))
	}

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

func TestCleanup(t *testing.T) {
	// this cannot be a parallel test because it manipulates the operator deployment which affects other tests
	testEnv.Test(t, features.New("cleanup").
		WithSetup("get initial deployent", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			// get initial operator deployment so we can restore it later
			client, err := config.NewClient()
			if err != nil {
				t.Fatal(fmt.Errorf("creating client: %w", err))
			}

			deployment := &appsv1.Deployment{}
			if err := client.Resources().Get(ctx, operatorName, operatorNs, deployment); err != nil {
				t.Fatal(fmt.Errorf("getting operator deployment: %w", err))
			}

			return e2eutil.SetDeployment(ctx, deployment)
		}).
		Assess("all dns deployed", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			deployment, err := e2eutil.GetDeployment(ctx)
			if err != nil {
				t.Fatal(err)
			}

			client, err := config.NewClient()
			if err != nil {
				t.Fatal(fmt.Errorf("creating client: %w", err))
			}

			containers, err := e2eutil.UpdateContainerDnsZoneIds(deployment, append(conf.PrivateDNSZoneIDs, conf.PublicDNSZoneIDs...))
			if err != nil {
				t.Fatal(fmt.Errorf("updating container dns zone ids: %w", err))
			}

			if err := e2eutil.UpdateContainers(ctx, client, deployment, containers); err != nil {
				t.Fatal(fmt.Errorf("updating containers: %w", err))
			}

			// this uses retry backoff because it takes time for operator to clean and create resources
			if err := e2eutil.RetryBackoff(3, time.Second*10, func() error {
				if err := e2eutil.EnsureExternalDns(ctx, client, manifests.PublicProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring public external dns: %w", err)
				}

				if err := e2eutil.EnsureExternalDns(ctx, client, manifests.PrivateProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring private external dns: %w", err)
				}

				return nil
			}); err != nil {
				t.Fatal(fmt.Errorf("ensuring external dns exists: %w", err))
			}

			return ctx
		}).
		Assess("private dns only", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			deployment, err := e2eutil.GetDeployment(ctx)
			if err != nil {
				t.Fatal(err)
			}

			client, err := config.NewClient()
			if err != nil {
				t.Fatal(fmt.Errorf("creating client: %w", err))
			}

			containers, err := e2eutil.UpdateContainerDnsZoneIds(deployment, conf.PrivateDNSZoneIDs)
			if err != nil {
				t.Fatal(fmt.Errorf("updating container dns zone ids: %w", err))
			}

			if err := e2eutil.UpdateContainers(ctx, client, deployment, containers); err != nil {
				t.Fatal(fmt.Errorf("updating containers: %w", err))
			}

			// this uses retry backoff because it takes time for operator to clean and create resources
			if err := e2eutil.RetryBackoff(3, time.Second*10, func() error {
				if err := e2eutil.EnsureExternalDnsCleaned(ctx, client, manifests.PublicProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring public external dns: %w", err)
				}

				if err := e2eutil.EnsureExternalDns(ctx, client, manifests.PrivateProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring private external dns: %w", err)
				}

				return nil
			}); err != nil {
				t.Fatal(fmt.Errorf("ensuring external dns exists: %w", err))
			}

			return ctx
		}).
		Assess("public dns only", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			deployment, err := e2eutil.GetDeployment(ctx)
			if err != nil {
				t.Fatal(err)
			}

			client, err := config.NewClient()
			if err != nil {
				t.Fatal(fmt.Errorf("creating client: %w", err))
			}

			containers, err := e2eutil.UpdateContainerDnsZoneIds(deployment, conf.PublicDNSZoneIDs)
			if err != nil {
				t.Fatal(fmt.Errorf("updating container dns zone ids: %w", err))
			}

			if err := e2eutil.UpdateContainers(ctx, client, deployment, containers); err != nil {
				t.Fatal(fmt.Errorf("updating containers: %w", err))
			}

			// this uses retry backoff because it takes time for operator to clean and create resources
			if err := e2eutil.RetryBackoff(3, time.Second*10, func() error {
				if err := e2eutil.EnsureExternalDnsCleaned(ctx, client, manifests.PrivateProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring public external dns: %w", err)
				}

				if err := e2eutil.EnsureExternalDns(ctx, client, manifests.PublicProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring private external dns: %w", err)
				}

				return nil
			}); err != nil {
				t.Fatal(fmt.Errorf("ensuring external dns exists: %w", err))
			}

			return ctx
		}).
		Assess("no dns", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			deployment, err := e2eutil.GetDeployment(ctx)
			if err != nil {
				t.Fatal(err)
			}

			client, err := config.NewClient()
			if err != nil {
				t.Fatal(fmt.Errorf("creating client: %w", err))
			}

			containers, err := e2eutil.UpdateContainerDnsZoneIds(deployment, []string{})
			if err != nil {
				t.Fatal(fmt.Errorf("updating container dns zone ids: %w", err))
			}

			if err := e2eutil.UpdateContainers(ctx, client, deployment, containers); err != nil {
				t.Fatal(fmt.Errorf("updating containers: %w", err))
			}

			// this uses retry backoff because it takes time for operator to clean and create resources
			if err := e2eutil.RetryBackoff(3, time.Second*10, func() error {
				if err := e2eutil.EnsureExternalDnsCleaned(ctx, client, manifests.PublicProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring public external dns: %w", err)
				}

				if err := e2eutil.EnsureExternalDnsCleaned(ctx, client, manifests.PrivateProvider.ResourceName()); err != nil {
					return fmt.Errorf("ensuring private external dns: %w", err)
				}

				return nil
			}); err != nil {
				t.Fatal(fmt.Errorf("ensuring external dns exists: %w", err))
			}

			return ctx

			return ctx
		}).
		WithTeardown("restore deployment", func(ctx context.Context, t *testing.T, config *envconf.Config) context.Context {
			deployment, err := e2eutil.GetDeployment(ctx)
			if err != nil {
				t.Fatal(err)
			}

			client, err := config.NewClient()
			if err != nil {
				t.Fatal(fmt.Errorf("creating client: %w", err))
			}

			if err := e2eutil.UpdateContainers(ctx, client, deployment, deployment.Spec.Template.Spec.Containers); err != nil {
				t.Fatal(fmt.Errorf("updating containers: %w", err))
			}

			return ctx
		}).Feature(),
	)
}

func generateTestingObjects(t *testing.T, namespace, keyvaultURI string, config *zoneConfig) (clientDeployment *appsv1.Deployment, serverDeployment *appsv1.Deployment, service *corev1.Service) {
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
	var ret []*zoneConfig

	// generate private zone configs
	for i, privateZoneId := range conf.PrivateDNSZoneIDs {
		parsedId, err := azure.ParseResourceID(privateZoneId)
		if err != nil {
			panic(fmt.Errorf("failed to parse private zone id: %s", err.Error()))
		}
		withoutRandom := strings.Replace(parsedId.ResourceName, fmt.Sprintf("%s-", conf.RandomPrefix), "", 1)
		certId := conf.PrivateCertIDs[withoutRandom]
		versionlessCertId := conf.PrivateCertVersionlessIDs[withoutRandom]

		ret = append(ret, &zoneConfig{
			DNSZoneId:         privateZoneId,
			ZoneType:          "private",
			NameServer:        conf.PrivateNameserver,
			CertID:            certId,
			CertVersionlessID: versionlessCertId,
			Id:                fmt.Sprintf("-private-%d", i),
		})
	}

	// generate public zone configs
	for i, publicZoneId := range conf.PublicDNSZoneIDs {
		parsedId, err := azure.ParseResourceID(publicZoneId)
		if err != nil {
			panic(fmt.Errorf("failed to parse private zone id: %s", err.Error()))
		}
		withoutRandom := strings.Replace(parsedId.ResourceName, fmt.Sprintf("%s-", conf.RandomPrefix), "", 1)

		publicNameserver := conf.PublicNameservers[withoutRandom][0]
		certId := conf.PublicCertIDs[withoutRandom]
		certVersionlessId := conf.PublicCertIDs[withoutRandom]

		ret = append(ret, &zoneConfig{
			DNSZoneId:         publicZoneId,
			ZoneType:          "public",
			NameServer:        publicNameserver,
			CertID:            certId,
			CertVersionlessID: certVersionlessId,
			Id:                fmt.Sprintf("-public-%d", i),
		})
	}

	return ret
}
