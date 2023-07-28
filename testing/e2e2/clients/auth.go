package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

// https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles

type Role struct {
	Name string
	Id   string
}

var (
	DnsContributorRole = Role{
		Name: "DNS Zone Contributor",
		Id:   "befefa01-2a29-4197-83a8-272ff33ce314",
	}
	PrivateDnsContributorRole = Role{
		Name: "Private DNS Zone Contributor",
		Id:   "b12aa53e-6015-4669-85d0-8515ebb3ae7f",
	}
	AcrPullRole = Role{
		Name: "AcrPull",
		Id:   "7f951dda-4ed3-4680-a7ca-43fe172d538d",
	}
	KeyVaultSecretsUserRole = Role{
		Name: "Key Vault Secrets User",
		Id:   "/providers/Microsoft.Authorization/roleDefinitions/4633458b-17de-408a-b874-0445c86b69e6",
	}
)

type roleAssignment struct{}

func NewRoleAssignment(ctx context.Context, subscriptionId, scope, principalId string, role Role) (*roleAssignment, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create role assignment " + role.Name)
	defer lgr.Info("finished creating role assignment " + role.Name)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armauthorization.NewRoleAssignmentsClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	_, err = client.Create(ctx, scope, role.Id, armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			RoleDefinitionID: to.Ptr(role.Id),
			PrincipalID:      to.Ptr(principalId),
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("creating role assignment: %w", err)
	}

	return &roleAssignment{}, nil
}
