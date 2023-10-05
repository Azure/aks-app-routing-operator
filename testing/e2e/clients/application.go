package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphapplications "github.com/microsoftgraph/msgraph-sdk-go/applications"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

func TimePtr(t time.Time) *time.Time { return &t }

func GetServicePrincipalOptions(ctx context.Context, applicationObjectID string, credName string) (*ServicePrincipalOptions, error) {
	lgr := logger.FromContext(ctx).With("appid", applicationObjectID)
	lgr.Info(fmt.Sprintf("getting application with appid %s", applicationObjectID))

	cred, err := getAzCred()
	scopes := []string{"https://graph.microsoft.com/.default"}
	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)

	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}
	getAppResponse, err := graphClient.Applications().ByApplicationId(applicationObjectID).Get(ctx, nil)
	if err != nil {
		return nil, err
	}

	addPasswordReq := graphapplications.NewItemAddPasswordPostRequestBody()
	newCreds := graphmodels.NewPasswordCredential()
	newCreds.SetDisplayName(util.StringPtr(credName))
	newCreds.SetEndDateTime(TimePtr(time.Now().Add(24 * time.Hour)))
	addPasswordReq.SetPasswordCredential(newCreds)
	addPasswordCredResp, err := graphClient.Applications().ByApplicationId(applicationObjectID).AddPassword().Post(ctx, addPasswordReq, nil)
	if err != nil {
		return nil, fmt.Errorf("adding password to application: %w", err)
	}
	lgr.Info(fmt.Sprintf("added password with display name %s: ", *addPasswordCredResp.GetDisplayName()))

	// get sp object id
	sp, err := graphClient.ServicePrincipalsWithAppId(getAppResponse.GetAppId()).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting service principal: %w", err)
	}

	spOpt := &ServicePrincipalOptions{
		ApplicationObjectID:          *getAppResponse.GetId(),
		ApplicationClientID:          *getAppResponse.GetAppId(),
		ServicePrincipalObjectID:     *sp.GetId(),
		ServicePrincipalCredPassword: *addPasswordCredResp.GetSecretText(),
	}

	return spOpt, nil
}
