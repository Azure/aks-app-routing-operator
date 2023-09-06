package infra

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type infras []infra

type infra struct {
	Name   string
	Suffix string
	// ResourceGroup is a unique new resource group name
	// for resources to be provisioned inside
	ResourceGroup, Location string
	McOpts                  []clients.McOpt
}

type Identifier interface {
	GetId() string
}

type cluster interface {
	GetVnetId(ctx context.Context) (string, error)
	Deploy(ctx context.Context, objs []client.Object) error
	Clean(ctx context.Context, objs []client.Object) error
	GetPrincipalId() string
	GetLocation() string
	GetDnsServiceIp() string
	GetCluster(ctx context.Context) (*armcontainerservice.ManagedCluster, error)
	Identifier
}

type containerRegistry interface {
	GetName() string
	BuildAndPush(ctx context.Context, imageName, dockerfilePath string) error
	Identifier
}

type zone interface {
	GetDnsZone(ctx context.Context) (*armdns.Zone, error)
	GetName() string
	GetNameservers() []string
	Identifier
}

type privateZone interface {
	GetDnsZone(ctx context.Context) (*armprivatedns.PrivateZone, error)
	LinkVnet(ctx context.Context, linkName, vnetId string) error
	GetName() string
	Identifier
}

type resourceGroup interface {
	GetName() string
	Identifier
}

type keyVault interface {
	GetId() string
	CreateCertificate(ctx context.Context, name string, dnsnames []string, certOpts ...clients.CertOpt) (*clients.Cert, error)
	AddAccessPolicy(ctx context.Context, objectId string, permissions armkeyvault.Permissions) error
	Identifier
}

type cert interface {
	GetName() string
	GetId() string
}

type Provisioned struct {
	Name              string
	Cluster           cluster
	ContainerRegistry containerRegistry
	Zones             []zone
	PrivateZones      []privateZone
	KeyVault          keyVault
	Cert              cert
	ResourceGroup     resourceGroup
	SubscriptionId    string
	TenantId          string
	E2eImage          string
	OperatorImage     string
}

type LoadableZone struct {
	ResourceId  arm.ResourceID
	Nameservers []string
}

// LoadableProvisioned is a struct that can be used to load a Provisioned struct from a file.
// Ensure that all fields are exported so that they can properly be serialized/deserialized.
type LoadableProvisioned struct {
	Name                                                     string
	Cluster                                                  arm.ResourceID
	ClusterLocation, ClusterDnsServiceIp, ClusterPrincipalId string
	ContainerRegistry                                        arm.ResourceID
	Zones                                                    []LoadableZone
	PrivateZones                                             []arm.ResourceID
	KeyVault                                                 arm.ResourceID
	CertName                                                 string
	CertId                                                   string
	ResourceGroup                                            arm.ResourceID
	SubscriptionId                                           string
	TenantId                                                 string
	E2eImage                                                 string
	OperatorImage                                            string
}
