package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphapplications "github.com/microsoftgraph/msgraph-sdk-go/applications"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// GetServicePrincipal populates a new ServicePrincipalOptions struct with fresh credentials and application/client/servicePrincipal object ids
func GetServicePrincipal(ctx context.Context, applicationObjectID string, credName string) (ServicePrincipal, error) {
	lgr := logger.FromContext(ctx)
	lgr.Info(fmt.Sprintf("getting application with object id %s", applicationObjectID))
	sp := ServicePrincipal{}

	cred, err := getAzCred()
	scopes := []string{"https://graph.microsoft.com/.default"}
	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return sp, fmt.Errorf("creating graph client: %w", err)
	}

	getAppResponse, err := graphClient.Applications().ByApplicationId(applicationObjectID).Get(ctx, nil)
	if err != nil {
		return sp, fmt.Errorf("getting application with object id %s: %w", applicationObjectID, err)
	}

	// add new password credential
	addPasswordReq := graphapplications.NewItemAddPasswordPostRequestBody()
	newCreds := graphmodels.NewPasswordCredential()
	newCreds.SetDisplayName(util.StringPtr(credName))
	newCreds.SetEndDateTime(to.Ptr(time.Now().Add(2 * time.Hour)))
	addPasswordReq.SetPasswordCredential(newCreds)
	addPasswordCredResp, err := graphClient.Applications().ByApplicationId(applicationObjectID).AddPassword().Post(ctx, addPasswordReq, nil)
	if err != nil {
		return sp, fmt.Errorf("adding password to application: %w", err)
	}
	lgr.Info(fmt.Sprintf("added password with display name %s: ", *addPasswordCredResp.GetDisplayName()))

	// get service principal object id
	spObj, err := graphClient.ServicePrincipalsWithAppId(getAppResponse.GetAppId()).Get(ctx, nil)
	if err != nil {
		return sp, fmt.Errorf("getting service principal: %w", err)
	}

	spOpt := ServicePrincipal{
		ApplicationObjectID:          *getAppResponse.GetId(),
		ApplicationClientID:          *getAppResponse.GetAppId(),
		ServicePrincipalObjectID:     *spObj.GetId(),
		ServicePrincipalCredPassword: *addPasswordCredResp.GetSecretText(),
	}
	return spOpt, nil
}
