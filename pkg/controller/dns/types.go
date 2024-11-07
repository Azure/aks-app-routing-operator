package dns

import (
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type instance struct {
	config    *manifests.ExternalDNSConfig
	resources []client.Object
	action    action
}

type action int

const (
	deploy action = iota
	clean
)

type cleanObj struct {
	resources []client.Object
	labels    map[string]string
}
