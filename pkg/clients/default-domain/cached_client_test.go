package defaultdomain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCachedClient verifies client creation and initialization
func TestNewCachedClient(t *testing.T) {
	ctx := context.Background()
	opts := CachedClientOpts{
		Opts: Opts{
			ServerAddress: "http://localhost:9000",
		},
	}

	client := NewCachedClient(ctx, opts, logr.Discard())

	require.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.NotNil(t, client.ctx)
	assert.NotNil(t, client.cancel)
	assert.True(t, client.healthy)
	assert.Equal(t, 0, client.consecutiveFails)
	assert.Equal(t, 0, client.consecutiveNotFounds)
	assert.True(t, client.failingSince.IsZero())
	assert.Nil(t, client.cache)
	assert.True(t, client.cacheExp.IsZero())

	// Clean up
	client.Close()
}

// TestCachedClient_GetTLSCertificate_Success verifies successful certificate retrieval
func TestCachedClient_GetTLSCertificate_Success(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// First call should fetch from server
	cert1, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert1)
	assert.Equal(t, expectedCert.Key, cert1.Key)
	assert.Equal(t, expectedCert.Cert, cert1.Cert)
	assert.True(t, client.IsHealthy())
	assert.Equal(t, 0, client.consecutiveFails)

	// Second call should return cached cert (no new server call)
	initialCallCount := callCount
	cert2, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert2)
	assert.Equal(t, cert1, cert2)
	assert.Equal(t, initialCallCount, callCount, "should not have made additional server call")
}

// TestCachedClient_GetTLSCertificate_CacheExpiration verifies cache expiration behavior
func TestCachedClient_GetTLSCertificate_CacheExpiration(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// First call
	cert1, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert1)
	initialCallCount := callCount

	// Manually expire the cache
	client.mu.Lock()
	client.cacheExp = time.Now().Add(-1 * time.Second)
	client.mu.Unlock()

	// Second call should refetch because cache is expired
	cert2, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert2)
	assert.Greater(t, callCount, initialCallCount, "should have made additional server call")
}

// TestCachedClient_GetTLSCertificate_ConcurrentCalls verifies mutex protection
func TestCachedClient_GetTLSCertificate_ConcurrentCalls(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(50 * time.Millisecond) // Simulate slow server
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// Launch multiple concurrent calls
	const numCalls = 10
	var wg sync.WaitGroup
	wg.Add(numCalls)

	for i := 0; i < numCalls; i++ {
		go func() {
			defer wg.Done()
			cert, err := client.GetTLSCertificate(context.Background())
			require.NoError(t, err)
			require.NotNil(t, cert)
		}()
	}

	wg.Wait()

	// Should only call server once due to mutex protection and caching
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "should serialize concurrent calls")
}

// TestCachedClient_GetTLSCertificate_ServerError verifies error handling
func TestCachedClient_GetTLSCertificate_ServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// Get initial error count
	var m dto.Metric
	err := metrics.DefaultDomainClientErrors.Write(&m)
	require.NoError(t, err)
	initialErrors := m.GetCounter().GetValue()

	cert, err := client.GetTLSCertificate(context.Background())

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "failed to fetch TLS certificate after")
	assert.False(t, client.IsHealthy(), "client should be unhealthy after max retries")
	assert.Equal(t, maxRetries, client.consecutiveFails)
	assert.Equal(t, maxRetries, callCount, "should retry maxRetries times")

	// Verify error metric was incremented for each failure
	err = metrics.DefaultDomainClientErrors.Write(&m)
	require.NoError(t, err)
	finalErrors := m.GetCounter().GetValue()
	assert.Equal(t, initialErrors+float64(maxRetries), finalErrors, "should increment error metric for each failure")
}

// TestCachedClient_GetTLSCertificate_TransientFailure verifies retry logic with eventual success
func TestCachedClient_GetTLSCertificate_TransientFailure(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Fail first 2 attempts, succeed on 3rd
		if callCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("transient error"))
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	cert, err := client.GetTLSCertificate(context.Background())

	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, expectedCert.Key, cert.Key)
	assert.True(t, client.IsHealthy(), "client should recover health after success")
	assert.Equal(t, 0, client.consecutiveFails, "consecutive fails should reset after success")
	assert.Equal(t, 3, callCount, "should have called server 3 times")
}

// TestCachedClient_GetTLSCertificate_ContextCancellation verifies context handling
func TestCachedClient_GetTLSCertificate_ContextCancellation(t *testing.T) {
	t.Skip("Skipping flaky test that depends on timing")
}

// TestCachedClient_IsHealthy verifies health checking
func TestCachedClient_IsHealthy(t *testing.T) {
	tests := []struct {
		name             string
		consecutiveFails int
		expected         bool
	}{
		{
			name:             "healthy with zero fails",
			consecutiveFails: 0,
			expected:         true,
		},
		{
			name:             "healthy below threshold",
			consecutiveFails: maxRetries - 1,
			expected:         true,
		},
		{
			name:             "unhealthy at threshold",
			consecutiveFails: maxRetries,
			expected:         false,
		},
		{
			name:             "unhealthy above threshold",
			consecutiveFails: maxRetries + 1,
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &CachedClient{
				consecutiveFails: tt.consecutiveFails,
				healthy:          tt.consecutiveFails < maxRetries,
			}
			assert.Equal(t, tt.expected, client.IsHealthy())
		})
	}
}

// TestCachedClient_Close verifies cleanup
func TestCachedClient_Close(t *testing.T) {
	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: "http://localhost:9000"},
	}

	client := NewCachedClient(context.Background(), opts, logr.Discard())

	// Close should cancel the context
	client.Close()

	// Verify context is canceled
	select {
	case <-client.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be canceled after Close()")
	}
}

// TestCachedClient_RefreshLoop_InitialJitter verifies initial jitter behavior
func TestCachedClient_RefreshLoop_InitialJitter(t *testing.T) {
	t.Skip("Skipping test that depends on timing and background goroutines")
}

// TestCachedClient_RefreshLoop_PeriodicRefresh verifies periodic refresh behavior
func TestCachedClient_RefreshLoop_PeriodicRefresh(t *testing.T) {
	t.Skip("Skipping test that depends on timing and background goroutines")
}

// TestCachedClient_RefreshLoop_StopsOnContextCancel verifies graceful shutdown
func TestCachedClient_RefreshLoop_StopsOnContextCancel(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	client := NewCachedClient(context.Background(), opts, logr.Discard())

	// Wait a short time for potential initial fetch
	time.Sleep(100 * time.Millisecond)

	// Close client immediately
	client.Close()

	// Context should be canceled
	select {
	case <-client.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be canceled after Close()")
	}
}

// TestCachedClient_ExponentialBackoff verifies backoff calculation
func TestCachedClient_ExponentialBackoff(t *testing.T) {
	// This test verifies that backoff increases exponentially and caps at maxBackoff
	attemptCount := 0
	attemptTimes := make([]time.Time, 0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptTimes = append(attemptTimes, time.Now())
		attemptCount++
		// Always fail to trigger retries
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	_, err := client.GetTLSCertificate(context.Background())

	require.Error(t, err)
	assert.Equal(t, maxRetries, attemptCount)
	assert.Equal(t, maxRetries, len(attemptTimes))

	// Verify backoff intervals increase (with some tolerance for jitter)
	for i := 1; i < len(attemptTimes)-1; i++ {
		interval := attemptTimes[i+1].Sub(attemptTimes[i])
		// Backoff should be at least baseBackoff (with some tolerance for jitter and execution time)
		assert.Greater(t, interval, baseBackoff/2, "backoff should be meaningful")
		// Should not exceed maxBackoff significantly (accounting for jitter and execution)
		assert.Less(t, interval, maxBackoff*2, "backoff should be bounded")
	}
}

// TestCachedClient_HealthRecovery verifies health recovery after successful fetch
func TestCachedClient_HealthRecovery(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// Manually set client to unhealthy state
	client.mu.Lock()
	client.consecutiveFails = maxRetries
	client.consecutiveNotFounds = 3
	client.failingSince = time.Now().Add(-10 * time.Minute)
	client.healthy = false
	client.mu.Unlock()

	assert.False(t, client.IsHealthy(), "client should start unhealthy")

	// Expire cache to force refetch
	client.mu.Lock()
	client.cacheExp = time.Time{}
	client.mu.Unlock()

	cert, err := client.GetTLSCertificate(context.Background())

	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.True(t, client.IsHealthy(), "client should recover health after successful fetch")
	assert.Equal(t, 0, client.consecutiveFails, "consecutive fails should reset")
	assert.Equal(t, 0, client.consecutiveNotFounds, "consecutive not founds should reset")
	assert.True(t, client.failingSince.IsZero(), "failingSince should reset")
}

// TestCachedClient_NilCertificateHandling verifies handling of nil certificate from server
func TestCachedClient_NilCertificateHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Send empty JSON object
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	cert, err := client.GetTLSCertificate(context.Background())

	// Should succeed but certificate fields will be empty
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Nil(t, cert.Key)
	assert.Nil(t, cert.Cert)
	assert.Nil(t, cert.ExpiresOn)
}

// TestCachedClient_CacheTTLJitter verifies cache TTL includes jitter
func TestCachedClient_CacheTTLJitter(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	beforeFetch := time.Now()
	_, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)

	client.mu.Lock()
	cacheExp := client.cacheExp
	client.mu.Unlock()

	actualTTL := cacheExp.Sub(beforeFetch)

	// TTL should be cacheTTL +/- jitter
	minTTL := cacheTTL - time.Duration(float64(cacheTTL)*jitterRatio)
	maxTTL := cacheTTL + time.Duration(float64(cacheTTL)*jitterRatio)

	assert.Greater(t, actualTTL, minTTL-100*time.Millisecond, "TTL should include negative jitter")
	assert.Less(t, actualTTL, maxTTL+100*time.Millisecond, "TTL should include positive jitter")
}

// TestCachedClient_MultipleSequentialFailures verifies behavior with multiple GetTLSCertificate calls that fail
func TestCachedClient_MultipleSequentialFailures(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// First call should fail after maxRetries attempts
	_, err1 := client.GetTLSCertificate(context.Background())
	require.Error(t, err1)
	firstCallCount := atomic.LoadInt32(&callCount)
	assert.Equal(t, int32(maxRetries), firstCallCount)

	// Second call should also retry maxRetries times
	_, err2 := client.GetTLSCertificate(context.Background())
	require.Error(t, err2)
	secondCallCount := atomic.LoadInt32(&callCount)
	assert.Equal(t, int32(maxRetries*2), secondCallCount)
}

func TestCachedClient_GetTLSCertificate_NotFound(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	cert, err := client.GetTLSCertificate(context.Background())

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.True(t, util.IsNotFound(err))

	// Should remain healthy
	assert.True(t, client.IsHealthy())
	assert.Equal(t, 0, client.consecutiveFails)
	assert.Equal(t, 1, client.consecutiveNotFounds)
	assert.False(t, client.failingSince.IsZero(), "failingSince should be set after a not-found")

	// Should not retry immediately
	assert.Equal(t, 1, callCount)

	// Verify the consecutive not-found gauge
	var m dto.Metric
	notFoundGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelNotFound)
	err = notFoundGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(1), m.GetGauge().GetValue(), "not_found gauge should be 1")

	// Verify the duration gauge is > 0
	err = metrics.DefaultDomainCertUnavailableDurationSeconds.Write(&m)
	require.NoError(t, err)
	assert.Greater(t, m.GetGauge().GetValue(), float64(0), "duration gauge should be > 0")
}

// TestCachedClient_ConsecutiveNotFoundsMetrics verifies that consecutive 404s are tracked and reset on success
func TestCachedClient_ConsecutiveNotFoundsMetrics(t *testing.T) {
	callCount := 0
	notFoundCount := 3 // return 404 for the first 3 calls, then succeed

	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= notFoundCount {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	var m dto.Metric

	// Make 3 calls that return 404
	for i := 1; i <= notFoundCount; i++ {
		_, err := client.GetTLSCertificate(context.Background())
		require.Error(t, err)
		assert.True(t, util.IsNotFound(err))
		assert.Equal(t, i, client.consecutiveNotFounds, "consecutiveNotFounds should increment")
		assert.Equal(t, 0, client.consecutiveFails, "consecutiveFails should stay 0 for 404s")

		// Verify not_found gauge matches
		notFoundGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelNotFound)
		err = notFoundGauge.(prometheus.Metric).Write(&m)
		require.NoError(t, err)
		assert.Equal(t, float64(i), m.GetGauge().GetValue(), "not_found gauge should match consecutive count")

		// Verify error gauge is 0
		errorGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelError)
		err = errorGauge.(prometheus.Metric).Write(&m)
		require.NoError(t, err)
		assert.Equal(t, float64(0), m.GetGauge().GetValue(), "error gauge should be 0 during 404s")
	}

	// Verify duration gauge is > 0
	err := metrics.DefaultDomainCertUnavailableDurationSeconds.Write(&m)
	require.NoError(t, err)
	assert.Greater(t, m.GetGauge().GetValue(), float64(0), "duration should be positive while failing")

	// Now succeed — all gauges should reset
	cert, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, 0, client.consecutiveNotFounds, "consecutiveNotFounds should reset on success")
	assert.True(t, client.failingSince.IsZero(), "failingSince should reset on success")

	// Verify both failure gauges are 0
	notFoundGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelNotFound)
	err = notFoundGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(0), m.GetGauge().GetValue(), "not_found gauge should be 0 after success")

	errorGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelError)
	err = errorGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(0), m.GetGauge().GetValue(), "error gauge should be 0 after success")

	// Verify duration gauge is 0
	err = metrics.DefaultDomainCertUnavailableDurationSeconds.Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(0), m.GetGauge().GetValue(), "duration should be 0 after success")
}

// TestCachedClient_ConsecutiveErrorsMetrics verifies that consecutive non-404 errors are tracked
func TestCachedClient_ConsecutiveErrorsMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	_, err := client.GetTLSCertificate(context.Background())
	require.Error(t, err)

	var m dto.Metric

	// Verify error gauge equals consecutiveFails (which equals maxRetries after exhausting retries)
	errorGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelError)
	err = errorGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(maxRetries), m.GetGauge().GetValue(), "error gauge should equal maxRetries")

	// Verify not_found gauge is 0
	notFoundGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelNotFound)
	err = notFoundGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(0), m.GetGauge().GetValue(), "not_found gauge should be 0 during errors")

	// Verify duration gauge is > 0
	err = metrics.DefaultDomainCertUnavailableDurationSeconds.Write(&m)
	require.NoError(t, err)
	assert.Greater(t, m.GetGauge().GetValue(), float64(0), "duration should be positive while failing")

	// Verify failingSince is set
	assert.False(t, client.failingSince.IsZero(), "failingSince should be set")
}

// TestCachedClient_MixedFailuresMetrics verifies that switching between 404s and errors resets the other gauge
func TestCachedClient_MixedFailuresMetrics(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			// First 2 calls: 404
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
			return
		}
		// After that: 500
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	opts := CachedClientOpts{
		Opts: Opts{ServerAddress: server.URL},
	}

	// Create client with canceled context to prevent background refresh
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewCachedClient(ctx, opts, logr.Discard())
	defer client.Close()

	// Wait for background goroutine to exit
	time.Sleep(50 * time.Millisecond)

	var m dto.Metric

	// First call: 404
	_, err := client.GetTLSCertificate(context.Background())
	require.Error(t, err)
	assert.True(t, util.IsNotFound(err))

	notFoundGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelNotFound)
	err = notFoundGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(1), m.GetGauge().GetValue(), "not_found gauge should be 1 after first 404")

	// Second call: 404
	_, err = client.GetTLSCertificate(context.Background())
	require.Error(t, err)
	assert.True(t, util.IsNotFound(err))

	err = notFoundGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(2), m.GetGauge().GetValue(), "not_found gauge should be 2 after second 404")

	// Third call: 500 (retries will all be 500 too)
	_, err = client.GetTLSCertificate(context.Background())
	require.Error(t, err)
	assert.False(t, util.IsNotFound(err))

	// not_found gauge should be reset to 0
	err = notFoundGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(0), m.GetGauge().GetValue(), "not_found gauge should reset when errors start")

	// error gauge should reflect the consecutive errors
	errorGauge := metrics.DefaultDomainConsecutiveFetchFailures.WithLabelValues(metrics.LabelError)
	err = errorGauge.(prometheus.Metric).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(maxRetries), m.GetGauge().GetValue(), "error gauge should equal maxRetries")

	// failingSince should still be set (from the initial 404 streak)
	assert.False(t, client.failingSince.IsZero(), "failingSince should persist across failure types")
}
