locals {
  publicdnsdomains = [
    for i in range(var.numpublicdnszones): "ingress-${random_string.random.result}-public-${i}.dev"
    ]
}

resource "azurerm_dns_zone" "dnszone" {
  for_each             = publicdnsdomains.dnszone
  name                = each.value 
  resource_group_name = azurerm_resource_group.rg.name
}

resource "azurerm_role_assignment" "approutingdnszone" {
  for_each             = azurerm_dns_zone.dnszone
  role_definition_name = "Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
  count = var.dnszonetype == "public" ? 1 : 0
}


locals {
  publicdnszoneids = azurerm_dns_zone.dnszone[*].id
}
