package defaultdomain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	opts := Opts{
		ServerAddress: "http://localhost:9000",
	}

	client := NewClient(opts)

	require.NotNil(t, client)
	assert.Equal(t, opts, client.opts)
	assert.NotNil(t, client.httpClient)
}

func TestClient_GetTLSCertificate_Success(t *testing.T) {
	expiresOn := time.Now().Add(24 * time.Hour)
	expectedCert := &TLSCertificate{
		Key:       []byte("test-key"),
		Cert:      []byte("test-cert"),
		ExpiresOn: &expiresOn,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/defaultdomain/subscriptions/sub-123/resourcegroups/rg-test/clusters/cluster-1/ccpid/ccp-456/defaultdomaintls", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	// Get initial metric value
	var before dto.Metric
	_ = metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelSuccess).Write(&before)
	initialCount := before.GetCounter().GetValue()

	client := NewClient(Opts{ServerAddress: server.URL})
	cert, err := client.GetTLSCertificate(context.Background(), "sub-123", "rg-test", "cluster-1", "ccp-456")

	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, expectedCert.Key, cert.Key)
	assert.Equal(t, expectedCert.Cert, cert.Cert)
	assert.NotNil(t, cert.ExpiresOn)

	// Verify success metric
	var m dto.Metric
	err = metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelSuccess).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, initialCount+1, m.GetCounter().GetValue())
}

func TestClient_GetTLSCertificate_URLEscaping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that special characters are properly escaped in the URL path
		// Note: url.PathEscape doesn't escape + and = as they're valid path characters
		assert.Equal(t, "/defaultdomain/subscriptions/sub%2F123/resourcegroups/rg%20test/clusters/cluster+name/ccpid/ccp=456/defaultdomaintls", r.URL.Path)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&TLSCertificate{})
	}))
	defer server.Close()

	client := NewClient(Opts{ServerAddress: server.URL})
	_, err := client.GetTLSCertificate(context.Background(), "sub/123", "rg test", "cluster+name", "ccp=456")

	require.NoError(t, err)
}

func TestClient_GetTLSCertificate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	// Get initial metric values
	var beforeTotal, beforeErrors dto.Metric
	_ = metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Write(&beforeTotal)
	initialTotal := beforeTotal.GetCounter().GetValue()
	_ = metrics.DefaultDomainClientErrors.Write(&beforeErrors)
	initialErrors := beforeErrors.GetCounter().GetValue()

	client := NewClient(Opts{ServerAddress: server.URL})
	cert, err := client.GetTLSCertificate(context.Background(), "sub-123", "rg-test", "cluster-1", "ccp-456")

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "unexpected status code 500")
	assert.Contains(t, err.Error(), "internal server error")

	// Verify error metric
	var m dto.Metric
	err = metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelError).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, initialTotal+1, m.GetCounter().GetValue())

	err = metrics.DefaultDomainClientErrors.Write(&m)
	require.NoError(t, err)
	assert.Equal(t, initialErrors+1, m.GetCounter().GetValue())
}

func TestClient_GetTLSCertificate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(Opts{ServerAddress: server.URL})
	cert, err := client.GetTLSCertificate(context.Background(), "sub-123", "rg-test", "cluster-1", "ccp-456")

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "decoding response")
}

func TestClient_GetTLSCertificate_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&TLSCertificate{})
	}))
	defer server.Close()

	client := NewClient(Opts{ServerAddress: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cert, err := client.GetTLSCertificate(ctx, "sub-123", "rg-test", "cluster-1", "ccp-456")

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "executing request")
}

func TestClient_GetTLSCertificate_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	// Get initial metric value
	var before dto.Metric
	_ = metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelNotFound).Write(&before)
	initialCount := before.GetCounter().GetValue()

	client := NewClient(Opts{ServerAddress: server.URL})
	cert, err := client.GetTLSCertificate(context.Background(), "sub-123", "rg-test", "cluster-1", "ccp-456")

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.True(t, util.IsNotFound(err))
	assert.Contains(t, err.Error(), "not found: not found")

	// Verify not found metric
	var m dto.Metric
	err = metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelNotFound).Write(&m)
	require.NoError(t, err)
	assert.Equal(t, initialCount+1, m.GetCounter().GetValue())
}

func TestClient_GetTLSCertificate_InvalidServerAddress(t *testing.T) {
	client := NewClient(Opts{ServerAddress: "http://nonexistent-server-12345.local"})
	cert, err := client.GetTLSCertificate(context.Background(), "sub-123", "rg-test", "cluster-1", "ccp-456")

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "executing request")
}
