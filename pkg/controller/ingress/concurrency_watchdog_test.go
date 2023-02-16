// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"container/ring"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakecgo "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
)

func TestConcurrencyWatchdogPositive(t *testing.T) {
	ctx := context.Background()
	list := buildTestPods(5)
	cli := fake.NewClientBuilder().WithLists(list).Build()
	cs := fakecgo.NewSimpleClientset()

	c := newTestConcurrencyWatchdog()
	c.clientset = cs
	c.client = cli
	c.scrapeFn = func(ctx context.Context, pod *corev1.Pod) (float64, error) {
		if pod.Name == "pod-1" {
			return 2000, nil
		}
		return 1, nil
	}

	// No eviction after first tick of the loop
	require.NoError(t, c.tick(ctx))
	assert.Len(t, cs.Fake.Actions(), 0)

	// Eviction after second tick of the loop
	require.NoError(t, c.tick(ctx))
	assert.Len(t, cs.Fake.Actions(), 1)
}

func TestConcurrencyWatchdogPodNotReady(t *testing.T) {
	ctx := context.Background()
	list := buildTestPods(2)
	list.Items[0].Status.Conditions[0].Status = corev1.ConditionFalse
	cli := fake.NewClientBuilder().WithLists(list).Build()

	c := newTestConcurrencyWatchdog()
	c.client = cli
	c.scrapeFn = func(ctx context.Context, pod *corev1.Pod) (float64, error) {
		if pod.Name == "pod-1" {
			return 2000, nil
		}
		return 1, nil
	}

	// No eviction after first tick of the loop
	require.NoError(t, c.tick(ctx))
	eviction := &policyv1beta1.Eviction{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}}
	assert.True(t, errors.IsNotFound(cli.Get(ctx, client.ObjectKeyFromObject(eviction), eviction)))

	// No eviction after first tick of the loop because only two pods are ready
	require.NoError(t, c.tick(ctx))
	assert.True(t, errors.IsNotFound(cli.Get(ctx, client.ObjectKeyFromObject(eviction), eviction)))
}

func TestConcurrencyWatchdogProcessVotesNegative(t *testing.T) {
	c := newTestConcurrencyWatchdog()
	c.minVotesBeforeEviction = 1

	list := buildTestPods(5)
	connectionCountByPod := []float64{10, 10, 10, 10, 10}
	avgConnectionCount := float64(10)

	pod := c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Empty(t, pod, "no pod was evicted")
}

func TestConcurrencyWatchdogProcessVotesPositive(t *testing.T) {
	c := newTestConcurrencyWatchdog()

	// First vote (over threshold)
	list := buildTestPods(5)
	connectionCountByPod := []float64{10, 20, 10, 10, 10}
	avgConnectionCount := float64(10)

	pod := c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Empty(t, pod, "no pod was evicted")

	// Second vote (under threshold)
	connectionCountByPod = []float64{10, 10, 10, 10, 10}
	pod = c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Empty(t, pod, "no pod was evicted")

	// Third vote (over threshold)
	connectionCountByPod = []float64{10, 20, 10, 10, 10}
	pod = c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Equal(t, "pod-1", pod, "the pod was evicted")
}

func TestConcurrencyWatchdogScrapeHappyPath(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/namespaces/test-ns/pods/test-pod:10254/proxy/metrics", r.URL.Path)

		io.WriteString(w, strings.Join([]string{
			// Another metric
			"# TYPE a_not_our_metric gauge",
			"a_not_our_metric{state=\"active\"} 123",

			// The right metric
			"# TYPE nginx_ingress_controller_nginx_process_connections gauge",
			"nginx_ingress_controller_nginx_process_connections{state=\"active\"} 123",
			"",
		}, "\n"))
	}))
	defer svr.Close()

	u, err := url.Parse(svr.URL)
	require.NoError(t, err)

	c := newTestConcurrencyWatchdog()
	c.restClient, err = rest.NewRESTClient(u, "", rest.ClientContentConfig{}, nil, http.DefaultClient)
	require.NoError(t, err)

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test-ns"}}
	value, err := c.scrape(context.Background(), pod)
	require.NoError(t, err)
	assert.Equal(t, float64(123), value)
}

func TestConcurrencyWatchdogScrapeMissingLabel(t *testing.T) {
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Join([]string{
			"# TYPE nginx_ingress_controller_nginx_process_connections gauge",
			"nginx_ingress_controller_nginx_process_connections{state=\"notactive\"} 123",
			"",
		}, "\n"))
	}))
	defer svr.Close()

	u, err := url.Parse(svr.URL)
	require.NoError(t, err)

	c := newTestConcurrencyWatchdog()
	c.restClient, err = rest.NewRESTClient(u, "", rest.ClientContentConfig{}, nil, http.DefaultClient)
	require.NoError(t, err)

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "test-ns"}}
	_, err = c.scrape(context.Background(), pod)
	require.EqualError(t, err, "active connections metric not found")
}

func TestConcurrencyWatchdogProcessVotesWrapAroundBuffer(t *testing.T) {
	c := newTestConcurrencyWatchdog()

	// Fill buffer for votes for pod 1
	list := buildTestPods(5)
	connectionCountByPod := []float64{10, 20, 10, 10, 10}
	avgConnectionCount := float64(10)
	for i := 0; i < 30; i++ {
		n := i
		if n > 20 {
			n = 20
		}
		assert.Equal(t, n, countVotes(c, "pod-1"))
		c.processVotes(list, connectionCountByPod, avgConnectionCount)
	}

	// Replace buffer with votes for pod 2
	connectionCountByPod = []float64{10, 10, 20, 10, 10}
	for i := 0; i < 30; i++ {
		n := i
		if n > 20 {
			n = 20
		}
		assert.Equal(t, n, countVotes(c, "pod-2"))
		c.processVotes(list, connectionCountByPod, avgConnectionCount)
	}
}

func TestConcurrencyWatchdogProcessVotesNewPods(t *testing.T) {
	c := newTestConcurrencyWatchdog()
	list := buildTestPods(5)
	list.Items[1].CreationTimestamp.Time = time.Now()     // pod 1 is new
	connectionCountByPod := []float64{10, 20, 10, 10, 10} // pod 1 should get a vote
	avgConnectionCount := float64(10)
	c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Equal(t, 0, countVotes(c, "pod-1"))
}

func TestConcurrencyWatchdogProcessVotesOldVotes(t *testing.T) {
	c := newTestConcurrencyWatchdog()
	list := buildTestPods(5)
	connectionCountByPod := []float64{10, 20, 10, 10, 10} // pod 1 should get a vote
	avgConnectionCount := float64(10)

	// Register the first vote
	pod := c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Empty(t, pod, "no pod was evicted")

	// Mutate the vote into the past so that it won't be considered
	c.votes.Value.(*evictionVote).Time = time.Now().Add(-time.Hour)

	// The pod would have been evicted if both votes were considered
	pod = c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Empty(t, pod, "no pod was evicted")
}

func TestConcurrencyWatchdogProcessVotesMissingPod(t *testing.T) {
	c := newTestConcurrencyWatchdog()
	list := buildTestPods(5)
	connectionCountByPod := []float64{10, 20, 10, 10, 10} // pod 1 should get a vote
	avgConnectionCount := float64(10)

	// Register the first vote
	pod := c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Empty(t, pod, "no pod was evicted")

	// Mutate the vote to reference a different pod that doesn't exist
	c.votes.Value.(*evictionVote).PodName = "nope"

	// The pod would have been evicted if both votes were considered
	pod = c.processVotes(list, connectionCountByPod, avgConnectionCount)
	assert.Empty(t, pod, "no pod was evicted")
}

func buildTestPods(n int) *corev1.PodList {
	list := &corev1.PodList{}
	for i := 0; i < n; i++ {
		list.Items = append(list.Items, corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("pod-%d", i),
				CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
				Labels:            manifests.IngressPodLabels,
			},
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		})
	}
	return list
}

func countVotes(c *ConcurrencyWatchdog, pod string) int {
	var n int
	c.votes.Do(func(obj interface{}) {
		vote, ok := obj.(*evictionVote)
		if ok && vote.PodName == pod {
			n++
		}
	})
	return n
}

func newTestConcurrencyWatchdog() *ConcurrencyWatchdog {
	return &ConcurrencyWatchdog{
		config:                      &config.Config{},
		logger:                      logr.Discard(),
		minPodAge:                   time.Minute,
		voteTTL:                     time.Second,
		minVotesBeforeEviction:      2,
		minPercentOverAvgBeforeVote: 200,
		votes:                       ring.New(20),
	}
}
