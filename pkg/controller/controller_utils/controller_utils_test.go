package controller_utils

import (
	"github.com/Azure/aks-app-routing-operator/pkg/controller/testutils"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMetricsName(t *testing.T) {
	cn1 := ControllerName{"SomeFakeControllerName"}
	cn2 := ControllerName{"Some", "Controller", "Name"}
	cn3 := ControllerName{" SomeName", "Entered  ", "poorly"}

	metricName1 := cn1.MetricsName()
	metricName2 := cn2.MetricsName()
	metricName3 := cn3.MetricsName()

	require.True(t, testutils.IsPrometheusBestPracticeName(metricName1))
	require.True(t, testutils.IsPrometheusBestPracticeName(metricName2))
	require.True(t, testutils.IsPrometheusBestPracticeName(metricName3))
}
