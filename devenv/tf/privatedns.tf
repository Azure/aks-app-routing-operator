locals {
  privatednsdomains = [
    for i in range(var.numprivatednszones): "ingress-${random_string.random.result}-private-${i}.dev"
    ]
}

resource "azurerm_private_dns_zone" "dnszone" {
  for_each            = toset(local.privatednsdomains)
  name                = each.value
  resource_group_name = azurerm_resource_group.rg.name
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
  count = var.privatednszones > 0 ? 1 : 0
}

resource "azurerm_private_dns_zone_virtual_network_link" "approutingvnetconnection" {
  for_each              = azurerm_private_dns_zone.dnszone
  name                  = "approutingdev-link-${each.value.name}"
  resource_group_name   = azurerm_resource_group.rg.name
  private_dns_zone_name = each.value.name
  virtual_network_id    = data.azurerm_resources.noderesourcegroup[0].resources[0].id
}

locals {
  publicdnszoneids = azurerm_private_dns_zone.dnszone[*].id
}
