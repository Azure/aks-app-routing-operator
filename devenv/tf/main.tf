variable az_sub_id{
  type = string
  description = "The Azure Subscription ID in which resources will be created."
}

variable az_tenant_id{
  type = string
  description = "The Azure Subscription ID in which resources will be created."
}

terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "= 4.16.0"
    }

    random = {
      source = "hashicorp/random"
      version = "3.6.2"
    }
  }
}

provider "azurerm" {
  features {
    key_vault {
      purge_soft_delete_on_destroy = true
    }
  }
  subscription_id = var.az_sub_id
  tenant_id = var.az_tenant_id
}

// randomly choose location to be less to resource limits on our subscription (they are at the location level)
resource "random_shuffle" "locations" {
  input = ["North Central US", "South Central US", "East US 2", "West US", "West US 2", "West US 3"]
  result_count = 1
}

variable "location" {
  type = string
  description = "The Azure Region in which resources will be created"
  default = ""
}

locals {
  location = var.location == "" ? random_shuffle.locations.result[0] : var.location
}

resource "random_string" "random" {
  length  = 12
  upper   = false
  special = false
}

resource "time_static" "provisiontime" {}

variable "clustertype" {
  description = "The type of cluster to deploy. Can be 'private' or 'public'."
  type = string
}

data "azurerm_client_config" "current" {
}

data "azurerm_subscription" "current" {
}

provider "kubernetes" {
  host                   =  azurerm_kubernetes_cluster.cluster.kube_config.0.host
  client_certificate     =  base64decode(azurerm_kubernetes_cluster.cluster.kube_config.0.client_certificate)
  client_key             =  base64decode(azurerm_kubernetes_cluster.cluster.kube_config.0.client_key)
  cluster_ca_certificate =  base64decode(azurerm_kubernetes_cluster.cluster.kube_config.0.cluster_ca_certificate)
}

resource "azurerm_container_registry" "acr" {
  name                = "approutingdev${random_string.random.result}a"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
  sku                 = "Basic"
}

resource "azurerm_role_assignment" "acr" {
  principal_id                     = azurerm_kubernetes_cluster.cluster.kubelet_identity[0].object_id
  role_definition_name             = "AcrPull"
  scope                            = azurerm_container_registry.acr.id
  skip_service_principal_aad_check = true
}

resource "local_file" "e2econf" {
  content = jsonencode({
    PrivateNameserver = length(var.privatezones) > 0 ? azurerm_kubernetes_cluster.cluster.network_profile[0].dns_service_ip : ""
    RandomPrefix = random_string.random.result
    PublicNameservers    = length(var.publiczones) > 0 ? {for k, v in azurerm_dns_zone.dnszone : k => v.name_servers}:{}
    PublicCertIDs           = {for k,v in azurerm_key_vault_certificate.testcert-public : k => v.id}
    PublicCertVersionlessIDs = {for k,v in azurerm_key_vault_certificate.testcert-public : k => v.versionless_id}
    PrivateCertIDs           = {for k,v in azurerm_key_vault_certificate.testcert-private : k => v.id}
    PrivateCertVersionlessIDs = {for k,v in azurerm_key_vault_certificate.testcert-private : k=> v.versionless_id}
    PrivateDnsZoneIDs = [for k,v in azurerm_private_dns_zone.dnszone: v.id]
    PublicDnsZoneIDs = [for k,v in azurerm_dns_zone.dnszone: v.id]
  })
  filename = "${path.module}/../state/kustomize/e2e/e2e.json"
}

resource "local_file" "registryconf" {
  content  = azurerm_container_registry.acr.login_server
  filename = "${path.module}/../state/registry.txt"
}

resource "local_file" "cluster_info" {
  content = jsonencode({
    ClusterName = azurerm_kubernetes_cluster.cluster.name
    ClusterResourceGroup = azurerm_kubernetes_cluster.cluster.resource_group_name
  })
  filename = "${path.module}/../state/cluster-info.json"
}

resource "local_file" "addon_deployment_auth_info"{
  content = jsonencode({
    ClusterClientId = data.azurerm_user_assigned_identity.clusteridentity.client_id
    ArmTenantId = data.azurerm_client_config.current.tenant_id
    ResourceGroupLocation = azurerm_resource_group.rg.location
    DnsZones = join(",",concat([for zone in azurerm_private_dns_zone.dnszone : zone.id], [for zone in azurerm_dns_zone.dnszone : zone.id]))
    ClusterUid = var.clustertype == "private" ? azurerm_kubernetes_cluster.cluster.private_fqdn : azurerm_kubernetes_cluster.cluster.fqdn # tf doesn't expose CCP ID so using cluster fqdn instead
  })
  filename = "${path.module}/../state/deployment-auth-info.json"
}
