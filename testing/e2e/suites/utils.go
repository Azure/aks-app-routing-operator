package suites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	unavailableDeploymentLogTailLines  int64 = 100
	unavailableDeploymentLogLimitBytes int64 = 16 * 1024
)

func waitForAvailable(ctx context.Context, config *rest.Config, c client.Client, deployment appsv1.Deployment) error {
	lgr := logger.FromContext(ctx).With("deployment", deployment.Name, "namespace", deployment.Namespace)
	lgr.Info("waiting for deployment to be available")
	start := time.Now()
	for {
		d := &deployment
		if err := c.Get(ctx, client.ObjectKeyFromObject(d), d); err != nil {
			return fmt.Errorf("getting deployment: %w", err)
		}

		for _, condition := range d.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == "True" {
				lgr.Info("deployment is available")
				return nil
			}
		}

		// 20 minutes because it takes a decent amount of time for dns to "propagate", and up to 30 min for Azure RBAC to propagate for ExternalDNS to read the DNS zones
		if time.Since(start) > 20*time.Minute {
			summary, err := unavailableDeploymentSummary(ctx, config, c, d)
			if err != nil {
				return fmt.Errorf("timed out waiting for deployment to be available; additionally failed to collect diagnostics: %w", err)
			}
			return fmt.Errorf("timed out waiting for deployment to be available:\n%s", summary)
		}

		lgr.Info("deployment is not available yet, waiting 5 seconds for retry")
		time.Sleep(5 * time.Second)
	}
}

func unavailableDeploymentSummary(ctx context.Context, config *rest.Config, c client.Client, deployment *appsv1.Deployment) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "deployment %s/%s replicas=%d updated=%d ready=%d available=%d observedGeneration=%d generation=%d\n",
		deployment.Namespace,
		deployment.Name,
		deployment.Status.Replicas,
		deployment.Status.UpdatedReplicas,
		deployment.Status.ReadyReplicas,
		deployment.Status.AvailableReplicas,
		deployment.Status.ObservedGeneration,
		deployment.Generation,
	)
	for _, condition := range deployment.Status.Conditions {
		fmt.Fprintf(&b, "deployment condition type=%s status=%s reason=%s message=%q\n", condition.Type, condition.Status, condition.Reason, condition.Message)
	}

	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("building deployment selector: %w", err)
	}

	podList := &corev1.PodList{}
	if err := c.List(ctx, podList, client.InNamespace(deployment.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return "", fmt.Errorf("listing pods for deployment: %w", err)
	}

	podNames := make(map[string]struct{}, len(podList.Items))
	for _, pod := range podList.Items {
		podNames[pod.Name] = struct{}{}
		fmt.Fprintf(&b, "pod %s phase=%s reason=%s message=%q podIP=%s\n", pod.Name, pod.Status.Phase, pod.Status.Reason, pod.Status.Message, pod.Status.PodIP)
		for _, condition := range pod.Status.Conditions {
			fmt.Fprintf(&b, "  pod condition type=%s status=%s reason=%s message=%q\n", condition.Type, condition.Status, condition.Reason, condition.Message)
		}
		for _, container := range pod.Status.ContainerStatuses {
			fmt.Fprintf(&b, "  container %s ready=%t restartCount=%d image=%s\n", container.Name, container.Ready, container.RestartCount, container.Image)
			if container.State.Waiting != nil {
				fmt.Fprintf(&b, "    waiting reason=%s message=%q\n", container.State.Waiting.Reason, container.State.Waiting.Message)
			}
			if container.State.Terminated != nil {
				fmt.Fprintf(&b, "    terminated reason=%s exitCode=%d message=%q\n", container.State.Terminated.Reason, container.State.Terminated.ExitCode, container.State.Terminated.Message)
			}
			if container.LastTerminationState.Terminated != nil {
				fmt.Fprintf(&b, "    last terminated reason=%s exitCode=%d message=%q\n", container.LastTerminationState.Terminated.Reason, container.LastTerminationState.Terminated.ExitCode, container.LastTerminationState.Terminated.Message)
			}
		}
	}

	eventList := &corev1.EventList{}
	if err := c.List(ctx, eventList, client.InNamespace(deployment.Namespace)); err != nil {
		return "", fmt.Errorf("listing namespace events: %w", err)
	}

	writtenEvents := 0
	for _, event := range eventList.Items {
		if event.InvolvedObject.Kind == "Deployment" && event.InvolvedObject.Name != deployment.Name {
			continue
		}
		if event.InvolvedObject.Kind == "Pod" {
			if _, ok := podNames[event.InvolvedObject.Name]; !ok {
				continue
			}
		}
		if event.InvolvedObject.Kind != "Deployment" && event.InvolvedObject.Kind != "Pod" {
			continue
		}
		fmt.Fprintf(&b, "event kind=%s name=%s type=%s reason=%s message=%q count=%d last=%s\n",
			event.InvolvedObject.Kind,
			event.InvolvedObject.Name,
			event.Type,
			event.Reason,
			event.Message,
			event.Count,
			event.LastTimestamp.String(),
		)
		writtenEvents++
		if writtenEvents >= 20 {
			break
		}
	}

	appendPodLogs(ctx, config, &b, deployment.Namespace, podList.Items)

	return b.String(), nil
}

func appendPodLogs(ctx context.Context, config *rest.Config, b *strings.Builder, namespace string, pods []corev1.Pod) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(b, "pod logs unavailable: creating clientset: %v\n", err)
		return
	}

	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			appendContainerLogs(ctx, clientset, b, namespace, pod.Name, container.Name, false)
		}
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.RestartCount > 0 {
				appendContainerLogs(ctx, clientset, b, namespace, pod.Name, containerStatus.Name, true)
			}
		}
	}
}

func appendContainerLogs(ctx context.Context, clientset kubernetes.Interface, b *strings.Builder, namespace, podName, containerName string, previous bool) {
	tailLines := unavailableDeploymentLogTailLines
	limitBytes := unavailableDeploymentLogLimitBytes
	opts := &corev1.PodLogOptions{
		Container:  containerName,
		Previous:   previous,
		TailLines:  &tailLines,
		LimitBytes: &limitBytes,
	}

	logs, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	logKind := "current"
	if previous {
		logKind = "previous"
	}
	if err != nil {
		fmt.Fprintf(b, "pod log %s/%s container=%s %s unavailable: %v\n", namespace, podName, containerName, logKind, err)
		return
	}
	defer logs.Close()

	contents, err := io.ReadAll(logs)
	if err != nil {
		fmt.Fprintf(b, "pod log %s/%s container=%s %s read failed: %v\n", namespace, podName, containerName, logKind, err)
		return
	}

	fmt.Fprintf(b, "pod log %s/%s container=%s %s tailLines=%d limitBytes=%d:\n%s\n", namespace, podName, containerName, logKind, unavailableDeploymentLogTailLines, unavailableDeploymentLogLimitBytes, strings.TrimSpace(string(contents)))
}

func upsert(ctx context.Context, c client.Client, obj client.Object) error {
	copy := obj.DeepCopyObject().(client.Object)
	lgr := logger.FromContext(ctx).With("object", copy.GetName(), "namespace", copy.GetNamespace())
	lgr.Info("upserting object")

	// create or update the object
	lgr.Info("attempting to create object")
	err := c.Create(ctx, copy)
	if err == nil {
		obj.SetName(copy.GetName()) // supports objects that want to use generate name
		lgr.Info("object created")
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating object: %w", err)
	}

	lgr.Info("object already exists, attempting to update")
	if err := c.Patch(ctx, copy, client.Apply, client.ForceOwnership, client.FieldOwner("e2e-test")); err != nil {
		return fmt.Errorf("updating object: %w", err)
	}

	lgr.Info("object updated")
	return nil
}

func waitForNICAvailable(ctx context.Context, c client.Client, nic *v1alpha1.NginxIngressController) (*v1alpha1.NginxIngressController, error) {
	lgr := logger.FromContext(ctx).With("nic", nic.Name)
	lgr.Info("waiting for NIC to be available")
	var new v1alpha1.NginxIngressController

	if err := wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		lgr.Info("checking if NIC is available")
		if err := c.Get(ctx, client.ObjectKeyFromObject(nic), &new); err != nil {
			return false, fmt.Errorf("get nic: %w", err)
		}

		for _, cond := range new.Status.Conditions {
			if cond.Type == v1alpha1.ConditionTypeAvailable {
				lgr.Info("found nic")
				if len(new.Status.ManagedResourceRefs) == 0 {
					lgr.Info("nic has no ManagedResourceRefs")
					return false, nil
				}
				return true, nil
			}
		}
		lgr.Info("nic not available")
		return false, nil
	}); err != nil {
		return nil, fmt.Errorf("waiting for NIC to be available: %w", err)
	}

	return &new, nil
}

func getNginxLbServiceRef(nic *v1alpha1.NginxIngressController) (v1alpha1.ManagedObjectReference, error) {
	for _, ref := range nic.Status.ManagedResourceRefs {
		// we are looking for the load balancer service, not metrics service
		if ref.Kind == "Service" && !strings.HasSuffix(ref.Name, "-metrics") {
			return ref, nil
		}
	}

	return v1alpha1.ManagedObjectReference{}, errors.New("no load balancer service available")
}

// waitForDefaultSA waits for the "default" ServiceAccount to be created in the given namespace.
// Kubernetes automatically creates this SA when a namespace is created, but there can be a brief
// delay. Without waiting, pod creation can fail with "serviceaccount default not found".
func waitForDefaultSA(ctx context.Context, c client.Client, namespace string) error {
	lgr := logger.FromContext(ctx).With("namespace", namespace)
	lgr.Info("waiting for default service account to be available")

	return wait.PollImmediate(500*time.Millisecond, 1*time.Minute, func() (bool, error) {
		sa := &corev1.ServiceAccount{}
		err := c.Get(ctx, types.NamespacedName{Name: "default", Namespace: namespace}, sa)
		if err == nil {
			lgr.Info("default service account is available")
			return true, nil
		}
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting default service account: %w", err)
	})
}
