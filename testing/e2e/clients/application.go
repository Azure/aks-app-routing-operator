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
	//other-imports
)

type Application struct {
	ID           string
	AppID        string
	Name         string
	CredPassword string
}

func TimePtr(t time.Time) *time.Time { return &t }

func GetApplicationByAppIDWithNewPasswordCred(ctx context.Context, appId string, credName string) (*Application, error) {
	lgr := logger.FromContext(ctx).With("appid", appId)
	lgr.Info(fmt.Sprintf("getting application with appid %s", appId))

	scopes := []string{"https://graph.microsoft.com/.default"}
	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}
	getAppResponse, err := graphClient.Applications().ByApplicationId(appId).Get(ctx, nil)
	if err != nil {
		return nil, err
	}

	addPasswordReq := graphapplications.NewItemAddPasswordPostRequestBody()
	newCreds := graphmodels.NewPasswordCredential()
	newCreds.SetDisplayName(util.StringPtr(credName))
	newCreds.SetEndDateTime(TimePtr(time.Now().Add(24 * time.Hour)))
	addPasswordReq.SetPasswordCredential(newCreds)
	addPasswordCredResp, err := graphClient.Applications().ByApplicationId(appId).AddPassword().Post(ctx, addPasswordReq, nil)
	if err != nil {
		return nil, fmt.Errorf("adding password to application: %w", err)
	}
	lgr.Info(fmt.Sprintf("added password with display name %s: ", addPasswordCredResp.GetDisplayName()))

	app := &Application{
		ID:           *getAppResponse.GetId(),
		AppID:        *getAppResponse.GetAppId(),
		Name:         *getAppResponse.GetDisplayName(),
		CredPassword: *addPasswordCredResp.GetSecretText(),
	}

	return app, nil
}

func NewApplication(ctx context.Context, name string) (*Application, error) {
	lgr := logger.FromContext(ctx).With("name", name)
	lgr.Info(fmt.Sprintf("creating application %s", name))

	scopes := []string{"https://graph.microsoft.com/.default"}
	cred, err := getAzCred()
	newAppBody := graphmodels.NewApplication()
	ptrName := util.StringPtr(name)
	newAppBody.SetDisplayName(ptrName)

	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return nil, fmt.Errorf("creating graph client: %w", err)
	}
	createdApp, err := graphClient.Applications().Post(ctx, newAppBody, nil)
	if err != nil {
		return nil, fmt.Errorf("creating application: %w", err)
	}

	addPasswordReq := graphapplications.NewItemAddPasswordPostRequestBody()
	newCreds := graphmodels.NewPasswordCredential()
	newCreds.SetDisplayName(util.StringPtr(fmt.Sprintf("%s-cred", name)))
	newCreds.SetEndDateTime(TimePtr(time.Now().Add(24 * time.Hour)))
	addPasswordReq.SetPasswordCredential(newCreds)
	resp, err := graphClient.Applications().ByApplicationId(*createdApp.GetId()).AddPassword().Post(ctx, addPasswordReq, nil)
	if err != nil {
		return nil, fmt.Errorf("adding password to application: %w", err)
	}
	lgr.Info(fmt.Sprintf("added password with display name %s: ", resp.GetDisplayName()))

	app := &Application{
		ID:           *createdApp.GetId(),
		AppID:        *createdApp.GetAppId(),
		Name:         name,
		CredPassword: *resp.GetSecretText(),
	}

	//get app to verify the identity has propagated through our sad excuse of a caching system
	//exponential backoff
	// uP tO 30 mInUtEs tO PrOpAgAtE
	// https://learn.microsoft.com/en-us/answers/questions/524792/azure-rbac-propagation-latency
	maxRetries := 8
	retryTime := 5 * time.Second

	retryNum := 0
	for {
		sp, err := graphClient.ServicePrincipalsWithAppId(util.StringPtr(app.AppID)).Get(ctx, nil)
		if err == nil {
			lgr.Info(fmt.Sprintf("successfully retrieved service principal with appid %s id %s", app.AppID, *sp.GetId()))
			break
		}
		if retryNum >= maxRetries {
			return nil, fmt.Errorf("retrieving service principal with appid %s: %w", app.AppID, err)
		}

		lgr.Info(fmt.Sprintf("failed to retrieve service principal with appid %s, retrying in %s", app.AppID, retryTime.String()))
		time.Sleep(retryTime)

		retryNum++
		retryTime *= 2
	}

	return app, nil
}

func (a Application) Delete(ctx context.Context) error {
	lgr := logger.FromContext(ctx).With("name", a.Name)
	lgr.Info(fmt.Sprintf("starting to delete application %s", a.Name))

	scopes := []string{"https://graph.microsoft.com/.default"}
	cred, err := getAzCred()
	appToDelete := graphmodels.NewApplication()
	appToDelete.SetId(util.StringPtr(a.ID))

	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return fmt.Errorf("deleting graph client: %w", err)
	}
	err = graphClient.Applications().ByApplicationId(a.ID).Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("deleting application: %w", err)
	}

	return nil
}
