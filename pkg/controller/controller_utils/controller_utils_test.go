package controller_utils

import (
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
)

func TestMetricsName(t *testing.T) {
	cn1 := ControllerName{"SomeFakeControllerName"}
	cn2 := ControllerName{"Some", "Controller", "Name"}
	cn3 := ControllerName{" SomeName", "Entered  ", "poorly"}

	metricName1 := cn1.MetricsName()
	metricName2 := cn2.MetricsName()
	metricName3 := cn3.MetricsName()

	require.True(t, isPrometheusBestPracticeName(metricName1))
	require.True(t, isPrometheusBestPracticeName(metricName2))
	require.True(t, isPrometheusBestPracticeName(metricName3))
}

func TestLoggerName(t *testing.T) {
	cn1 := ControllerName{"SomeFakeControllerName"}
	cn2 := ControllerName{"Some", "Controller", "Name"}
	cn3 := ControllerName{" SomeName", "Entered  ", "poorly"}

	metricName1 := cn1.LoggerName()
	metricName2 := cn2.LoggerName()
	metricName3 := cn3.LoggerName()

	require.True(t, isBestPracticeLoggerName(metricName1))
	require.True(t, isBestPracticeLoggerName(metricName2))
	require.True(t, isBestPracticeLoggerName(metricName3))
}

func TestIsPrometheusBestPracticeName(t *testing.T) {
	notSnakeCase := "obviouslyNotSnakeCase"
	simpleSnakeCase := "snake_case"
	complexSnakeCase := "complex_snake_case"
	leadingSlash := "_leading_slash"
	trailingSlash := "trailing_slash_"

	require.False(t, isPrometheusBestPracticeName(notSnakeCase))
	require.True(t, isPrometheusBestPracticeName(simpleSnakeCase))
	require.True(t, isPrometheusBestPracticeName(complexSnakeCase))
	require.False(t, isPrometheusBestPracticeName(leadingSlash))
	require.False(t, isPrometheusBestPracticeName(trailingSlash))
}

func TestIsBestLoggerName(t *testing.T) {
	notSnakeCase := "obviouslyNotKebabCase"
	simpleSnakeCase := "snake-case"
	complexSnakeCase := "complex-kebab-case"
	leadingSlash := "-leading-slash"
	trailingSlash := "trailing-slash-"

	require.False(t, isPrometheusBestPracticeName(notSnakeCase))
	require.True(t, isPrometheusBestPracticeName(simpleSnakeCase))
	require.True(t, isPrometheusBestPracticeName(complexSnakeCase))
	require.False(t, isPrometheusBestPracticeName(leadingSlash))
	require.False(t, isPrometheusBestPracticeName(trailingSlash))
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
