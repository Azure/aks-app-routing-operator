// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package e2eutil

import (
	"context"
	"fmt"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	"log"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/e2e-framework/pkg/env"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

type Suite struct {
	Env env.Environment
}

func (s *Suite) StartTestCase(t *testing.T) *Case {
	return &Case{Suite: s, t: t}
}

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
		fmt.Printf("cleaning up namespace from previous run %q\n", item.Name)
		err = client.Resources().Delete(ctx, &item)
		if err != nil {
			fmt.Printf("error while cleaning up namespace %q: %s\n", item.Name, err)
			return ctx, err
		}
	}

	return ctx, nil
}

type Case struct {
	*Suite
	t  *testing.T
	ns string
}

func (c *Case) Retry(fn func() error) {
	for {
		err := fn()
		if err == nil {
			return
		}
		log.Printf("error: %s", err)
		time.Sleep(util.Jitter(time.Millisecond*200, 0.5))
	}
}

func (c *Case) Hostname(domain string) string {
	ns := c.NS()
	return strings.ToLower(ns) + "." + domain
}

// WithResources creates Kubernetes resources for the test case and waits for them to become ready.
func (c *Case) WithResources(resources ...client.Object) {
	c.ensureNS()

	var wg sync.WaitGroup
	for _, res := range resources {
		wg.Add(1)
		go func(res client.Object) {
			defer wg.Done()

			res.SetNamespace(c.ns)
			c.Retry(func() error {
				err := c.Client.Create(context.Background(), res)
				if err != nil && k8serrors.IsAlreadyExists(err) {
					err = c.Client.Update(context.Background(), res)
				}

				return err
			})

			switch obj := res.(type) {
			case *appsv1.Deployment:
				c.watchDeployment(obj)
			}
		}(res)
	}
	wg.Wait()
}

func (c *Case) watchDeployment(obj *appsv1.Deployment) {
	c.Retry(func() error {
		watch, err := c.Clientset.AppsV1().Deployments(c.ns).Watch(context.Background(), metav1.ListOptions{
			FieldSelector:   "metadata.name=" + obj.Name,
			ResourceVersion: obj.ResourceVersion,
		})
		if err != nil {
			return err
		}
		c.t.Cleanup(watch.Stop)
		for event := range watch.ResultChan() {
			item, ok := event.Object.(*appsv1.Deployment)
			if !ok {
				return fmt.Errorf("unknown event type: %T", event.Object)
			}
			if item.Status.ReadyReplicas == *item.Spec.Replicas {
				break
			}
		}
		return nil
	})
}

func (c *Case) ensureNS(env env.Environment) string {
	if c.ns != "" {
		return c.ns
	}

	c.Retry(func() error {
		ns := &corev1.Namespace{}
		ns.GenerateName = "e2e-" + strings.ToLower(c.t.Name()) + "-"
		ns.Labels = map[string]string{"app.kubernetes.io/managed-by": "e2eutil", "openservicemesh.io/monitored-by": "osm"}
		ns.Annotations = map[string]string{"openservicemesh.io/sidecar-injection": "enabled"}
		ns, err := c.Clientset.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
		c.ns = ns.Name
		return err
	})

	return c.ns

	//// Log events in the namespace
	//go func() {
	//	c.Retry(func() error {
	//		watch, err := c.Clientset.CoreV1().Events(c.ns).Watch(context.Background(), metav1.ListOptions{})
	//		if err != nil {
	//			return err
	//		}
	//		c.t.Cleanup(watch.Stop)
	//		for msg := range watch.ResultChan() {
	//			event, ok := msg.Object.(*corev1.Event)
	//			if !ok {
	//				return fmt.Errorf("unknown event type: %T", msg.Object)
	//			}
	//			log.Printf("k8s event: (%s %s) %s %s - %s", event.InvolvedObject.Kind, event.InvolvedObject.Name, event.Kind, event.Reason, event.Message)
	//
	//			// Print pod logs if they crash or fail readiness probes
	//			probeFailed := strings.Contains(event.Message, "Readiness probe failed")
	//			containerCrashed := event.Message == "Back-off restarting failed container"
	//			if !containerCrashed && !probeFailed {
	//				continue
	//			}
	//			logs, err := c.Clientset.CoreV1().Pods(c.ns).
	//				GetLogs(event.InvolvedObject.Name, &corev1.PodLogOptions{Previous: containerCrashed, Container: "container", TailLines: util.Int64Ptr(3)}).
	//				DoRaw(context.Background())
	//			if err != nil {
	//				log.Printf("error while getting pod logs: %s", err)
	//				continue
	//			}
	//			log.Printf("log from pod %s:\n%s", event.InvolvedObject.Name, logs)
	//		}
	//		return nil
	//	})
	//}()
}

func (c *Case) NS(env env.Environment) string {
	if c.ns == "" {
		return c.ensureNS(env)
	}
	return c.ns
}

// CreateNSForTest creates a random namespace with the runID as a prefix. It is stored in the context
// so that the deleteNSForTest routine can look it up and delete it.
func CreateNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T, runID string) (context.Context, error) {
	ns := envconf.RandomName(runID, 10)
	ctx = context.WithValue(ctx, GetNamespaceKey(t), ns)

	t.Logf("Creating NS %s for test %s", ns, t.Name())
	nsObj := &corev1.Namespace{}
	nsObj.Name = ns
	return ctx, cfg.Client().Resources().Create(ctx, nsObj)
}

// DeleteNSForTest looks up the namespace corresponding to the given test and deletes it.
func DeleteNSForTest(ctx context.Context, cfg *envconf.Config, t *testing.T, runID string) (context.Context, error) {
	ns := fmt.Sprint(ctx.Value(GetNamespaceKey(t)))
	t.Logf("Deleting NS %v for test %v", ns, t.Name())

	nsObj := &corev1.Namespace{}
	nsObj.GenerateName = "e2e-" + strings.ToLower(c.t.Name()) + "-" = ns
	return ctx, cfg.Client().Resources().Delete(ctx, nsObj)
}

// GetNamespaceKey returns the context key for a given test
func GetNamespaceKey(t *testing.T) NamespaceCtxKey {
	// When we pass t.Name() from inside an `assess` step, the name is in the form TestName/Features/Assess
	if strings.Contains(t.Name(), "/") {
		return NamespaceCtxKey(strings.Split(t.Name(), "/")[0])
	}

	// When pass t.Name() from inside a `testenv.BeforeEachTest` function, the name is just TestName
	return NamespaceCtxKey(t.Name())
}