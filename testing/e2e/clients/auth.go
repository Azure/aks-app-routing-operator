package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/google/uuid"
)

// https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles

type Role struct {
	Name string
	Id   string
}

var (
	DnsContributorRole = Role{
		Name: "DNS Zone Contributor",
		Id:   "/subscriptions/8ecadfc9-d1a3-4ea4-b844-0d9f87e4d7c8/providers/Microsoft.Authorization/roleDefinitions/befefa01-2a29-4197-83a8-272ff33ce314",
	}
	PrivateDnsContributorRole = Role{
		Name: "Private DNS Zone Contributor",
		Id:   "/subscriptions/8ecadfc9-d1a3-4ea4-b844-0d9f87e4d7c8/providers/Microsoft.Authorization/roleDefinitions/b12aa53e-6015-4669-85d0-8515ebb3ae7f",
	}
	AcrPullRole = Role{
		Name: "AcrPull",
		Id:   "/subscriptions/8ecadfc9-d1a3-4ea4-b844-0d9f87e4d7c8/providers/Microsoft.Authorization/roleDefinitions/7f951dda-4ed3-4680-a7ca-43fe172d538d",
	}
)

type roleAssignment struct{}

func NewRoleAssignment(ctx context.Context, subscriptionId, scope, principalId string, role Role) (*roleAssignment, error) {
	lgr := logger.FromContext(ctx).With("role", role.Name, "subscriptionId", subscriptionId, "scope", scope, "principalId", principalId)
	ctx = logger.WithContext(ctx, lgr)
	lgr.Info("starting to create role assignment")
	defer lgr.Info("finished creating role assignment")

	cred, err := getAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armauthorization.NewRoleAssignmentsClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	_, err = client.Create(ctx, scope, uuid.New().String(), armauthorization.RoleAssignmentCreateParameters{
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
