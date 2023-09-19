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

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	prommodel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
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

var (
	concurrencyWatchdogControllerName = controllername.New("concurrency", "watchdog")
)

// ScrapeFn returns the connection count for the given pod
type ScrapeFn func(ctx context.Context, client rest.Interface, pod *corev1.Pod) (float64, error)

// NginxScrapeFn is the scrape function for Nginx
func NginxScrapeFn(ctx context.Context, client rest.Interface, pod *corev1.Pod) (float64, error) {
	resp, err := client.Get().
		AbsPath("/api/v1/namespaces", pod.Namespace, "pods", pod.Name+":10254", "proxy/metrics").
		Timeout(time.Second * 30).
		MaxRetries(4).
		DoRaw(ctx)
	if err != nil {
		return 0, err
	}

	family := &prommodel.MetricFamily{}
	dec := expfmt.NewDecoder(bytes.NewReader(resp), expfmt.FmtOpenMetrics_0_0_1)
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

// LabelGetter returns unique pod labels for an Ingress controller
type LabelGetter interface {
	PodLabels() map[string]string
}

// WatchdogTarget refers to a target the concurrency watchdog should track
type WatchdogTarget struct {
	ScrapeFn ScrapeFn
	LabelGetter
}

// ConcurrencyWatchdog evicts ingress controller pods that have too many active connections relative to others.
// This helps redistribute long-running connections when the ingress controller scales up.
type ConcurrencyWatchdog struct {
	client     client.Client
	clientset  kubernetes.Interface
	restClient rest.Interface
	logger     logr.Logger
	config     *config.Config
	targets    []*WatchdogTarget

	interval, minPodAge, voteTTL time.Duration
	minVotesBeforeEviction       int
	minPercentOverAvgBeforeVote  float64

	votes *ring.Ring
}

func NewConcurrencyWatchdog(manager ctrl.Manager, conf *config.Config, targets []*WatchdogTarget) error {
	metrics.InitControllerMetrics(concurrencyWatchdogControllerName)
	clientset, err := kubernetes.NewForConfig(manager.GetConfig())
	if err != nil {
		return err
	}

	c := &ConcurrencyWatchdog{
		client:     manager.GetClient(),
		clientset:  clientset,
		restClient: clientset.CoreV1().RESTClient(),
		logger:     concurrencyWatchdogControllerName.AddToLogger(manager.GetLogger()),
		config:     conf,
		targets:    targets,

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
	start := time.Now()
	var retErr *multierror.Error
	defer func() {
		c.logger.Info("finished checking on ingress controller pods", "latencySec", time.Since(start).Seconds())

		//placing this call inside a closure allows for result and err to be bound after tick executes
		//this makes sure they have the proper value
		//just calling defer metrics.HandleControllerReconcileMetrics(controllerName, result, err) would bind
		//the values of result and err to their zero values, since they were just instantiated
		metrics.HandleControllerReconcileMetrics(concurrencyWatchdogControllerName, ctrl.Result{}, retErr.ErrorOrNil())
	}()

	for _, target := range c.targets {
		c.logger.Info("starting checking on ingress controller pods", "labels", target.PodLabels())

		list := &corev1.PodList{}
		err := c.client.List(ctx, list, client.InNamespace(c.config.NS), client.MatchingLabels(target.LabelGetter.PodLabels()))
		if err != nil {
			c.logger.Error(err, "error listing pods")
			retErr = multierror.Append(retErr, fmt.Errorf("listing pods: %w", err))
			continue
		}

		connectionCountByPod := make([]float64, len(list.Items))
		nReadyPods := 0
		var totalConnectionCount float64
		for i, pod := range list.Items {
			if !podIsReady(&pod) {
				c.logger.Info("pod is not ready", "name", pod.Name)
				continue
			}
			nReadyPods++
			count, err := target.ScrapeFn(ctx, c.restClient, &pod)
			if err != nil {
				c.logger.Error(err, "error scraping pod", "name", pod.Name)
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
			continue
		}

		pod := c.processVotes(list, connectionCountByPod, avgConnectionCount)
		if pod == "" {
			continue
		}

		c.logger.Info("evicting pod due to high relative connection concurrency", "name", pod)
		eviction := &policyv1beta1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod,
				Namespace: c.config.NS,
			},
		}

		if err := c.clientset.CoreV1().Pods(eviction.Namespace).EvictV1beta1(ctx, eviction); err != nil {
			c.logger.Error(err, "unable to evict pod", "name", pod)
			// don't add the error to return since we shouldn't retry right away
		}
	}
	if retErr.ErrorOrNil() != nil {
		c.logger.Error(retErr, "error reconciling ingress controller resources")
	}
	return retErr.ErrorOrNil()
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
