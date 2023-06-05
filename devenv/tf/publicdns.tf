resource "azurerm_dns_zone" "dnszone" {
  name                = var.domain
  resource_group_name = azurerm_resource_group.rg.name
  count = var.dnszonetype == "public" ? 1 : 0
}

resource "azurerm_role_assignment" "approutingdnszone" {
  scope                = azurerm_dns_zone.dnszone[0].id
  role_definition_name = "Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
  count = var.dnszonetype == "public" ? 1 : 0
}
