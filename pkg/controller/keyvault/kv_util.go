package keyvault

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	v1 "k8s.io/api/networking/v1"
	"net/url"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
	"strings"
)

var nginxNamePrefix = "keyvault-nginx-"

func buildSPC(obj client.Object, spc *secv1.SecretProviderClass, config *config.Config) (bool, error) {
	var certURI, specSecretName string

	switch objType := reflect.TypeOf(obj).String(); objType {
	case "*v1alpha1.NginxIngressController":
		nic, _ := obj.(*v1alpha1.NginxIngressController)
		if nic.Spec.DefaultSSLCertificate == nil || nic.Spec.DefaultSSLCertificate.KeyVaultURI == nil {
			return false, nil
		}
		certURI = *nic.Spec.DefaultSSLCertificate.KeyVaultURI
		specSecretName = DefaultNginxCertName(nic)
	case "*v1.Ingress":
		ing, _ := obj.(*v1.Ingress)
		if ing.Annotations == nil {
			return false, nil
		}

		certURI = ing.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]
		specSecretName = fmt.Sprintf("keyvault-%s", ing.Name)
	default:
		return false, fmt.Errorf("incorrect object type: %s", objType)
	}

	if certURI == "" {
		return false, nil
	}

	uri, err := url.Parse(certURI)
	if err != nil {
		return false, err
	}
	vaultName := strings.Split(uri.Host, ".")[0]
	chunks := strings.Split(uri.Path, "/")
	if len(chunks) < 3 {
		return false, fmt.Errorf("invalid secret uri: %s", certURI)
	}
	secretName := chunks[2]
	p := map[string]interface{}{
		"objectName": secretName,
		"objectType": "secret",
	}
	if len(chunks) > 3 {
		p["objectVersion"] = chunks[3]
	}

	params, err := json.Marshal(p)
	if err != nil {
		return false, err
	}
	objects, err := json.Marshal(map[string]interface{}{"array": []string{string(params)}})
	if err != nil {
		return false, err
	}

	spc.Spec = secv1.SecretProviderClassSpec{
		Provider: secv1.Provider("azure"),
		SecretObjects: []*secv1.SecretObject{{
			SecretName: specSecretName,
			Type:       "kubernetes.io/tls",
			Data: []*secv1.SecretObjectData{
				{
					ObjectName: secretName,
					Key:        "tls.key",
				},
				{
					ObjectName: secretName,
					Key:        "tls.crt",
				},
			},
		}},
		// https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/getting-started/usage/#create-your-own-secretproviderclass-object
		Parameters: map[string]string{
			"keyvaultName":           vaultName,
			"useVMManagedIdentity":   "true",
			"userAssignedIdentityID": config.MSIClientID,
			"tenantId":               config.TenantID,
			"objects":                string(objects),
		},
	}

	if config.Cloud != "" {
		spc.Spec.Parameters[kvcsi.CloudNameParameter] = config.Cloud
	}

	return true, nil
}

// DefaultNginxCertName returns a default name for the nginx certificate name using the IngressClassName from the spec.
// Truncates characters in the IngressClassName passed the max secret length (255) if the IngressClassName and the default namespace are over the limit
func DefaultNginxCertName(nic *v1alpha1.NginxIngressController) string {
	secretMaxSize := 255
	certName := nginxNamePrefix + nic.Name

	if len(certName) > secretMaxSize {
		return certName[0:secretMaxSize]
	}

	return certName
}
