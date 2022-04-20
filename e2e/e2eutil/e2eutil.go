// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package e2eutil

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	rbacv1 "k8s.io/client-go/applyconfigurations/rbac/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

type Suite struct {
	Client    client.Client
	Clientset kubernetes.Interface
}

func (s *Suite) StartTestCase(t *testing.T) *Case {
	return &Case{Suite: s, t: t}
}

// Purge cleans up resources created by the previous run.
func (s *Suite) Purge() {
	list, err := s.Clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=e2eutil",
	})
	if err != nil {
		fmt.Printf("error while listing namespaces to purge past test runs: %s\n", err)
		return
	}
	for _, item := range list.Items {
		fmt.Printf("cleaning up namespace from previous run %q\n", item.Name)
		err = s.Clientset.CoreV1().Namespaces().Delete(context.Background(), item.Name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Printf("error while cleaning up namespace %q: %s\n", item.Name, err)
		}
	}
}

type Case struct {
	*Suite
	t  *testing.T
	ns string
}

func (c *Case) Retry(fn func() error) {
	c.t.Helper()
	for {
		err := fn()
		if err == nil {
			return
		}
		c.t.Logf("error: %s", err)
		time.Sleep(util.Jitter(time.Millisecond*200, 0.5))
	}
}

// WithResources creates Kubernetes resources for the test case and waits for them to become ready.
func (c *Case) WithResources(resources ...client.Object) {
	c.ensureNS()
	var wg sync.WaitGroup
	for _, res := range resources {
		res.SetNamespace(c.ns)
		c.Retry(func() error {
			return c.Client.Create(context.Background(), res)
		})

		wg.Add(1)
		go func(res interface{}) {
			defer wg.Done()
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

func (c *Case) ensureNS() {
	if c.ns != "" {
		return
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

	// Log events in the namespace
	go func() {
		c.Retry(func() error {
			watch, err := c.Clientset.CoreV1().Events(c.ns).Watch(context.Background(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			c.t.Cleanup(watch.Stop)
			for msg := range watch.ResultChan() {
				event, ok := msg.Object.(*corev1.Event)
				if !ok {
					return fmt.Errorf("unknown event type: %T", msg.Object)
				}
				log.Printf("k8s event: (%s %s) %s %s - %s", event.InvolvedObject.Kind, event.InvolvedObject.Name, event.Kind, event.Reason, event.Message)

				// Print pod logs if they crash or fail readiness probes
				probeFailed := strings.Contains(event.Message, "Readiness probe failed")
				containerCrashed := event.Message == "Back-off restarting failed container"
				if !containerCrashed && !probeFailed {
					continue
				}
				logs, err := c.Clientset.CoreV1().Pods(c.ns).
					GetLogs(event.InvolvedObject.Name, &corev1.PodLogOptions{Previous: containerCrashed, Container: "container", TailLines: util.Int64Ptr(3)}).
					DoRaw(context.Background())
				if err != nil {
					log.Printf("error while getting pod logs: %s", err)
					continue
				}
				log.Printf("log from pod %s:\n%s", event.InvolvedObject.Name, logs)
			}
			return nil
		})
	}()
}

// SetupRBAC creates and binds a ClusterRole manifest to a given username. Intended to be used
// by e2e tests that need to limit their own k8s access.
func SetupRBAC(cs kubernetes.Interface, roleManifestPath, userName string) error {
	role := &rbacv1.ClusterRoleApplyConfiguration{}
	rawRole, err := os.ReadFile(roleManifestPath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(rawRole, role); err != nil {
		return err
	}
	_, err = cs.RbacV1().ClusterRoles().Apply(context.Background(), role, metav1.ApplyOptions{FieldManager: "e2e"})
	if err != nil {
		return err
	}

	roleBinding := &rbacv1.ClusterRoleBindingApplyConfiguration{
		ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
			Name: role.Name,
		},
		Subjects: []rbacv1.SubjectApplyConfiguration{{
			Kind: util.StringPtr("User"),
			Name: util.StringPtr(userName),
		}},
		RoleRef: &rbacv1.RoleRefApplyConfiguration{
			Kind:     util.StringPtr("ClusterRole"),
			Name:     role.Name,
			APIGroup: util.StringPtr("rbac.authorization.k8s.io"),
		},
	}
	roleBinding.Kind = util.StringPtr("ClusterRoleBinding")
	roleBinding.APIVersion = util.StringPtr("rbac.authorization.k8s.io/v1")
	_, err = cs.RbacV1().ClusterRoleBindings().Apply(context.Background(), roleBinding, metav1.ApplyOptions{FieldManager: "e2e"})
	return err
}
