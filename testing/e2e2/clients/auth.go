package clients

import (
	"context"
	"fmt"

	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
)

type roleAssignment struct{}

func NewRoleAssignment(ctx context.Context, subscriptionId, scope, assignmentName, principalId string) (*roleAssignment, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info("starting to create role assignment " + assignmentName)
	defer lgr.Info("finished creating role assignment " + assignmentName)

	cred, err := GetAzCred()
	if err != nil {
		return nil, fmt.Errorf("getting az credentials: %w", err)
	}

	client, err := armauthorization.NewRoleAssignmentsClient(subscriptionId, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	_, err = client.Create(ctx, scope, assignmentName, armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID: to.Ptr(principalId),
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("creating role assignment: %w", err)
	}

	return &roleAssignment{}, nil
}
