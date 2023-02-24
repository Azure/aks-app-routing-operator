// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package ingress

import (
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/controller/common"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reconcileInterval = time.Minute * 3

// NewIngressControllerReconciler creates a reconciler that manages ingress controller resources
func NewIngressControllerReconciler(manager ctrl.Manager, resources []client.Object, name string) error {
	return common.NewResourceReconciler(manager, name+"IngressControllerReconciler", resources, reconcileInterval)
}
