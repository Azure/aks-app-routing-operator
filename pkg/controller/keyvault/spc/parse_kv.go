package spc

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/aks-app-routing-operator/pkg/util"
)

type certReference struct {
	// vaultName is the name of the Keyvault that contains the certificate
	vaultName string
	// certName is the name of the certificate in Keyvault
	certName string
	// objectVersion is the version of the secret in Keyvault, if empty, the latest version will be used
	objectVersion string
}

func parseKeyVaultCertURI(certURI string) (certReference, error) {
	uri, err := url.Parse(certURI)
	if err != nil {
		return certReference{}, util.NewUserError(err, fmt.Sprintf("unable to parse certificate uri: %s", certURI))
	}

	vaultName := strings.Split(uri.Host, ".")[0]
	chunks := strings.Split(uri.Path, "/")

	if len(chunks) < 3 {
		return certReference{}, util.NewUserError(fmt.Errorf("uri Path contains too few segments: has: %d requires greater than: %d uri path: %s", len(chunks), 3, uri.Path), fmt.Sprintf("invalid secret uri: %s", certURI))
	}
	secretName := chunks[2]

	objectVersion := ""
	if len(chunks) > 3 {
		objectVersion = chunks[3]
	}

	return certReference{
		vaultName:     vaultName,
		certName:      secretName,
		objectVersion: objectVersion,
	}, nil
}
