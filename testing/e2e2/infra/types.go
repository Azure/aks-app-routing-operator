package infra

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e2/clients"
)

type Infras []Infra

type Infra struct {
	Name   string
	Suffix string
	// ResourceGroup is a unique new resource group name
	// for resources to be provisioned inside
	ResourceGroup, Location string
	McOpts                  []clients.McOpt
}
