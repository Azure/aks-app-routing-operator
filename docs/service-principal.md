# Service Principal Auth Support

The app routing operator supports service-principal authentication for AKS clusters.

When using service principal auth, the automatically created addon identity that uses MSI is no longer available. Authentication with a provided service principal is used instead which requires users to provide a service principal appId and secret as a kubernetes secret.

## Manual Secrets
The following Kubernetes secrets must be manually created when using a Service Principal configuration:

### Keyvault CSI Driver Secret
A secret is needed for the placeholder pods to access keyvault, which allows mirroring of Key Vault secrets into Kubernetes secrets

Service Principal secrets and permissions must be configured as specified by the [keyvault csi driver documentation](https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/configurations/identity-access-modes/service-principal-mode/#configure-service-principal-to-access-keyvault)

The name of the created secret must be `keyvault-service-principal`, and it should be created in the same namespaces as the ingress.

The service principal used for this secret must have the following permissions:
- `get` and `list` permissions on the keyvault for the secret

The final secret should look like this:
```yaml
# keyvault-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: keyvault-service-principal
  namespace: <namespace of the ingress>
type: Opaque
data:
  clientid: <base64 encoded client id>
  clientsecret: <base64 encoded client secret>
```

One secret must be created for each namespace that has an ingress.

### ExternalDNS Secret
A secret is needed for the external dns pods to access Azure resources, which allows external dns to create dns records in Azure DNS Zones
Only a single secret is needed per externaldns deployment, and it should be created in the same namespace as the externaldns deployments (usually `app-routing-system`)

There are at most two secrets needed, one for public dns zones and one for private dns zones.

The secrets are named `sp-creds-external-dns` and `sp-creds-external-dns-private`

Both secrets should be set up following the [externaldns documentation](https://github.com/kubernetes-sigs/external-dns/blob/0725104c9e594ef6f91c380f8bdc0e21129eae70/docs/tutorials/azure.md#service-principal)

The should each contain a single entry in their `data` field with the key `azure.json` which contains the basew64 encoded version of the following JSON schema
```json
// azure.json
{
  "tenantId": "<tenant id>",
  "subscriptionId": "<subscription id>",
  "resourceGroup": "<resource group of the related dns zones>",
  "aadClientId": "<EXTERNALDNS_SP_APP_ID>",
  "aadClientSecret": "<EXTERNALDNS_SP_PASSWORD>"
}
```

The final secrets shouuld look like this:
```yaml
# external-dns-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: <sp-creds-external-dns | sp-creds-external-dns-private>
  namespace: app-routing-operator
type: Opaque
data:
  azure.json: <base64 encoded azure.json file>

```