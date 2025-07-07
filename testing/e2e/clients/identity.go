package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/go-autorest/autorest/azure"
)

// managedIdentity represents an Azure Managed Identity
type managedIdentity struct {
	name           string
	resourceGroup  string
	subscriptionID string
	clientID       string
	principalID    string
	id             string
}

// LoadManagedIdentity loads a managed identity from its ID and client ID
func LoadManagedIdentity(id azure.Resource, clientID, principalID string) *managedIdentity {
	return &managedIdentity{
		id:             id.String(),
		clientID:       clientID,
		name:           id.ResourceName,
		resourceGroup:  id.ResourceGroup,
		principalID:    principalID,
		subscriptionID: id.SubscriptionID,
	}
}

// NewManagedIdentity creates a new ManagedIdentity client
func NewManagedIdentity(ctx context.Context, subscriptionID, resourceGroup, name, location string) (*managedIdentity, error) {
	lgr := logger.FromContext(ctx).With("name", name, "resourceGroup", resourceGroup, "subscriptionID", subscriptionID, "location", location)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create managed identity")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting azure credential: %w", err)
	}

	client, err := armmsi.NewUserAssignedIdentitiesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating managed identity client: %w", err)
	}

	resp, err := client.CreateOrUpdate(ctx, resourceGroup, name, armmsi.Identity{
		Name:     to.Ptr(name),
		Location: &location,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("creating managed identity: %w", err)
	}

	// guard against things that should be impossible
	if resp.ID == nil {
		return nil, fmt.Errorf("managed identity ID is nil")
	}

	if resp.Properties.ClientID == nil {
		return nil, fmt.Errorf("managed identity client ID is nil")
	}

	if resp.Properties.PrincipalID == nil {
		return nil, fmt.Errorf("managed identity principal ID is nil")
	}

	return &managedIdentity{
		name:           name,
		resourceGroup:  resourceGroup,
		subscriptionID: subscriptionID,
		clientID:       *resp.Properties.ClientID,
		id:             *resp.ID,
		principalID:    *resp.Properties.PrincipalID,
	}, nil
}

// FederateServiceAccount creates a federated identity credential for a service account in a cluster
func (m *managedIdentity) FederateServiceAccount(ctx context.Context, name, oidcUrl, sa, namespace string) error {
	lgr := logger.FromContext(ctx).With("name", name, "oidcUrl", oidcUrl, "sa", sa, "namespace", namespace)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to federate service account")

	cred, err := getAzCred()
	if err != nil {
		return fmt.Errorf("getting azure credential: %w", err)
	}

	client, err := armmsi.NewFederatedIdentityCredentialsClient(m.subscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("creating federated identity credentials client: %w", err)
	}

	_, err = client.CreateOrUpdate(ctx, m.resourceGroup, m.name, name, armmsi.FederatedIdentityCredential{
		Properties: &armmsi.FederatedIdentityCredentialProperties{
			Issuer:    to.Ptr(oidcUrl),
			Subject:   to.Ptr(fmt.Sprintf("system:serviceaccount:\"%s\":\"%s\"", namespace, sa)),
			Audiences: []*string{to.Ptr("api://AzureADTokenExchange")},
		},
		Name: to.Ptr(name),
	}, nil)
	if err != nil {
		return fmt.Errorf("creating federated identity credential: %w", err)
	}

	return nil
}

// GetId returns the ID of the managed identity
func (m *managedIdentity) GetId() string {
	return m.id
}

// GetClientID returns the client ID of the managed identity
func (m *managedIdentity) GetClientID() string {
	return m.clientID
}

// GetPrincipalID returns the principal ID of the managed identity
func (m *managedIdentity) GetPrincipalID() string {
	return m.principalID
}
