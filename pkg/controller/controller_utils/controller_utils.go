package controller_utils

import (
	"strings"
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

func (c ControllerName) clean(delimiter string) ControllerName {
	for i := range c {
		// replace spaces with _
		c[i] = strings.ReplaceAll(c[i], " ", delimiter)
	}
	return c
}

func (c ControllerName) lowercase() ControllerName {
	for i := range c {
		c[i] = strings.ToLower(c[i])
	}
	return c
}

func (c ControllerName) MetricsName() string {
	return strings.Join(c.lowercase().clean(metricsNameDelimiter), metricsNameDelimiter)
}

func (c ControllerName) LoggerName() string {
	return strings.Join(c.lowercase().clean(loggerNameDelimiter), loggerNameDelimiter)
}
