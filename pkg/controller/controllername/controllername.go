package controllername

import (
	"strings"
	"unicode"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	metricsNameDelimiter = "_"
	loggerNameDelimiter  = "-"
)

// ControllerNamer is an interface that returns the name of the controller in all necessary forms
type ControllerNamer interface {
	// String returns the name of the controller in a human-readable form
	String() string
	// MetricsName returns the name of the controller in a form that can be used for Prometheus metrics, specifically as a Prometheus label https://prometheus.io/docs/practices/naming/#labels
	MetricsName() string
	// LoggerName returns the name of the controller in a form that can be used for logr logger naming
	LoggerName() string
	// AddToLogger adds controller name fields to the logger then returns the logger with the added fields
	AddToLogger(l logr.Logger) logr.Logger
	// AddToController adds the controller name to the controller builder then returns the builder with the added name. This is useful for naming managed controllers from controller-runtime
	AddToController(blder *builder.Builder, l logr.Logger) *builder.Builder
}

// controllerName ex. {"My","Controller", "Name"} ->  MyControllerName
type controllerName []string

// New returns a new controllerName after taking each word of the controller name as a separate argument
func New(firstWord string, additionalWords ...string) controllerName { // using a non-variadic for the first word makes it impossible to accidentally pass no arguments in. Accepting variadic versus slices also helps with not accepting empty slices and is easier to use
	cn := make(controllerName, 0, len(additionalWords)+1)
	for _, w := range append([]string{firstWord}, additionalWords...) {
		cleaned := clean(w)
		if cleaned != "" {
			cn = append(cn, cleaned)
		}
	}

	return cn
}

// clean removes spaces and non letters and lowercases
func clean(s string) string {
	rr := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsLetter(r) {
			rr = append(rr, unicode.ToLower(r))
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

func (c controllerName) AddToLogger(l logr.Logger) logr.Logger {
	return l.
		WithName(c.LoggerName()).
		WithValues("controller", c.String()).
		WithValues("controllerMetricsName", c.MetricsName()) // include metrics name, so we can automate creating queries that check Logs based on alerts
}

func (c controllerName) AddToController(blder *builder.Builder, l logr.Logger) *builder.Builder {
	return blder.
		Named(c.MetricsName()).
		WithLogConstructor(func(req *reconcile.Request) logr.Logger {
			logger := c.AddToLogger(l)
			if req != nil {
				logger.WithValues(
					"namespace", req.Namespace,
					"name", req.Name,
				)
			}
			return logger
		})
}
