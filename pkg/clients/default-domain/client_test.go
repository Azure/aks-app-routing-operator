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
	"github.com/go-logr/logr"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	opts := Opts{
		ServerAddress: "http://localhost:9000",
	}

	client := NewClient(opts, logr.Discard())

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
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	// Get initial metric value
	var before dto.Metric
	_ = metrics.DefaultDomainClientCallsTotal.WithLabelValues(metrics.LabelSuccess).Write(&before)
	initialCount := before.GetCounter().GetValue()

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())

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

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())

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

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())

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

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cert, err := client.GetTLSCertificate(ctx)

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

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())

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
	client := NewClient(Opts{ServerAddress: "http://nonexistent-server-12345.local"}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())

	require.Error(t, err)
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "executing request")
}

// TestClient_CertExpiryMetric_SetOnSuccess verifies the expiry gauge is set when ExpiresOn is provided
func TestClient_CertExpiryMetric_SetOnSuccess(t *testing.T) {
	expiresOn := time.Now().Add(90 * 24 * time.Hour) // 90 days from now
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

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Verify the expiry gauge is set to approximately 90 days in seconds
	var m dto.Metric
	err = metrics.DefaultDomainCertExpirySeconds.Write(&m)
	require.NoError(t, err)

	gaugeValue := m.GetGauge().GetValue()
	expectedSeconds := (90 * 24 * time.Hour).Seconds()
	// Allow 60 seconds of tolerance for test execution time
	assert.InDelta(t, expectedSeconds, gaugeValue, 60, "expiry gauge should be approximately 90 days in seconds")
	assert.Greater(t, gaugeValue, float64(0), "expiry gauge should be positive for a future cert")
}

// TestClient_CertExpiryMetric_ExpiredCert verifies the expiry gauge is negative for an already-expired cert
func TestClient_CertExpiryMetric_ExpiredCert(t *testing.T) {
	expiresOn := time.Now().Add(-2 * 24 * time.Hour) // 2 days ago
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

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert)

	// Verify the expiry gauge is negative for an expired cert
	var m dto.Metric
	err = metrics.DefaultDomainCertExpirySeconds.Write(&m)
	require.NoError(t, err)

	gaugeValue := m.GetGauge().GetValue()
	assert.Less(t, gaugeValue, float64(0), "expiry gauge should be negative for an expired cert")

	expectedSeconds := (-2 * 24 * time.Hour).Seconds()
	assert.InDelta(t, expectedSeconds, gaugeValue, 60, "expiry gauge should be approximately -2 days in seconds")
}

// TestClient_CertExpiryMetric_NilExpiresOn verifies the gauge is not updated when ExpiresOn is nil
func TestClient_CertExpiryMetric_NilExpiresOn(t *testing.T) {
	expectedCert := &TLSCertificate{
		Key:  []byte("test-key"),
		Cert: []byte("test-cert"),
		// ExpiresOn intentionally nil
	}

	// Set gauge to a known value so we can verify it doesn't change
	metrics.DefaultDomainCertExpirySeconds.Set(12345)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedCert)
	}))
	defer server.Close()

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())
	cert, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Nil(t, cert.ExpiresOn)

	// Verify gauge was not changed
	var m dto.Metric
	err = metrics.DefaultDomainCertExpirySeconds.Write(&m)
	require.NoError(t, err)
	assert.Equal(t, float64(12345), m.GetGauge().GetValue(), "gauge should not change when ExpiresOn is nil")
}

// TestClient_CertExpiryMetric_UpdatesOnEachCall verifies the expiry gauge updates on each successful fetch
func TestClient_CertExpiryMetric_UpdatesOnEachCall(t *testing.T) {
	callCount := 0
	expiresOn1 := time.Now().Add(30 * 24 * time.Hour) // 30 days
	expiresOn2 := time.Now().Add(60 * 24 * time.Hour) // 60 days

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		cert := &TLSCertificate{
			Key:  []byte("test-key"),
			Cert: []byte("test-cert"),
		}
		if callCount == 1 {
			cert.ExpiresOn = &expiresOn1
		} else {
			cert.ExpiresOn = &expiresOn2
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(cert)
	}))
	defer server.Close()

	client := NewClient(Opts{ServerAddress: server.URL}, logr.Discard())

	// First fetch: 30 days
	_, err := client.GetTLSCertificate(context.Background())
	require.NoError(t, err)

	var m dto.Metric
	err = metrics.DefaultDomainCertExpirySeconds.Write(&m)
	require.NoError(t, err)
	firstValue := m.GetGauge().GetValue()
	assert.InDelta(t, (30 * 24 * time.Hour).Seconds(), firstValue, 60)

	// Second fetch: 60 days
	_, err = client.GetTLSCertificate(context.Background())
	require.NoError(t, err)

	err = metrics.DefaultDomainCertExpirySeconds.Write(&m)
	require.NoError(t, err)
	secondValue := m.GetGauge().GetValue()
	assert.InDelta(t, (60 * 24 * time.Hour).Seconds(), secondValue, 60)

	// Second value should be larger than first
	assert.Greater(t, secondValue, firstValue, "expiry gauge should update on each successful fetch")
}
