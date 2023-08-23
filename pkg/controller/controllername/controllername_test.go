package controllername

import (
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
)

func TestMetricsName(t *testing.T) {

	cn1 := NewControllerName([]string{"SomeFakeControllerName"})
	cn2 := NewControllerName([]string{"Some", "Controller", "Name"})
	cn3 := NewControllerName([]string{" SomeName", "Entered  ", "poorly"})
	cn4 := NewControllerName([]string{"Some Spaces"})
	cn5 := NewControllerName([]string{"Too  Many       Spaces"})

	metricName1 := cn1.MetricsName()
	metricName2 := cn2.MetricsName()
	metricName3 := cn3.MetricsName()
	metricName4 := cn4.MetricsName()
	metricName5 := cn5.MetricsName()

	require.True(t, isPrometheusBestPracticeName(metricName1))
	require.True(t, isPrometheusBestPracticeName(metricName2))
	require.True(t, isPrometheusBestPracticeName(metricName3))
	require.True(t, isPrometheusBestPracticeName(metricName4))
	require.True(t, isPrometheusBestPracticeName(metricName5))
}

func TestLoggerName(t *testing.T) {
	cn1 := NewControllerName([]string{"SomeFakeControllerName"})
	cn2 := NewControllerName([]string{"Some", "Controller", "Name"})
	cn3 := NewControllerName([]string{" SomeName", "Entered  ", "poorly"})
	cn4 := NewControllerName([]string{"Some Spaces"})
	cn5 := NewControllerName([]string{"Too  Many       Spaces"})

	loggerName1 := cn1.LoggerName()
	loggerName2 := cn2.LoggerName()
	loggerName3 := cn3.LoggerName()
	loggerName4 := cn4.LoggerName()
	loggerName5 := cn5.LoggerName()

	require.True(t, isBestPracticeLoggerName(loggerName1))
	require.True(t, isBestPracticeLoggerName(loggerName2))
	require.True(t, isBestPracticeLoggerName(loggerName3))
	require.True(t, isBestPracticeLoggerName(loggerName4))
	require.True(t, isBestPracticeLoggerName(loggerName5))
}

// IsPrometheusBestPracticeName - function returns true if the name given matches best practices for prometheus name, i.e. snake_case
func isPrometheusBestPracticeName(controllerName string) bool {
	pattern := "^[a-z]+(_[a-z]+)*$"
	match, _ := regexp.MatchString(pattern, controllerName)

	return match
}

// IsBestPracticeLoggerName - function returns true if the name given matches best practices for prometheus name, i.e. kebab-case
func isBestPracticeLoggerName(controllerName string) bool {
	pattern := "^[a-z]+(-[a-z]+)*$"
	match, _ := regexp.MatchString(pattern, controllerName)

	return match
}
