// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"bytes"
	"container/ring"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	approutingv1alpha1 "github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/nginxingress"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	prommodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/controllername"
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

var concurrencyWatchdogControllerName = controllername.New("concurrency", "watchdog")

// ScrapeFn returns the connection count for the given pod
type ScrapeFn func(ctx context.Context, client rest.Interface, pod *corev1.Pod) (float64, error)

// NginxScrapeFn is the scrape function for Nginx
func NginxScrapeFn(ctx context.Context, client rest.Interface, pod *corev1.Pod) (float64, error) {
	lgr := logr.FromContextOrDiscard(ctx)

	lgr.Info("scraping pod", "pod", pod.Name)
	resp, err := client.Get().
		AbsPath("/api/v1/namespaces", pod.Namespace, "pods", pod.Name+":10254", "proxy/metrics").
		Timeout(time.Second * 30).
		MaxRetries(4).
		DoRaw(ctx)
	if err != nil {
		return 0, err
	}

	family := &prommodel.MetricFamily{}
	format, err := expfmt.NewOpenMetricsFormat(expfmt.OpenMetricsVersion_0_0_1)
	if err != nil {
		return 0, fmt.Errorf("creating open metrics format: %w", err)
	}

	dec := expfmt.NewDecoder(bytes.NewReader(resp), format)
	for {
		err = dec.Decode(family)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
		if family.GetName() != "nginx_ingress_controller_nginx_process_connections" {
			continue
		}
		for _, metric := range family.Metric {
			if metric.Gauge == nil || !metricHasLabel(metric, "state", "active") {
				continue
			}
			return metric.Gauge.GetValue(), nil
		}
	}

	return 0, fmt.Errorf("active connections metric not found")
}

// WatchdogTarget refers to a target the concurrency watchdog should track
type WatchdogTarget struct {
	ScrapeFn  ScrapeFn
	PodLabels map[string]string
}

type ListWatchdogTargets func() ([]WatchdogTarget, error)

func GetListNginxWatchdogTargets(cl client.Client, defaultNicControllerClass string) ListWatchdogTargets {
	return func() ([]WatchdogTarget, error) {
		nics := &approutingv1alpha1.NginxIngressControllerList{}
		if err := cl.List(nil, nics); err != nil {
			return nil, fmt.Errorf("listing NginxIngressController objects: %w", err)
		}

		var targets []WatchdogTarget
		for _, nic := range nics.Items {
			ingCfg := nginxingress.ToNginxIngressConfig(&nic, defaultNicControllerClass)
			if ingCfg == nil {
				continue
			}

			targets = append(targets, WatchdogTarget{
				ScrapeFn:  NginxScrapeFn,
				PodLabels: ingCfg.PodLabels(),
			})
		}

		return targets, nil
	}
}

// ConcurrencyWatchdog evicts ingress controller pods that have too many active connections relative to others.
// This helps redistribute long-running connections when the ingress controller scales up.
type ConcurrencyWatchdog struct {
	client              client.Client
	clientset           kubernetes.Interface
	restClient          rest.Interface
	logger              logr.Logger
	config              *config.Config
	listWatchdogTargets ListWatchdogTargets

	interval, minPodAge, voteTTL time.Duration
	minVotesBeforeEviction       int
	minPercentOverAvgBeforeVote  float64

	votes *ring.Ring
}

func NewConcurrencyWatchdog(manager ctrl.Manager, conf *config.Config, target ListWatchdogTargets) error {
	metrics.InitControllerMetrics(concurrencyWatchdogControllerName)
	clientset, err := kubernetes.NewForConfig(manager.GetConfig())
	if err != nil {
		return err
	}

	c := &ConcurrencyWatchdog{
		client:              manager.GetClient(),
		clientset:           clientset,
		restClient:          clientset.CoreV1().RESTClient(),
		logger:              concurrencyWatchdogControllerName.AddToLogger(manager.GetLogger()),
		config:              conf,
		listWatchdogTargets: target,

		interval:                    time.Minute,
		minPodAge:                   time.Minute * 5,
		minVotesBeforeEviction:      conf.ConcurrencyWatchdogVotes,
		minPercentOverAvgBeforeVote: conf.ConcurrencyWatchdogThres,
		voteTTL:                     time.Minute * 10,

		votes: ring.New(20),
	}

	return manager.Add(c)
}

func (c *ConcurrencyWatchdog) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(util.Jitter(c.interval, 0.3)):
		}
		if err := c.tick(ctx); err != nil {
			c.logger.Error(err, "error reconciling ingress controller resources")
			continue
		}
	}
}

func (c *ConcurrencyWatchdog) tick(ctx context.Context) error {
	lgr := c.logger
	start := time.Now()
	var retErr *multierror.Error
	defer func() {
		lgr.Info("finished checking on ingress controller pods", "latencySec", time.Since(start).Seconds())

		// placing this call inside a closure allows for result and err to be bound after tick executes
		// this makes sure they have the proper value
		// just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		// the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(concurrencyWatchdogControllerName, ctrl.Result{}, retErr.ErrorOrNil())
	}()

	lgr.Info("listing watchdog targets")
	targets, err := c.listWatchdogTargets()
	if err != nil {
		lgr.Error(err, "listing watchdog targets")
		retErr = multierror.Append(retErr, fmt.Errorf("listing watchdog targets: %w", err))
		return retErr.ErrorOrNil()
	}

	for _, target := range targets {
		lgr := c.logger.WithValues("target", target.PodLabels)
		lgr.Info("starting checking on ingress controller pods")

		lgr.Info("listing pods")
		list := &corev1.PodList{}
		err := c.client.List(ctx, list, client.InNamespace(c.config.NS), client.MatchingLabels(target.PodLabels))
		if err != nil {
			lgr.Error(err, "listing pods")
			retErr = multierror.Append(retErr, fmt.Errorf("listing pods: %w", err))
			continue
		}

		lgr.Info("gathering metrics for each pod")
		connectionCountByPod := make([]float64, len(list.Items))
		nReadyPods := 0
		var totalConnectionCount float64
		for i, pod := range list.Items {
			lgr := lgr.WithValues("pod", pod.Name, "namespace", pod.Namespace)
			if !podIsReady(&pod) {
				lgr.Info("pod is not ready", "name", pod.Name)
				continue
			}
			nReadyPods++
			ctx := logr.NewContext(ctx, lgr)
			count, err := target.ScrapeFn(ctx, c.restClient, &pod)
			if err != nil {

				// check if pod is still ready. Pod might have become unready after checking it (this solves a race condition).
				// we ignore an error on the podIsActive call, we want the retErr to be the error from scrapping not from checking if
				// the pod is active.
				if active, err := podIsActive(ctx, lgr, c.client, client.ObjectKeyFromObject(&pod)); err == nil && !active {
					lgr.Info("pod isn't active anymore")
					continue
				}

				lgr.Error(err, "scraping pod")
				retErr = multierror.Append(retErr, fmt.Errorf("scraping pod %q: %w", pod.Name, err))
				continue
			}
			connectionCountByPod[i] = count
			totalConnectionCount += count
		}
		avgConnectionCount := totalConnectionCount / float64(nReadyPods)

		// Only rebalance connections when three or more replicas are ready.
		// Otherwise we will just push the connections to the other replica.
		if nReadyPods < 3 {
			lgr.Info("not enough ready pods to rebalance connections", "readyPods", nReadyPods)
			continue
		}

		lgr.Info("processing votes")
		pod := c.processVotes(list, connectionCountByPod, avgConnectionCount)
		if pod == "" {
			lgr.Info("no pod to evict")
			continue
		}

		lgr.Info("evicting pod due to high relative connection concurrency", "name", pod)
		eviction := &policyv1beta1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod,
				Namespace: c.config.NS,
			},
		}

		if err := c.clientset.CoreV1().Pods(eviction.Namespace).EvictV1beta1(ctx, eviction); err != nil {
			lgr.Error(err, "unable to evict pod", "name", pod)
			// don't add the error to return since we shouldn't retry right away
		}
	}
	if err := retErr.ErrorOrNil(); err != nil {
		c.logger.Error(err, "reconciling ingress controller resources")
		return err
	}
	return nil
}

func (c *ConcurrencyWatchdog) processVotes(list *corev1.PodList, connectionCountByPod []float64, avgConnectionCount float64) string {
	// Vote on outlier(s)
	podsByName := map[string]struct{}{}
	for i, pod := range list.Items {
		podsByName[pod.Name] = struct{}{}

		rank := (connectionCountByPod[i] / avgConnectionCount) * 100
		if rank < c.minPercentOverAvgBeforeVote || time.Since(pod.CreationTimestamp.Time) < c.minPodAge {
			continue
		}
		c.logger.Info("voting to evict pod due to high connection concurrency", "name", pod.Name, "percentOfAvg", rank)

		c.votes = c.votes.Next()
		var vote *evictionVote
		if c.votes.Value == nil {
			vote = &evictionVote{}
			c.votes.Value = vote
		} else {
			vote = c.votes.Value.(*evictionVote)
		}

		vote.PodName = pod.Name
		vote.Time = time.Now()
	}

	// Aggregate votes
	votesPerPod := map[string]int{}
	c.votes.Do(func(cur interface{}) {
		vote, ok := cur.(*evictionVote)
		if !ok {
			return
		}
		if _, exists := podsByName[vote.PodName]; !exists || time.Since(vote.Time) > c.voteTTL {
			return
		}
		votesPerPod[vote.PodName]++
	})

	// Apply votes
	for pod, votes := range votesPerPod {
		if votes < c.minVotesBeforeEviction {
			continue
		}
		return pod
	}
	return ""
}

func (c *ConcurrencyWatchdog) NeedLeaderElection() bool {
	return true
}

type evictionVote struct {
	Time    time.Time
	PodName string
}

func metricHasLabel(metric *prommodel.Metric, key, value string) bool {
	for _, cur := range metric.Label {
		if cur.GetName() == key && cur.GetValue() == value {
			return true
		}
	}
	return false
}

func podIsReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// podIsActive checks if a Pod is Ready and exists and is able to serve connections
func podIsActive(ctx context.Context, lgr logr.Logger, cl client.Client, pod client.ObjectKey) (bool, error) {
	var obj corev1.Pod
	err := cl.Get(ctx, pod, &obj)
	if apierrors.IsNotFound(err) {
		lgr.Info("pod doesn't exist anymore")
		return false, nil
	}
	if err != nil {
		lgr.Error(err, "failed to get pod")
		return false, fmt.Errorf("getting pod: %w", err)
	}

	if !podIsReady(&obj) {
		lgr.Info("pod isn't ready anymore")
		return false, nil
	}

	return true, nil
}
