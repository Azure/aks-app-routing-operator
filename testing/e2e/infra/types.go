package infra

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/Azure/go-autorest/autorest/azure"
)

type AuthType string

const (
	AuthTypeManagedIdentity  AuthType = "" // MSI is the default
	AuthTypeServicePrincipal AuthType = "servicePrincipal"
)

type infras []infra

type infra struct {
	Name   string
	Suffix string
	// ResourceGroup is a unique new resource group name
	// for resources to be provisioned inside
	ResourceGroup, Location string
	McOpts                  []clients.McOpt
	AuthType                AuthType
	ServicePrincipal        *clients.ServicePrincipal
}

type Identifier interface {
	GetId() string
}

type cluster interface {
	GetVnetId(ctx context.Context) (string, error)
	Deploy(ctx context.Context, objs []client.Object) error
	Clean(ctx context.Context, objs []client.Object) error
	GetPrincipalId() string
	GetClientId() string
	GetLocation() string
	GetDnsServiceIp() string
	GetCluster(ctx context.Context) (*armcontainerservice.ManagedCluster, error)
	GetOptions() map[string]struct{}
	GetOidcUrl() string
	Identifier
}

type containerRegistry interface {
	GetName() string
	BuildAndPush(ctx context.Context, imageName, dockerfilePath, dockerFileName string) error
	Identifier
}

type Zone interface {
	GetDnsZone(ctx context.Context) (*armdns.Zone, error)
	GetName() string
	GetNameservers() []string
	Identifier
}

type PrivateZone interface {
	GetDnsZone(ctx context.Context) (*armprivatedns.PrivateZone, error)
	LinkVnet(ctx context.Context, linkName, vnetId string) error
	GetName() string
	Identifier
}

type resourceGroup interface {
	GetName() string
	Cleanup(ctx context.Context) error
	Identifier
}

type keyVault interface {
	GetId() string
	CreateCertificate(ctx context.Context, name string, cnName string, dnsnames []string, certOpts ...clients.CertOpt) (*clients.Cert, error)
	AddAccessPolicy(ctx context.Context, objectId string, permissions armkeyvault.Permissions) error
	Identifier
}

type cert interface {
	GetName() string
	GetId() string
}

// WithCert is a resource with a tls certificate valid for that resource. This is used to bundle DNS Zones
type WithCert[T any] struct {
	Zone T
	Cert cert
}

type Provisioned struct {
	Name              string
	Cluster           cluster
	ContainerRegistry containerRegistry
	Zones             []WithCert[Zone]
	PrivateZones      []WithCert[PrivateZone]
	KeyVault          keyVault
	ResourceGroup     resourceGroup
	SubscriptionId    string
	TenantId          string
	E2eImage          string
	OperatorImage     string
}

type LoadableZone struct {
	ResourceId  azure.Resource
	Nameservers []string
}

type withLoadableCert[T any] struct {
	Zone     T
	CertName string
	CertId   string
}

// LoadableProvisioned is a struct that can be used to load a Provisioned struct from a file.
// Ensure that all fields are exported so that they can properly be serialized/deserialized.
type LoadableProvisioned struct {
	Name                                                                                      string
	Cluster                                                                                   azure.Resource
	ClusterLocation, ClusterDnsServiceIp, ClusterPrincipalId, ClusterClientId, ClusterOidcUrl string
	ClusterOptions                                                                            map[string]struct{}
	ContainerRegistry                                                                         azure.Resource
	Zones                                                                                     []withLoadableCert[LoadableZone]
	PrivateZones                                                                              []withLoadableCert[azure.Resource]
	KeyVault                                                                                  azure.Resource
	ResourceGroup                                                                             arm.ResourceID // rg id is a little weird and can't be correctly parsed by azure.Resource so we have to use arm.ResourceID
	SubscriptionId                                                                            string
	TenantId                                                                                  string
	E2eImage                                                                                  string
	OperatorImage                                                                             string
}
