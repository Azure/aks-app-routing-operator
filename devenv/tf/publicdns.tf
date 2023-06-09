locals {
  publicdnsdomains = [
    for i in range(var.numpublicdnszones): "ingress-${random_string.random.result}-public-${i}.dev"
    ]
}

resource "azurerm_dns_zone" "dnszone" {
  for_each             = toset(local.privatednsdomains)
  name                = each.value 
  resource_group_name = azurerm_resource_group.rg.name
}

resource "azurerm_role_assignment" "approutingdnszone" {
  for_each             = azurerm_dns_zone.dnszone
  scope                = each.value.id
  role_definition_name = "Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
}


locals {
  publicdnszoneids = azurerm_dns_zone.dnszone[*].id
  publicnameservers = azurerm_dns_zone.dnszone[*].name_servers
}
