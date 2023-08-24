package controllername

import (
	"strings"
	"unicode"
)

const (
	metricsNameDelimiter = "_"
	loggerNameDelimiter  = "-"
)

type ControllerNamer interface {
	MetricsName() string
	LoggerName() string
}

// controllerName ex. {"My","Controller", "Name"} ->  MyControllerName
type controllerName []string

func NewControllerName(name []string) controllerName {
	cn := make(controllerName, len(name))

	for i, w := range name {
		cn[i] = strip(strings.ToLower(w))

	}
	return cn
}

// Strip removes spaces and non letters
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
