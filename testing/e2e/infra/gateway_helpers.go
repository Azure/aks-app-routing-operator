package infra

import "fmt"

const (
	// NumGatewayZones is the number of managed identity zones to provision for gateway tests.
	// HTTP and GRPC top-level entries each get a kind-disjoint slice (see kindZoneSlice in the
	// gateway suite) — splitting prevents the two ClusterExternalDNS instances (which share
	// --txt-owner-id=ClusterUid and the same --source=gateway-{http,grpc}route flags) from
	// racing on deletes in shared zones, which would non-deterministically erase each other's
	// records and break the "wait for delete log" assertion. Capped at 3 to stay under Azure's
	// 20-FIC-per-UAMI limit (each cluster-scoped namespace per zone × 4 prefixes is one FIC).
	NumGatewayZones    = 3
	NumHTTPGatewayZones = 2 // first NumHTTPGatewayZones zones go to HTTP entries
	NumGRPCGatewayZones = NumGatewayZones - NumHTTPGatewayZones

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

	// GRPC variants — kept in their own namespaces so HTTP and GRPC top-level
	// gateway test entries can run in parallel without colliding on resources.

	// GrpcGatewayClusterNsPrefix is the prefix for cluster-scoped GRPC gateway test namespaces (one per zone)
	GrpcGatewayClusterNsPrefix = "grpc-gateway-cluster-ns"
	// GrpcGatewayClusterSaName is the service account name used in cluster-scoped GRPC gateway test namespaces
	GrpcGatewayClusterSaName = "grpc-gateway-cluster-sa"

	// GrpcGatewayNsPublic is the namespace for namespace-scoped GRPC gateway tests with public zones
	GrpcGatewayNsPublic = "grpc-gateway-wi-ns"
	// GrpcGatewayNsPrivate is the namespace for namespace-scoped GRPC gateway tests with private zones
	GrpcGatewayNsPrivate = "private-grpc-gateway-wi-ns"
	// GrpcGatewayNsSaName is the service account name used in namespace-scoped GRPC gateway test namespaces
	GrpcGatewayNsSaName = "grpc-gateway-wi-sa"

	// GrpcFilterClusterNsPrefix is the prefix for cluster-scoped GRPC filter test namespaces (one per zone)
	GrpcFilterClusterNsPrefix = "grpc-filter-cluster-ns"
	// GrpcFilterClusterSaName is the service account name used in cluster-scoped GRPC filter test namespaces
	GrpcFilterClusterSaName = "grpc-filter-cluster-sa"

	// GrpcFilterNs is the namespace for namespace-scoped GRPC filter tests
	GrpcFilterNs = "grpc-filter-ns"
	// GrpcFilterNsSaName is the service account name used in namespace-scoped GRPC filter test namespace
	GrpcFilterNsSaName = "grpc-filter-sa"
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

// GrpcGatewayClusterNsName returns the namespace name for a cluster-scoped GRPC gateway test at the given zone index
func GrpcGatewayClusterNsName(zoneIndex int) string {
	return fmt.Sprintf("%s-%d", GrpcGatewayClusterNsPrefix, zoneIndex)
}

// GrpcFilterClusterNsName returns the namespace name for a cluster-scoped GRPC filter test at the given zone index
func GrpcFilterClusterNsName(zoneIndex int) string {
	return fmt.Sprintf("%s-%d", GrpcFilterClusterNsPrefix, zoneIndex)
}

// GenerateGatewayFederatedNamespaces generates all FederatedNamespace entries needed for gateway tests
func GenerateGatewayFederatedNamespaces() []FederatedNamespace {
	var namespaces []FederatedNamespace

	// Namespace-scoped test namespaces (single namespace for all zones)
	namespaces = append(namespaces,
		FederatedNamespace{Namespace: GatewayNsPublic, ServiceAccount: GatewayNsSaName},
		FederatedNamespace{Namespace: GatewayNsPrivate, ServiceAccount: GatewayNsSaName},
		FederatedNamespace{Namespace: FilterNs, ServiceAccount: FilterNsSaName},
		FederatedNamespace{Namespace: GrpcGatewayNsPublic, ServiceAccount: GrpcGatewayNsSaName},
		FederatedNamespace{Namespace: GrpcGatewayNsPrivate, ServiceAccount: GrpcGatewayNsSaName},
		FederatedNamespace{Namespace: GrpcFilterNs, ServiceAccount: GrpcFilterNsSaName},
	)

	// Cluster-scoped test namespaces (one per zone)
	for i := 0; i < NumGatewayZones; i++ {
		namespaces = append(namespaces,
			FederatedNamespace{Namespace: GatewayClusterNsName(i), ServiceAccount: GatewayClusterSaName},
			FederatedNamespace{Namespace: FilterClusterNsName(i), ServiceAccount: FilterClusterSaName},
			FederatedNamespace{Namespace: GrpcGatewayClusterNsName(i), ServiceAccount: GrpcGatewayClusterSaName},
			FederatedNamespace{Namespace: GrpcFilterClusterNsName(i), ServiceAccount: GrpcFilterClusterSaName},
		)
	}

	return namespaces
}
