variable "privatezones" {
  type = set(string)
  default = [
    "ingress-war-private-1.dev",
    "ingress-war-private-2.dev"
  ]
}
resource "azurerm_private_dns_zone" "dnszone" {
  for_each            = var.privatezones
  name = "${random_string.random.result}-${each.value}"
#  name                = "ingress-${random_string.random.result}-private-${count.index}.dev"
  resource_group_name = azurerm_resource_group.rg-private.name

}

resource "azurerm_role_assignment" "dns-role-assignment" {
  for_each             = azurerm_private_dns_zone.dnszone
  scope                = each.value.id
  role_definition_name = "Private DNS Zone Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
}

data "azurerm_resources" "noderesourcegroup" {
  resource_group_name = azurerm_kubernetes_cluster.cluster.node_resource_group
  type = "Microsoft.Network/virtualNetworks"
  count = length(var.privatezones) > 0 ? 1 : 0
}

resource "azurerm_private_dns_zone_virtual_network_link" "approutingvnetconnection" {
  for_each              = azurerm_private_dns_zone.dnszone
  name                  = "approutingdev-link-${each.value.name}"
  resource_group_name   = azurerm_resource_group.rg-private.name
  private_dns_zone_name = each.value.name
  virtual_network_id    = data.azurerm_resources.noderesourcegroup[0].resources[0].id
}