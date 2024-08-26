package keyvault

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/aks-app-routing-operator/api/v1alpha1"
	"github.com/Azure/aks-app-routing-operator/pkg/config"
	"github.com/Azure/aks-app-routing-operator/pkg/manifests"
	"github.com/Azure/aks-app-routing-operator/pkg/util"
	kvcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"github.com/go-logr/logr"
	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	secv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var nginxNamePrefix = "keyvault-nginx-"

func buildSPC(obj client.Object, spc *secv1.SecretProviderClass, config *config.Config) (bool, error) {
	var certURI, specSecretName string

	switch t := obj.(type) {
	case *v1alpha1.NginxIngressController:
		if t.Spec.DefaultSSLCertificate == nil || t.Spec.DefaultSSLCertificate.KeyVaultURI == nil {
			return false, nil
		}
		certURI = *t.Spec.DefaultSSLCertificate.KeyVaultURI
		specSecretName = DefaultNginxCertName(t)
	case *v1.Ingress:
		if t.Annotations == nil {
			return false, nil
		}

		certURI = t.Annotations["kubernetes.azure.com/tls-cert-keyvault-uri"]
		specSecretName = certSecretName(t.Name)
	default:
		return false, fmt.Errorf("incorrect object type: %s", t)
	}

	if certURI == "" {
		return false, nil
	}

	uri, err := url.Parse(certURI)
	if err != nil {
		return false, newUserError(err, fmt.Sprintf("unable to parse certificate uri: %s", certURI))
	}
	vaultName := strings.Split(uri.Host, ".")[0]
	chunks := strings.Split(uri.Path, "/")

	if len(chunks) < 3 {
		return false, newUserError(fmt.Errorf("uri Path contains too few segments: has: %d requires greater than: %d uri path: %s", len(chunks), 3, uri.Path), fmt.Sprintf("invalid secret uri: %s", certURI))
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

func ensureSA(ctx context.Context, conf *config.Config, cl client.Client, logger logr.Logger) error {
	if !conf.UseWorkloadIdentity {
		log.Info("not using workload identity, service account not needed")
		return nil
	}

	log.Info("upserting service account")
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-provider",
			Namespace: conf.NS,
			Annotations: map[string]string{
				"azure.workload.identity/client-id": conf.MSIClientID,
			},
			Labels: manifests.GetTopLevelLabels(),
		},
	}
	if err := util.Upsert(ctx, cl, sa); err != nil {
		return fmt.Errorf("upserting service account: %w", err)
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

type userError struct {
	err         error
	userMessage string
}

// for internal use
func (b userError) Error() string {
	return b.err.Error()
}

// for user facing messages
func (b userError) UserError() string {
	return b.userMessage
}

func newUserError(err error, msg string) userError {
	return userError{err, msg}
}
