package controllername

import (
	"regexp"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/require"
)

func TestMetricsName(t *testing.T) {

	cn1 := New("SomeFakeControllerName")
	cn2 := New("Some", "Controller", "Name")
	cn3 := New(" SomeName", "Entered  ", "poorly")
	cn4 := New("Some Spaces")
	cn5 := New("Too  Many       Spaces")
	cn6 := New("special!@characters")

	metricName1 := cn1.MetricsName()
	metricName2 := cn2.MetricsName()
	metricName3 := cn3.MetricsName()
	metricName4 := cn4.MetricsName()
	metricName5 := cn5.MetricsName()
	metricName6 := cn6.MetricsName()

	require.True(t, isPrometheusBestPracticeName(metricName1))
	require.True(t, isPrometheusBestPracticeName(metricName2))
	require.True(t, isPrometheusBestPracticeName(metricName3))
	require.True(t, isPrometheusBestPracticeName(metricName4))
	require.True(t, isPrometheusBestPracticeName(metricName5))
	require.True(t, isPrometheusBestPracticeName(metricName6))

}

func TestLoggerName(t *testing.T) {
	cn1 := New("SomeFakeControllerName")
	cn2 := New("Some", "Controller", "Name")
	cn3 := New(" SomeName", "Entered  ", "poorly")
	cn4 := New("Some Spaces")
	cn5 := New("Too  Many       Spaces")
	cn6 := New("special!@characters")

	loggerName1 := cn1.LoggerName()
	loggerName2 := cn2.LoggerName()
	loggerName3 := cn3.LoggerName()
	loggerName4 := cn4.LoggerName()
	loggerName5 := cn5.LoggerName()
	loggerName6 := cn6.LoggerName()

	require.True(t, isBestPracticeLoggerName(loggerName1))
	require.True(t, isBestPracticeLoggerName(loggerName2))
	require.True(t, isBestPracticeLoggerName(loggerName3))
	require.True(t, isBestPracticeLoggerName(loggerName4))
	require.True(t, isBestPracticeLoggerName(loggerName5))
	require.True(t, isBestPracticeLoggerName(loggerName6))
}

func TestControllerNameString(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "single word",
			input:    []string{"SomeFakeControllerName"},
			expected: "somefakecontrollername",
		},
		{
			name:     "multiple words",
			input:    []string{"Some", "Controller", "Name"},
			expected: "some controller name",
		},
		{
			name:     "multiple words with spaces",
			input:    []string{" SomeName", "Entered  ", "poorly"},
			expected: "somename entered poorly",
		},
		{
			name:     "multiple words with characters",
			input:    []string{"special!@characters", "and", "numbers123"},
			expected: "specialcharacters and numbers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cn := New(test.input[0], test.input[1:]...)
			require.Equal(t, test.expected, cn.String())
		})
	}
}

func TestStrip(t *testing.T) {
	str := "a *&b   c "
	striped := clean(str)

	require.Equal(t, striped, "abc")
}

// isPrometheusBestPracticeName - function returns true if the name given matches best practices for prometheus name, i.e. snake_case
func isPrometheusBestPracticeName(controllerName string) bool {
	pattern := "^[a-z]+(_[a-z]+)*$"
	match, _ := regexp.MatchString(pattern, controllerName)

	return match
}

// isBestPracticeLoggerName - function returns true if the name given matches best practices for prometheus name, i.e. kebab-case
func isBestPracticeLoggerName(controllerName string) bool {
	pattern := "^[a-z]+(-[a-z]+)*$"
	match, _ := regexp.MatchString(pattern, controllerName)

	return match
}

func TestAddToLogger(t *testing.T) {
	cn := New("Fake", "Controller")
	logger := testr.New(t)
	cn.AddToLogger(logger)
	require.NotNil(t, logger)
}
