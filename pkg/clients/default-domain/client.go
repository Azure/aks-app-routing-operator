package defaultdomain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

// Opts contains configuration options for the client
type Opts struct {
	ServerAddress string
}

// Client is a client for the default domain service
type Client struct {
	opts       Opts
	httpClient *http.Client
}

// NewClient creates a new default domain client
func NewClient(opts Opts) *Client {
	return &Client{
		opts:       opts,
		httpClient: &http.Client{},
	}
}

// GetTLSCertificate retrieves a TLS certificate for the specified parameters
func (c *Client) GetTLSCertificate(ctx context.Context, subscriptionID, resourceGroup, clusterName, ccpID string) (*TLSCertificate, error) {
	baseURL, err := url.Parse(c.opts.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("parsing server address: %w", err)
	}

	pathSegments := []string{
		"defaultdomain",
		"subscriptions",
		url.PathEscape(subscriptionID),
		"resourcegroups",
		url.PathEscape(resourceGroup),
		"clusters",
		url.PathEscape(clusterName),
		"ccpid",
		url.PathEscape(ccpID),
		"defaultdomaintls",
	}

	baseURL.Path = "/" + strings.Join(pathSegments, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Inc()
		metrics.DefaultDomainClientErrors.Inc()
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelNotFound).Inc()
			return nil, &util.NotFoundError{Body: string(body)}
		}
		metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Inc()
		metrics.DefaultDomainClientErrors.Inc()
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var cert TLSCertificate
	if err := json.NewDecoder(resp.Body).Decode(&cert); err != nil {
		metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Inc()
		metrics.DefaultDomainClientErrors.Inc()
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelSuccess).Inc()
	return &cert, nil
}
