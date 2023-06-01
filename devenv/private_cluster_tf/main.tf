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

provider "kubernetes" {
  host                   =  azurerm_kubernetes_cluster.cluster-private.kube_config.0.host
  client_certificate     =  base64decode(azurerm_kubernetes_cluster.cluster-private.kube_config.0.client_certificate)
  client_key             =  base64decode(azurerm_kubernetes_cluster.cluster-private.kube_config.0.client_key)
  cluster_ca_certificate =  base64decode(azurerm_kubernetes_cluster.cluster-private.kube_config.0.cluster_ca_certificate)
}

resource "random_string" "random" {
  length  = 12
  upper   = false
  special = false
}

resource "time_static" "provisiontime" {}

variable "domain" {
  default = "ingress.dev"
}

data "azurerm_client_config" "current" {
}

data "azurerm_subscription" "current" {
}

resource "azurerm_resource_group" "rg" {
  name     = "app-routing-dev-${random_string.random.result}"
  location = "South Central US"
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
  principal_id                     = azurerm_kubernetes_cluster.cluster-private.kubelet_identity[0].object_id
  role_definition_name             = "AcrPull"
  scope                            = azurerm_container_registry.acr.id
  skip_service_principal_aad_check = true
}


resource "local_sensitive_file" "kubeconfig" {
  content  =azurerm_kubernetes_cluster.cluster-private.kube_config_raw
  filename = "${path.module}/../state/kubeconfig"
}


resource "local_file" "e2econfprivatedns" {
  content = jsonencode({
    TestNameservers    = [azurerm_kubernetes_cluster.cluster-private.network_profile[0].dns_service_ip]
    CertID            = azurerm_key_vault_certificate.testcert.id
    CertVersionlessID = azurerm_key_vault_certificate.testcert.versionless_id
    DNSZoneDomain     = var.domain
  })
  filename = "${path.module}/../state/kustomize/e2e/e2e.json"
}

resource "local_file" "registryconf" {
  content  = azurerm_container_registry.acr.login_server
  filename = "${path.module}/../state/registry.txt"
}

resource "local_file" "cluster_info" {
  content = jsonencode({
    ClusterName = azurerm_kubernetes_cluster.cluster-private.name
    ClusterResourceGroup = azurerm_kubernetes_cluster.cluster-private.resource_group_name
  })
  filename = "${path.module}/../state/cluster-info.json"
}

resource "local_file" "private_cluster_addon_deployment_auth_info"{
  content = jsonencode({
    ClusterClientId = data.azurerm_user_assigned_identity.clusteridentity.client_id
    ArmTenantId = data.azurerm_client_config.current.tenant_id
    ResourceGroupLocation = azurerm_resource_group.rg.location
    DnsResourceGroup = azurerm_private_dns_zone.dnszone.resource_group_name
    DnsZoneSubscription = data.azurerm_subscription.current.subscription_id
    DnsZoneDomain = azurerm_private_dns_zone.dnszone.name
  })
  filename = "${path.module}/../state/deployment-auth-info.json"
}