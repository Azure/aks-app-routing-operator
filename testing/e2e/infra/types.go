package infra

import (
	"context"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/clients"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
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

type cluster interface {
	GetKubeconfig(ctx context.Context) ([]byte, error)
	GetCluster(ctx context.Context) (*armcontainerservice.ManagedCluster, error)
	GetVnetId(ctx context.Context) (string, error)
}

type containerRegistry interface {
	GetName() string
	GetId() string
}

type zone interface {
	GetDnsZone(ctx context.Context) (*armdns.Zone, error)
	GetName() string
}

type privateZone interface {
	GetDnsZone(ctx context.Context) (*armprivatedns.PrivateZone, error)
	LinkVnet(ctx context.Context, linkName, vnetId string) error
	GetName() string
}

type resourceGroup interface {
	GetName() string
}

type keyVault interface {
	GetId() string
	CreateCertificate(ctx context.Context, name string, dnsnames []string, certOpts ...clients.CertOpt) (*clients.Cert, error)
	AddAccessPolicy(ctx context.Context, objectId string, permissions armkeyvault.Permissions) error
}

type cert interface {
	GetName() string
}

type ProvisionedInfra struct {
	Name              string
	Cluster           cluster
	ContainerRegistry containerRegistry
	Zones             []zone
	PrivateZones      []privateZone
	KeyVault          keyVault
	Cert              cert
	ResourceGroup     resourceGroup
}
