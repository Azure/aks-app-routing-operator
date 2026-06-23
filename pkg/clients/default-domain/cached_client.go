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
	// DefaultCacheTTL is the default time-to-live for cached certificates.
	DefaultCacheTTL = 15 * time.Minute
	// jitterRatio is the ratio of jitter to add to cache TTL.
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
	CacheTTL time.Duration
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
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = DefaultCacheTTL
	}

	c := &CachedClient{
		client:  NewClient(opts.Opts, logger),
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
		c.logger.Info("serving TLS certificate from cache",
			"expiresAt", c.cacheExp.UTC().Format(time.RFC3339),
			"validFor", time.Until(c.cacheExp).Truncate(time.Second).String())
		return c.cache, nil
	}

	// Cache expired or not present, fetch with lock held to prevent concurrent fetches
	if c.cache == nil {
		c.logger.Info("cache empty, fetching TLS certificate")
	} else {
		c.logger.Info("cache expired, fetching fresh TLS certificate",
			"expiredAt", c.cacheExp.UTC().Format(time.RFC3339))
	}
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
		cert, err := c.client.GetTLSCertificate(ctx)

		if err == nil {
			// Success! Update cache and reset health tracking
			ttl := util.Jitter(c.opts.CacheTTL, jitterRatio)
			wasUnhealthy := !c.healthy
			c.cache = cert
			c.cacheExp = time.Now().Add(ttl)
			c.consecutiveFails = 0
			c.healthy = true

			c.logger.Info("updated certificate cache",
				"attempt", attempt+1,
				"ttl", ttl.Truncate(time.Second).String(),
				"expiresAt", c.cacheExp.UTC().Format(time.RFC3339))
			if wasUnhealthy {
				c.logger.Info("default domain client recovered and is healthy again")
			}
			return cert, nil
		}

		// Use IsNotFound to check if the error is a 404
		if util.IsNotFound(err) {
			// 404 is a valid state (cert not ready yet), don't mark as unhealthy
			c.consecutiveFails = 0
			c.healthy = true
			c.logger.Info("certificate not found (404), service is reachable but certificate is not issued yet")
			return nil, err
		}

		lastErr = err
		c.logger.Error(err, "failed to fetch TLS certificate",
			"attempt", attempt+1,
			"maxRetries", maxRetries,
			"consecutiveFails", c.consecutiveFails+1)

		// Update health on each failed attempt
		c.consecutiveFails++
		if c.consecutiveFails >= maxRetries && c.healthy {
			c.healthy = false
			c.logger.Error(nil, "default domain client marked UNHEALTHY after consecutive failed attempts",
				"consecutiveFails", c.consecutiveFails,
				"maxRetries", maxRetries)
		}
	}

	// All retries failed
	c.logger.Error(lastErr, "exhausted all retries fetching TLS certificate", "maxRetries", maxRetries)
	return nil, fmt.Errorf("failed to fetch TLS certificate after %d attempts: %w", maxRetries, lastErr)
}

// refreshLoop periodically refreshes the certificate cache
func (c *CachedClient) refreshLoop() {
	// Add initial jitter to avoid thundering herd on startup
	initialDelay := time.Duration(rand.Int63n(int64(initialJitter)))
	c.logger.Info("starting background certificate refresh loop", "initialDelay", initialDelay.Truncate(time.Second).String())

	select {
	case <-c.ctx.Done():
		c.logger.Info("certificate refresh loop cancelled before initial fetch")
		return
	case <-time.After(initialDelay):
	}

	// Initial fetch
	c.logger.Info("performing initial certificate fetch")
	c.mu.Lock()
	_, err := c.fetchWithRetryLocked(c.ctx)
	c.mu.Unlock()
	if err != nil {
		c.logger.Error(err, "initial certificate fetch failed, will retry on next refresh")
	}

	// Periodic refresh
	for {
		c.mu.Lock()
		nextRefresh := c.cacheExp
		c.mu.Unlock()

		// If cache not set yet, use a default interval
		if nextRefresh.IsZero() {
			nextRefresh = time.Now().Add(c.opts.CacheTTL)
		}

		waitDuration := time.Until(nextRefresh)
		if waitDuration < 0 {
			waitDuration = 0
		}

		c.logger.Info("scheduling next background certificate refresh",
			"wait", waitDuration.Truncate(time.Second).String(),
			"refreshAt", nextRefresh.UTC().Format(time.RFC3339))

		select {
		case <-c.ctx.Done():
			c.logger.Info("stopping background certificate refresh loop")
			return
		case <-time.After(waitDuration):
			c.logger.Info("performing scheduled background certificate refresh")
			c.mu.Lock()
			_, err := c.fetchWithRetryLocked(c.ctx)
			c.mu.Unlock()
			if err != nil {
				c.logger.Error(err, "scheduled background certificate refresh failed")
			}
		}
	}
}
