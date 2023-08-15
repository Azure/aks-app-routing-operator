package main

import (
	"github.com/Azure/aks-app-routing-operator/testing/e2e/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
