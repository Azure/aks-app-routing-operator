package infra

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
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
	GetKubeconfig(ctx context.Context) ([]byte, error)
	GetCluster(ctx context.Context) (*armcontainerservice.ManagedCluster, error)
	GetVnetId(ctx context.Context) (string, error)
	Identifier
}

type containerRegistry interface {
	GetName() string
	Identifier
}

type zone interface {
	GetDnsZone(ctx context.Context) (*armdns.Zone, error)
	GetName() string
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
}

type LoadableProvisioned struct {
	Name              string
	Cluster           arm.ResourceID
	ContainerRegistry arm.ResourceID
	Zones             []arm.ResourceID
	PrivateZones      []arm.ResourceID
	KeyVault          arm.ResourceID
	CertName          string
	ResourceGroup     arm.ResourceID
	SubscriptionId    string
	TenantId          string
}