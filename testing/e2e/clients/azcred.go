package clients

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/golang-jwt/jwt/v4"
)

var cred azcore.TokenCredential

func getAzCred() (azcore.TokenCredential, error) {
	if cred != nil {
		return cred, nil
	}

	// this is CLI instead of DefaultCredential to ensure we are using the same credential as the CLI
	// and authed through the cli. We use the az cli directly when pushing an image to ACR for now.
	c, err := azidentity.NewAzureCLICredential(nil)
	if err != nil {
		return nil, fmt.Errorf("getting az cli credential: %w", err)
	}

	cred = c
	return cred, nil
}

// getObjectId gets the object id of the azure credential.
// adapted from https://stackoverflow.com/a/75658185
func getObjectId(ctx context.Context, cred azcore.TokenCredential) (string, error) {
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		// okay to hardcode to PublicCloud since we should never deploy to anything else in public OSS repo
		Scopes: []string{cloud.AzurePublic.Services[cloud.ResourceManager].Endpoint + "/.default"},
	})
	if err != nil {
		return "", fmt.Errorf("getting token: %w", err)
	}

	type t struct {
		ObjectId string `json:"oid"`
		jwt.RegisteredClaims
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claim := &t{}
	if _, _, err := parser.ParseUnverified(token.Token, claim); err != nil {
		return "", fmt.Errorf("parsing token: %w", err)
	}

	objectId := claim.ObjectId
	if objectId == "" {
		return "", fmt.Errorf("object id is empty")
	}

	return objectId, nil
}
