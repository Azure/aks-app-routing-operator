terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "= 3.43.0"
    }
  }
}

provider "azurerm" {
  features {
    key_vault {
      purge_soft_delete_on_destroy = true
    }
  }
}

variable "location" {
  type = string
  description = "The Azure Region in which resources will be created"
  default = "South Central US"
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

variable "numprivatednszones" {
  description = "The number of private dns zones to create"
  type = string
}
variable "numpublicdnszones" {
  description = "The number of public dns zones to create"
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

resource "azurerm_resource_group" "rg" {
  name     = "app-routing-dev-${random_string.random.result}a"
  location = var.location
  tags = {
    deletion_due_time  = time_static.provisiontime.unix + 36000, // keep resources for 10hr
    deletion_marked_by = "gc",
  }
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
    TestNameservers    = var.dnszonetype == "private" ? [azurerm_kubernetes_cluster.cluster.network_profile[0].dns_service_ip] : azurerm_dns_zone.dnszone[0].name_servers
    CertID            = azurerm_key_vault_certificate.testcert.id
    CertVersionlessID = azurerm_key_vault_certificate.testcert.versionless_id
    DNSZoneDomain     = var.domain
    PrivateDnsZones = join(",", local.privatednszoneids)
    PublicDnsZones = join(",",  local.publicdnszoneids)
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
    DnsResourceGroup = azurerm_resource_group.rg.name
    DnsZoneSubscription = data.azurerm_subscription.current.subscription_id
    PrivateDnsZones = join(",", local.privatednszoneids)
    PublicDnsZones = join(",",  local.publicdnszoneids)
  })
  filename = "${path.module}/../state/deployment-auth-info.json"
}
