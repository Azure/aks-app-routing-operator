// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"testing"

	"github.com/Azure/aks-app-routing-operator/pkg/config"
)

func TestIngressControllerResources(t *testing.T) {
	// Just prove it doesn't panic
	IngressControllerResources(&config.Config{})
}
