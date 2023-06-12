variable "publiczones" {
  type = set(string)
  default = [
    "ingress-war-public-1.dev",
    "ingress-war-public-2.dev"
  ]
}


resource "azurerm_dns_zone" "dnszone" {
  for_each            = var.publiczones
  name = "${random_string.random.result}-${each.value}"
  resource_group_name = azurerm_resource_group.rg-public.name
}

resource "azurerm_role_assignment" "approutingdnszone" {
  for_each             = azurerm_dns_zone.dnszone
  scope                = each.value.id
  role_definition_name = "Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
}


#locals {
#  publicdnszoneids = azurerm_dns_zone.dnszone[*].id
#  publicnameservers = azurerm_dns_zone.dnszone[*].name_servers
#}

#locals {
#  publicdnszoneids = { for k, v in azurerm_dns_zone.dnszone : k => v.id }
#  publicnameservers = { for k, v in azurerm_dns_zone.dnszone : k => v.name_servers }
#}
