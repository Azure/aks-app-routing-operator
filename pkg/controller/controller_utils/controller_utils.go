package controller_utils

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

type ControllerName []string

func NewControllerName(controllerName []string) ControllerName {
	cn := make(ControllerName, len(controllerName))

	for i, w := range controllerName {
		cn[i] = removeSpace(strings.ToLower(w))

	}
	return cn
}

func removeSpace(s string) string {
	rr := make([]rune, 0, len(s))
	for _, r := range s {
		if !unicode.IsSpace(r) {
			rr = append(rr, r)
		}
	}
	return string(rr)
}

func (c ControllerName) lowercase() ControllerName {
	for i := range c {
		c[i] = strings.ToLower(c[i])
	}
	return c
}

func (c ControllerName) MetricsName() string {
	return strings.Join(c, metricsNameDelimiter)
}

func (c ControllerName) LoggerName() string {
	return strings.Join(c, loggerNameDelimiter)
}
