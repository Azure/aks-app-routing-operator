package clients

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

var cred azcore.TokenCredential

func GetAzCred() (azcore.TokenCredential, error) {
	if cred != nil {
		return cred, nil
	}

	new, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("getting default credential: %w", err)
	}

	cred = new
	return cred, nil
}
