package testutils

import (
	"github.com/Azure/aks-app-routing-operator/pkg/controller/metrics"
	promDTO "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"testing"
)

func GetErrMetricCount(t *testing.T, controllerName string) float64 {
	errMetric, err := metrics.AppRoutingReconcileErrors.GetMetricWithLabelValues(controllerName)
	require.NoError(t, err)

	metricProto := &promDTO.Metric{}

	err = errMetric.Write(metricProto)
	require.NoError(t, err)

	beforeCount := metricProto.GetCounter().GetValue()
	return beforeCount
}

func GetReconcileMetricCount(t *testing.T, controllerName, label string) float64 {
	errMetric, err := metrics.AppRoutingReconcileTotal.GetMetricWithLabelValues(controllerName, label)
	require.NoError(t, err)

	metricProto := &promDTO.Metric{}

	err = errMetric.Write(metricProto)
	require.NoError(t, err)

	beforeCount := metricProto.GetCounter().GetValue()
	return beforeCount
}
