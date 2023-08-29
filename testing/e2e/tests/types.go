package tests

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/infra"
)

type RunStrategy int

const (
	// Parallel runs the test at the same time as other tests
	Parallel RunStrategy = iota
	// Sequential runs the test alone
	Sequential
)

// T is an interface for a single test
type T interface {
	GetName() string
	GetRunStrategy() RunStrategy
	Run(ctx context.Context) error
	ShouldRun(infra infra.Provisioned) bool
}

// Ts is a slice of T
type Ts []T
