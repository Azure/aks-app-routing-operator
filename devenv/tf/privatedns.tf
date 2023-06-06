resource "azurerm_private_dns_zone" "dnszone" {
  name                = var.domain
  resource_group_name = azurerm_resource_group.rg.name
  count = var.dnszonetype == "private" ? 1 : 0
}

resource "azurerm_role_assignment" "dns-role-assignment" {
  scope                = azurerm_private_dns_zone.dnszone[0].id
  role_definition_name = "Private DNS Zone Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
  count = var.dnszonetype == "private" ? 1 : 0
}

data "azurerm_resources" "noderesourcegroup" {
  resource_group_name = azurerm_kubernetes_cluster.cluster.node_resource_group
  type = "Microsoft.Network/virtualNetworks"
  count = var.dnszonetype == "private" ? 1 : 0
}


resource "azurerm_private_dns_zone_virtual_network_link" "approutingvnetconnection" {
  name                  = "approutingdev${random_string.random.result}a"
  resource_group_name   = azurerm_resource_group.rg.name
  private_dns_zone_name = azurerm_private_dns_zone.dnszone[0].name
  virtual_network_id    = data.azurerm_resources.noderesourcegroup[0].resources[0].id
  count = var.dnszonetype == "private" ? 1 : 0
}
