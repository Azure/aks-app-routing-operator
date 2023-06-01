resource "azurerm_kubernetes_cluster" "cluster-private" {
  name                      = "cluster"
  location                  = azurerm_resource_group.rg.location
  resource_group_name       = azurerm_resource_group.rg.name
  dns_prefix                = "approutingdev"
  azure_policy_enabled      = true
  open_service_mesh_enabled = true
  oidc_issuer_enabled       = true
  private_cluster_enabled = true

  default_node_pool {
    name       = "default"
    node_count = 2
    vm_size    = "Standard_DS3_v2"
  }

  identity {
    type = "SystemAssigned"
  }

  network_profile {
    network_plugin = "kubenet"
    network_policy = "calico"
  }

  key_vault_secrets_provider {
    secret_rotation_enabled  = true
    secret_rotation_interval = "5m"
  }
}

data "azurerm_user_assigned_identity" "clusteridentity" {
  name                = "cluster-agentpool"
  resource_group_name = azurerm_kubernetes_cluster.cluster-private.node_resource_group
}

data "azurerm_resources" "noderesourcegroup" {
  resource_group_name = azurerm_kubernetes_cluster.cluster-private.node_resource_group
  type = "Microsoft.Network/virtualNetworks"
}