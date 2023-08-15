package test

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
)

type runStrategy int

const (
	// Parallel runs the test at the same time as other tests
	Parallel runStrategy = iota
	// Sequential runs the test alone
	Sequential
)

type Test interface {
	GetName() string
	GetRunStrategy() runStrategy
	Run(ctx context.Context) error
	ShouldRun(infra infra.Provisioned) bool
}

type tests []Test
