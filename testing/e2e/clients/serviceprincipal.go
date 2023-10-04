package clients

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
	"github.com/Azure/aks-app-routing-operator/testing/e2e/logger"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	graphserviceprincipals "github.com/microsoftgraph/msgraph-sdk-go/serviceprincipals"
)

type ServicePrincipal struct {
	ID           string
	AppID        string
	Name         string
	ClientSecret string
}

// NewServicePrincipal creates a new service principal with a password secret
func NewServicePrincipal(ctx context.Context, servicePrincipalName string, app *Application) (*ServicePrincipal, error) {
	lgr := logger.FromContext(ctx).With("name", servicePrincipalName)
	lgr.Info("creating service principal")

	scopes := []string{"https://graph.microsoft.com/.default"}
	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return nil, fmt.Errorf("creating client factory: %w", err)
	}
	spRequest := graphmodels.NewServicePrincipal()
	spRequest.SetAppId(util.StringPtr(app.AppID))
	created, err := graphClient.ServicePrincipals().Post(ctx, spRequest, nil)
	if err != nil {
		return nil, fmt.Errorf("creating service principal: %w", err)
	}

	sp := &ServicePrincipal{
		ID:    *created.GetId(),
		AppID: *created.GetAppId(),
		Name:  servicePrincipalName,
	}

	err = addServicePrincipalSecret(ctx, sp)
	if err != nil {
		return nil, fmt.Errorf("adding secret to service principal %w", err)
	}

	lgr.Info(fmt.Sprintf("created service principal %s with id:%s appId:%s", sp.Name, sp.ID, sp.AppID))
	return sp, nil
}

const charBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

func RandStringAlphaNum(n int) (string, error) {
	r := rand.Reader
	b := make([]byte, n)
	for i := range b {
		iRandBig, err := rand.Int(r, big.NewInt(int64(len(charBytes))))
		iRand := int(iRandBig.Int64())
		if err != nil {
			return "", fmt.Errorf("generating random string: %w", err)
		}
		b[i] = charBytes[iRand]
	}
	return string(b), nil
}

func addServicePrincipalSecret(ctx context.Context, servicePrincipal *ServicePrincipal) error {
	lgr := logger.FromContext(ctx)

	lgr.Info("start adding secret to service principal")
	scopes := []string{"https://graph.microsoft.com/.default"}
	graphClient, err := msgraphsdk.NewGraphServiceClientWithCredentials(cred, scopes)
	if err != nil {
		return fmt.Errorf("creating client factory: %w", err)
	}

	//randPassword, err := RandStringAlphaNum(32)
	//if err != nil {
	//	return fmt.Errorf("generating random password: %w", err)
	//}

	requestBody := graphserviceprincipals.NewItemAddPasswordPostRequestBody()
	passwordCredential := graphmodels.NewPasswordCredential()
	displayName := fmt.Sprintf("%s-password", servicePrincipal.Name)
	passwordCredential.SetDisplayName(&displayName)
	//passwordCredential.SetSecretText(&servicePrincipal.ClientSecret)
	//requestBody.SetPasswordCredential(passwordCredential)

	spID := servicePrincipal.ID
	spCreated, err := graphClient.ServicePrincipals().
		ByServicePrincipalId(spID).AddPassword().
		Post(context.Background(), requestBody, nil)
	if err != nil {
		return fmt.Errorf("adding password to service principal %w", err)
	}
	servicePrincipal.ClientSecret = *spCreated.GetSecretText()
	return nil
}
