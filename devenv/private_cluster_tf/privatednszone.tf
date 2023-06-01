resource "azurerm_private_dns_zone" "dnszone" {
  name                = var.domain
  resource_group_name = azurerm_resource_group.rg.name
}

resource "azurerm_role_assignment" "private-dns-role-assignment" {
  scope                = azurerm_private_dns_zone.dnszone.id
  role_definition_name = "Private DNS Zone Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
}

resource "azurerm_private_dns_zone_virtual_network_link" "approutingvnetconnection" {
  name                  = "approutingdev${random_string.random.result}a"
  resource_group_name   = azurerm_resource_group.rg.name
  private_dns_zone_name = azurerm_private_dns_zone.dnszone.name
  virtual_network_id    = data.azurerm_resources.noderesourcegroup.resources[0].id
}
