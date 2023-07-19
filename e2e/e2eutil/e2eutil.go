// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package e2eutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/e2e/fixtures"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/go-autorest/autorest/azure"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/klient/k8s"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

const (
	operatorDeploymentKey = "operator-deployment"
	externalDnsNs         = config.DefaultNs
)

var (
	externalDnsResources = []k8s.Object{
		&corev1.ServiceAccount{},
		&rbacv1.ClusterRole{},
		&rbacv1.ClusterRoleBinding{},
		&appsv1.Deployment{},
	}
)

// Purge cleans up resources created by the previous run.
var Purge = func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	client, err := cfg.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create client during setup/purge: %s", err)
	}
	var list corev1.NamespaceList
	err = client.Resources().List(ctx, &list, func(opts *metav1.ListOptions) {
		opts.LabelSelector = "app.kubernetes.io/managed-by=e2eutil"
	})

	if err != nil {
		fmt.Printf("error while listing namespaces to purge past test runs: %s\n", err)
		return ctx, err
	}
	for _, item := range list.Items {
		err = client.Resources().Delete(ctx, &item)
		if err != nil {
			fmt.Printf("error while cleaning up namespace %q: %s\n", item.Name, err)
			return ctx, err
		}
	}

	// cleanup Prometheus cluster-level resources
	promClusterRoleBinding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{
		Name: fixtures.PromServer,
	}}
	if err := client.Resources().Delete(ctx, promClusterRoleBinding); err != nil && !k8serrors.IsNotFound(err) {
		return ctx, err
	}

	// cleanup Prometheus cluster-level resources
	promClusterRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{
		Name: fixtures.PromServer,
	}}
	if err := client.Resources().Delete(ctx, promClusterRole); err != nil && !k8serrors.IsNotFound(err) {
		return ctx, err
	}

	return ctx, nil
}

func GetHostname(ns, dnsZoneId string) string {
	parsedId, err := azure.ParseResourceID(dnsZoneId)

	// this means that our dns zone is bad
	if err != nil {
		panic(fmt.Errorf("failed to parse dns zone id: %s", err))
	}
	return strings.ToLower(ns) + "." + parsedId.ResourceName
}

// CreateNSForTest creates a random namespace with the runID as a prefix. It is stored in the context
// so that the deleteNSForTest routine can look it up and delete it.
func CreateNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
	prefix := "e2e-" + strings.ToLower(t.Name()) + "-"

	nsObj := &corev1.Namespace{}
	nsObj.Labels = map[string]string{"app.kubernetes.io/managed-by": "e2eutil", "openservicemesh.io/monitored-by": "osm"}
	nsObj.Annotations = map[string]string{"openservicemesh.io/sidecar-injection": "enabled"}
	nsObj.GenerateName = prefix

	if err := cfg.Client().Resources().Create(ctx, nsObj); err != nil {
		return ctx, err
	}
	ctx = context.WithValue(ctx, GetNamespaceKey(t), nsObj.Name)
	t.Logf("Created NS %s for test %s", nsObj.Name, t.Name())

	return ctx, nil
}

// DeleteNSForTest looks up the namespace corresponding to the given test and deletes it.
func DeleteNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T) (context.Context, error) {
	ns := fmt.Sprint(ctx.Value(GetNamespaceKey(t)))
	t.Logf("Deleting NS %v for test %v", ns, t.Name())

	nsObj := &corev1.Namespace{}
	nsObj.Name = ns
	return ctx, cfg.Client().Resources().Delete(ctx, nsObj)
}

// GetNamespaceKey returns the context key for a given test
func GetNamespaceKey(t *testing.T) string {
	// When we pass t.Name() from inside an `assess` step, the name is in the form TestName/Features/Assess
	t.Logf("Getting key from test name %s", t.Name())
	if strings.Contains(t.Name(), "/") {
		return strings.Split(t.Name(), "/")[0]
	}

	// When pass t.Name() from inside a `testenv.BeforeEachTest` function, the name is just TestName
	return t.Name()
}

// UpdateContainers updates the containers in the deployment
func UpdateContainers(ctx context.Context, client klient.Client, deployment *appsv1.Deployment, containers []corev1.Container) error {
	deployment = deployment.DeepCopy()
	mergePatch, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": containers,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshalling merge patch: %w", err)
	}

	if err := client.Resources().Patch(ctx, deployment, k8s.Patch{
		PatchType: types.StrategicMergePatchType,
		Data:      mergePatch,
	}); err != nil {
		return fmt.Errorf("patching deployment: %w", err)
	}

	return nil
}

// UpdateContainerDnsZoneIds updates the dns zone ids for the operator container in the given deployment and returns the new containers.
// This does not update the deployment.
func UpdateContainerDnsZoneIds(deployment *appsv1.Deployment, dnsIds []string) ([]corev1.Container, error) {
	containers := deployment.DeepCopy().Spec.Template.Spec.Containers
	for _, container := range containers {
		// this must match the operator name in devenv/kustomize/operator-deployment/operator.yaml
		if container.Name == "operator" {
			var argI int
			found := false
			for i, arg := range container.Args {
				if arg == "--dns-zone-ids" {
					argI = i
					found = true
					break
				}
			}

			if !found {
				return nil, errors.New("finding --dns-zone-ids argument in operator deployment")
			}

			container.Args[argI+1] = strings.Join(dnsIds, ",")
		}
	}

	return containers, nil
}

func GetDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	deployment, ok := ctx.Value(operatorDeploymentKey).(*appsv1.Deployment)
	if !ok {
		return nil, errors.New("deployment not found in context")
	}

	return deployment, nil
}

func SetDeployment(ctx context.Context, deployment *appsv1.Deployment) context.Context {
	return context.WithValue(ctx, operatorDeploymentKey, deployment)
}

func EnsureExternalDns(ctx context.Context, client klient.Client, name string) error {
	ns := externalDnsNs
	for _, r := range externalDnsResources {
		r = r.DeepCopyObject().(k8s.Object)
		if err := client.Resources().Get(ctx, name, ns, r); err != nil {
			return fmt.Errorf("getting %s/%s of type %s: %w", ns, name, r.GetObjectKind().GroupVersionKind(), err)
		}

		if r.GetDeletionTimestamp() != nil {
			return fmt.Errorf("resource %s/%s of type %s is being deleted", ns, name, r.GetObjectKind().GroupVersionKind())
		}
	}

	return nil
}

func EnsureExternalDnsCleaned(ctx context.Context, client klient.Client, name string) error {
	ns := externalDnsNs
	for _, r := range externalDnsResources {
		r = r.DeepCopyObject().(k8s.Object)
		err := client.Resources().Get(ctx, name, ns, r)

		if err != nil && k8serrors.IsNotFound(err) {
			break
		}

		if err == nil && r.GetDeletionTimestamp() != nil {
			break
		}

		return fmt.Errorf("resource %s/%s of type %s still exists", ns, name, r.GetObjectKind().GroupVersionKind())
	}

	return nil
}

func RetryBackoff(tries int, initialSleep time.Duration, f func() error) error {
	var err error
	sleep := initialSleep
	for i := 0; i < tries; i++ {
		err = f()
		if err == nil {
			return nil
		}

		time.Sleep(sleep)
		sleep *= 2
	}

	return err
}
