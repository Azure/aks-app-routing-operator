package controllername

import (
	"strings"
	"unicode"

	"github.com/go-logr/logr"
)

const (
	metricsNameDelimiter = "_"
	loggerNameDelimiter  = "-"
)

// ControllerNamer is an interface that returns the name of the controller in all necessary forms
type ControllerNamer interface {
	// String returns the name of the controller in a human readable form
	String() string
	// MetricsName returns the name of the controller in a form that can be used for Prometheus metrics, specifically as a Prometheus label https://prometheus.io/docs/practices/naming/#labels
	MetricsName() string
	// LoggerName returns the name of the controller in a form that can be used for logr logger naming
	LoggerName() string
}

// controllerName ex. {"My","Controller", "Name"} ->  MyControllerName
type controllerName []string

// NewControllerName returns a new controllerName after taking input a slice of each word in the controller name
func NewControllerName(name []string) controllerName {
	cn := make(controllerName, len(name))

	for i, w := range name {
		cn[i] = strip(strings.ToLower(w))

	}
	return cn
}

// strip removes spaces and non letters
func strip(s string) string {
	rr := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsLetter(r) {
			rr = append(rr, r)
		}
	}
	return string(rr)
}

func (c controllerName) MetricsName() string {
	return strings.Join(c, metricsNameDelimiter)
}

func (c controllerName) LoggerName() string {
	return strings.Join(c, loggerNameDelimiter)
}

func (c controllerName) String() string {
	return strings.Join(c, " ")
}

// AddToLogger adds controller name fields to the logger then returns the logger with the added fields
func AddToLogger(l logr.Logger, cn ControllerNamer) logr.Logger {
	return l.
		WithName(cn.LoggerName()).
		WithValues("controller", cn.String()).
		WithValues("controllerMetricsName", cn.MetricsName()) // include metrics name so we can automate creating queries that check Logs based on alerts
}
