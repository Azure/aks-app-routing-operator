// this is useful for testing Workload Identity features in the dev environment (for example, workload-identity-backed ExternalDNS)

resource "azurerm_user_assigned_identity" "wi_dev" {
  name                = "wi-dev"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
}

resource "kubernetes_namespace" "wi_dev" {
  metadata {
    name = "wi-dev"
  }
}

resource "kubernetes_service_account" "wi_dev" {
  metadata {
    name      = "wi-dev"
    namespace = kubernetes_namespace.wi_dev.metadata[0].name
    annotations = {
      "azure.workload.identity/client-id" = azurerm_user_assigned_identity.wi_dev.client_id
      "azure.workload.identity/tenant-id" = data.azurerm_client_config.current.tenant_id
    }
  }
}

resource "azurerm_federated_identity_credential" "wi_dev" {
  name                = "wi-dev"
  resource_group_name = azurerm_resource_group.rg.name
  audience            = ["api://AzureADTokenExchange"]
  issuer             = azurerm_kubernetes_cluster.cluster.oidc_issuer_url
  parent_id          = azurerm_user_assigned_identity.wi_dev.id
  subject            = "system:serviceaccount:${kubernetes_namespace.wi_dev.metadata[0].name}:${kubernetes_service_account.wi_dev.metadata[0].name}"
}
