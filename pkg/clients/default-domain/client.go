package defaultdomain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/go-logr/logr"
)

// Opts contains configuration options for the client
type Opts struct {
	ServerAddress string
}

// Client is a client for the default domain service
type Client struct {
	opts       Opts
	httpClient *http.Client
	logger     logr.Logger
}

// NewClient creates a new default domain client
func NewClient(opts Opts, logger logr.Logger) *Client {
	return &Client{
		opts:       opts,
		httpClient: &http.Client{},
		// tag every log line from this client with the server it's talking to so
		// the source of a fetch is unambiguous when reading logs.
		logger: logger.WithValues("serverAddress", opts.ServerAddress),
	}
}

// GetTLSCertificate retrieves a TLS certificate from the configured server address
func (c *Client) GetTLSCertificate(ctx context.Context) (*TLSCertificate, error) {
	c.logger.Info("requesting TLS certificate from default domain service")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opts.ServerAddress, nil)
	if err != nil {
		c.logger.Error(err, "failed to build request to default domain service")
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Inc()
		metrics.DefaultDomainClientErrors.Inc()
		c.logger.Error(err, "request to default domain service failed (could not reach service)")
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	c.logger.Info("received response from default domain service", "statusCode", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelNotFound).Inc()
			c.logger.Info("default domain service has no certificate yet (404)", "responseBody", string(body))
			return nil, &util.NotFoundError{Body: string(body)}
		}
		metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Inc()
		metrics.DefaultDomainClientErrors.Inc()
		c.logger.Error(nil, "default domain service returned unexpected status", "statusCode", resp.StatusCode, "responseBody", string(body))
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var cert TLSCertificate
	if err := json.NewDecoder(resp.Body).Decode(&cert); err != nil {
		metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Inc()
		metrics.DefaultDomainClientErrors.Inc()
		c.logger.Error(err, "failed to decode response body from default domain service")
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Update certificate expiry metric and log status
	if cert.ExpiresOn != nil {
		timeUntilExpiry := time.Until(*cert.ExpiresOn)
		metrics.DefaultDomainCertExpirySeconds.Set(timeUntilExpiry.Seconds())
		c.logger.Info("successfully fetched TLS certificate from default domain service",
			"certBytes", len(cert.Cert),
			"keyBytes", len(cert.Key),
			"expiresOn", cert.ExpiresOn.UTC().Format(time.RFC3339),
			"timeUntilExpiry", timeUntilExpiry.Truncate(time.Second).String(),
		)
	} else {
		c.logger.Error(nil, "successfully fetched TLS certificate but ExpiresOn field is nil, unable to track expiry",
			"certBytes", len(cert.Cert),
			"keyBytes", len(cert.Key),
		)
	}

	metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelSuccess).Inc()
	return &cert, nil
}
