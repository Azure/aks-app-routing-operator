package utils

import "fmt"

const (
	// NumGatewayZones is the number of managed identity zones to provision and test for gateway tests
	NumGatewayZones = 3

	// GatewayClusterNsPrefix is the prefix for cluster-scoped gateway test namespaces (one per zone)
	GatewayClusterNsPrefix = "gateway-cluster-ns"
	// GatewayClusterSaName is the service account name used in cluster-scoped gateway test namespaces
	GatewayClusterSaName = "gateway-cluster-sa"

	// GatewayNsPublic is the namespace for namespace-scoped gateway tests with public zones
	GatewayNsPublic = "gateway-wi-ns"
	// GatewayNsPrivate is the namespace for namespace-scoped gateway tests with private zones
	GatewayNsPrivate = "private-gateway-wi-ns"
	// GatewayNsSaName is the service account name used in namespace-scoped gateway test namespaces
	GatewayNsSaName = "gateway-wi-sa"

	// FilterClusterNsPrefix is the prefix for cluster-scoped filter test namespaces (one per zone)
	FilterClusterNsPrefix = "filter-cluster-ns"
	// FilterClusterSaName is the service account name used in cluster-scoped filter test namespaces
	FilterClusterSaName = "filter-cluster-sa"

	// FilterNs is the namespace for namespace-scoped filter tests
	FilterNs = "filter-ns"
	// FilterNsSaName is the service account name used in namespace-scoped filter test namespace
	FilterNsSaName = "filter-sa"
)

// FederatedNamespace represents a namespace and service account pair to federate with the managed identity
type FederatedNamespace struct {
	Namespace      string
	ServiceAccount string
}

// GatewayClusterNsName returns the namespace name for a cluster-scoped gateway test at the given zone index
func GatewayClusterNsName(zoneIndex int) string {
	return fmt.Sprintf("%s-%d", GatewayClusterNsPrefix, zoneIndex)
}

// FilterClusterNsName returns the namespace name for a cluster-scoped filter test at the given zone index
func FilterClusterNsName(zoneIndex int) string {
	return fmt.Sprintf("%s-%d", FilterClusterNsPrefix, zoneIndex)
}

// GenerateGatewayFederatedNamespaces generates all FederatedNamespace entries needed for gateway tests
func GenerateGatewayFederatedNamespaces() []FederatedNamespace {
	var namespaces []FederatedNamespace

	// Namespace-scoped test namespaces (single namespace for all zones)
	namespaces = append(namespaces,
		FederatedNamespace{Namespace: GatewayNsPublic, ServiceAccount: GatewayNsSaName},
		FederatedNamespace{Namespace: GatewayNsPrivate, ServiceAccount: GatewayNsSaName},
		FederatedNamespace{Namespace: FilterNs, ServiceAccount: FilterNsSaName},
	)

	// Cluster-scoped test namespaces (one per zone)
	for i := 0; i < NumGatewayZones; i++ {
		namespaces = append(namespaces,
			FederatedNamespace{Namespace: GatewayClusterNsName(i), ServiceAccount: GatewayClusterSaName},
			FederatedNamespace{Namespace: FilterClusterNsName(i), ServiceAccount: FilterClusterSaName},
		)
	}

	return namespaces
}
