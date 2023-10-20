# Service Principal Authentication Support

App routing operator supports service-principal authentication for AKS clusters.


> ⚠️ Warning ⚠️
>
> Service Principals are not recommended in any environment that supports Managed Identities, as they are less secure and more difficult to manage.

When using a Service Principal AKS Cluster, the automatically created addon identity that uses MSI is no longer available.
In this case, authentication with a provided service principal can be used instead, which requires users to provide a service principal credentials via manually creating kubernetes secrets to be read by app routing components.


## Manual Secrets
The following Kubernetes secrets must be manually created when using a Service Principal configuration:

### Keyvault CSI Driver Secret
A secret is needed for the placeholder pods to access keyvault, which allows mirroring of Key Vault Secrets into Kubernetes Secrets

Service Principal secrets and permissions must be configured as specified by the [Key Vault CSI driver documentation](https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/configurations/identity-access-modes/service-principal-mode/#configure-service-principal-to-access-keyvault)

The name of the created secret must be `keyvault-service-principal`, and it must be created in the same namespaces as the ingress.

The service principal used for this secret must have the following permissions:
- `get` and `list` permissions on the keyvault for the secret

The Key Vault CSI driver secret has the following shape:
```yaml
# keyvault-csi-driver-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: keyvault-service-principal
  namespace: <namespace of the ingress>
type: Opaque
data:
  clientid: <base64 encoded service principal client id>
  clientsecret: <base64 encoded service principal client secret>
```

One secret must be created for each namespace that has an ingress.

### ExternalDNS Secret
A secret is needed for the externaldns pods to access Azure resources, which allows external dns to create dns records in Azure DNS Zones.
Only a single secret is needed per externaldns deployment, and it should be created in the same namespace as the externaldns deployments (normally `app-routing-system`)

There are at most two secrets needed, one for public dns zones and one for private dns zones.

The secrets are named `sp-creds-external-dns` and `sp-creds-external-dns-private`

Both secrets should be set up following the [externaldns documentation](https://github.com/kubernetes-sigs/external-dns/blob/0725104c9e594ef6f91c380f8bdc0e21129eae70/docs/tutorials/azure.md#service-principal)

The should each contain a single entry in their `data` field with the key `azure.json`, containing the base64 encoded version of the following JSON object 
```json
// azure.json
{
  "tenantId": "<tenant id>",
  "subscriptionId": "<subscription id>",
  "resourceGroup": "<resource group of the relevant dns zones>",
  "aadClientId": "<service principal application object client id>",
  "aadClientSecret": "<service principal credential secret>"
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