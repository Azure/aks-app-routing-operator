package keyvault

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	v1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var nginxNamePrefix = "keyvault-nginx-"

type spcConfig struct {
	ClientId        string
	TenantId        string
	KeyvaultCertUri string
	Name            string
	Cloud           string
}

func shouldDeploySpc(obj client.Object) bool {
	switch t := obj.(type) {
	case *v1alpha1.NginxIngressController:
		if t.Spec.DefaultSSLCertificate == nil || t.Spec.DefaultSSLCertificate.KeyVaultURI == nil || *t.Spec.DefaultSSLCertificate.KeyVaultURI == "" {
			return false
		}
		return true

	case *v1.Ingress:
		if t.Annotations == nil || t.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"] == "" {
			return false
		}
		return true
	default:
		return false
	}
}

func buildSPC(spc *secv1.SecretProviderClass, spcConfig spcConfig) error {
	certURI := spcConfig.KeyvaultCertUri

	uri, err := url.Parse(certURI)
	if err != nil {
		return util.NewUserError(err, fmt.Sprintf("unable to parse certificate uri: %s", certURI))
	}
	vaultName := strings.Split(uri.Host, ".")[0]
	chunks := strings.Split(uri.Path, "/")

	if len(chunks) < 3 {
		return util.NewUserError(fmt.Errorf("uri Path contains too few segments: has: %d requires greater than: %d uri path: %s", len(chunks), 3, uri.Path), fmt.Sprintf("invalid secret uri: %s", certURI))
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
		return err
	}
	objects, err := json.Marshal(map[string]interface{}{"array": []string{string(params)}})
	if err != nil {
		return err
	}

	spc.Spec = secv1.SecretProviderClassSpec{
		Provider: secv1.Provider("azure"),
		SecretObjects: []*secv1.SecretObject{{
			SecretName: spcConfig.Name,
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
			"userAssignedIdentityID": spcConfig.ClientId,
			"tenantId":               spcConfig.TenantId,
			"objects":                string(objects),
		},
	}

	if spcConfig.Cloud != "" {
		spc.Spec.Parameters[kvcsi.CloudNameParameter] = spcConfig.Cloud
	}

	return nil
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

func certSecretName(ingressName string) string {
	return fmt.Sprintf("keyvault-%s", ingressName)
}

func shouldReconcileGateway(gwObj *gatewayv1.Gateway) bool {
	return gwObj.Spec.GatewayClassName == istioGatewayClassName
}
