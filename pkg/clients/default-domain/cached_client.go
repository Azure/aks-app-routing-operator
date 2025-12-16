package defaultdomain

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
)

const (
	// cacheTTL is the time-to-live for cached certificates (6 hours)
	cacheTTL = 6 * time.Hour
	// jitterRatio is the ratio of jitter to add to cache TTL (0.0833 = ~30 min for 6 hour TTL)
	jitterRatio = 0.0833
	// initialJitter is the maximum jitter for the initial fetch (5 minutes)
	initialJitter = 5 * time.Minute
	// maxRetries is the maximum number of retry attempts
	maxRetries = 5
	// baseBackoff is the base duration for exponential backoff
	baseBackoff = 1 * time.Second
	// maxBackoff is the maximum backoff duration
	maxBackoff = 30 * time.Second
	// backoffJitterRatio is the ratio of jitter to add to backoff (0.5 = 50%)
	backoffJitterRatio = 0.5
)

// CachedClientOpts contains configuration options for the cached client
type CachedClientOpts struct {
	Opts
	SubscriptionID string
	ResourceGroup  string
	ClusterName    string
	CCPID          string
}

// CachedClient is a client that caches TLS certificates with automatic refresh
type CachedClient struct {
	client           *Client
	logger           logr.Logger
	opts             CachedClientOpts
	mu               sync.Mutex
	cache            *TLSCertificate
	cacheExp         time.Time
	consecutiveFails int
	healthy          bool
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewCachedClient creates a new cached client with automatic refresh
func NewCachedClient(ctx context.Context, opts CachedClientOpts, logger logr.Logger) *CachedClient {
	childCtx, cancel := context.WithCancel(ctx)

	c := &CachedClient{
		client:  NewClient(opts.Opts),
		logger:  logger,
		opts:    opts,
		ctx:     childCtx,
		cancel:  cancel,
		healthy: true, // Start healthy
	}

	// Start background refresh with initial jitter to avoid thundering herd
	go c.refreshLoop()

	return c
}

// GetTLSCertificate retrieves the cached TLS certificate or fetches a new one if expired
func (c *CachedClient) GetTLSCertificate(ctx context.Context) (*TLSCertificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we have a valid cached certificate
	if c.cache != nil && time.Now().Before(c.cacheExp) {
		return c.cache, nil
	}

	// Cache expired or not present, fetch with lock held to prevent concurrent fetches
	return c.fetchWithRetryLocked(ctx)
}

// IsHealthy returns true if the client is healthy (has not exceeded max retries)
func (c *CachedClient) IsHealthy() bool {
	return c.healthy
}

// Close stops the background refresh loop
func (c *CachedClient) Close() {
	c.cancel()
}

// fetchWithRetryLocked fetches a certificate with retry logic
// Caller must hold the mutex - this ensures only one fetch happens at a time
func (c *CachedClient) fetchWithRetryLocked(ctx context.Context) (*TLSCertificate, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff
			backoff := time.Duration(1<<uint(attempt-1)) * baseBackoff
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			// Add jitter using util.Jitter
			backoffWithJitter := util.Jitter(backoff, backoffJitterRatio)

			c.logger.Info("retrying certificate fetch after backoff",
				"attempt", attempt+1,
				"maxRetries", maxRetries,
				"backoff", backoffWithJitter)

			// Sleep with lock held - this is intentional to prevent concurrent fetches
			time.Sleep(backoffWithJitter)
		}

		// Fetch with lock held - this is intentional to prevent concurrent fetches
		cert, err := c.client.GetTLSCertificate(ctx,
			c.opts.SubscriptionID,
			c.opts.ResourceGroup,
			c.opts.ClusterName,
			c.opts.CCPID,
		)

		if err == nil {
			// Success! Update cache and reset health tracking
			ttl := util.Jitter(cacheTTL, jitterRatio)
			c.cache = cert
			c.cacheExp = time.Now().Add(ttl)
			c.consecutiveFails = 0
			c.healthy = true

			c.logger.Info("updated certificate cache",
				"ttl", ttl,
				"expiresAt", c.cacheExp)
			return cert, nil
		}

		lastErr = err
		c.logger.Error(err, "failed to fetch TLS certificate",
			"attempt", attempt+1,
			"maxRetries", maxRetries)

		// Update health on each failed attempt
		c.consecutiveFails++
		if c.consecutiveFails >= maxRetries {
			c.healthy = false
			c.logger.Error(nil, "client marked unhealthy after consecutive failed attempts",
				"consecutiveFails", c.consecutiveFails,
				"maxRetries", maxRetries)
		}
	}

	// All retries failed
	return nil, fmt.Errorf("failed to fetch TLS certificate after %d attempts: %w", maxRetries, lastErr)
}

// refreshLoop periodically refreshes the certificate cache
func (c *CachedClient) refreshLoop() {
	// Add initial jitter to avoid thundering herd on startup
	initialDelay := time.Duration(rand.Int63n(int64(initialJitter)))
	c.logger.Info("starting certificate refresh loop", "initialDelay", initialDelay)

	select {
	case <-c.ctx.Done():
		return
	case <-time.After(initialDelay):
	}

	// Initial fetch
	c.mu.Lock()
	_, err := c.fetchWithRetryLocked(c.ctx)
	c.mu.Unlock()
	if err != nil {
		c.logger.Error(err, "initial certificate fetch failed")
	}

	// Periodic refresh
	for {
		c.mu.Lock()
		nextRefresh := c.cacheExp
		c.mu.Unlock()

		// If cache not set yet, use a default interval
		if nextRefresh.IsZero() {
			nextRefresh = time.Now().Add(cacheTTL)
		}

		waitDuration := time.Until(nextRefresh)
		if waitDuration < 0 {
			waitDuration = 0
		}

		c.logger.Info("scheduling next certificate refresh", "wait", waitDuration)

		select {
		case <-c.ctx.Done():
			c.logger.Info("stopping certificate refresh loop")
			return
		case <-time.After(waitDuration):
			c.mu.Lock()
			_, err := c.fetchWithRetryLocked(c.ctx)
			c.mu.Unlock()
			if err != nil {
				c.logger.Error(err, "periodic certificate refresh failed")
			}
		}
	}
}
