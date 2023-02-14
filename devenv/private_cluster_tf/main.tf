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
  name     = "app-routing-dev-${random_string.random.result}a"
  location = "South Central US"
  tags = {
    deletion_due_time  = time_static.provisiontime.unix + 36000, // keep resources for 10hr
    deletion_marked_by = "gc",
  }
}

resource "azurerm_kubernetes_cluster" "cluster-private" {
  name                      = "cluster"
  location                  = azurerm_resource_group.rg.location
  resource_group_name       = azurerm_resource_group.rg.name
  dns_prefix                = "approutingdev"
  azure_policy_enabled      = true
  open_service_mesh_enabled = true
  oidc_issuer_enabled       = true
  private_cluster_enabled = true

  default_node_pool {
    name       = "default"
    node_count = 2
    vm_size    = "Standard_DS3_v2"
  }

  identity {
    type = "SystemAssigned"
  }

  network_profile {
    network_plugin = "kubenet"
    network_policy = "calico"
  }

  key_vault_secrets_provider {
    secret_rotation_enabled  = true
    secret_rotation_interval = "5m"
  }
}

data "azurerm_user_assigned_identity" "clusteridentity" {
  name                = "cluster-agentpool"
  resource_group_name = azurerm_kubernetes_cluster.cluster-private.node_resource_group
}

data "azurerm_resources" "noderesourcegroup" {
  resource_group_name = azurerm_kubernetes_cluster.cluster-private.node_resource_group
  type = "Microsoft.Network/virtualNetworks"
}

resource "azurerm_key_vault" "keyvault" {
  name                     = "dev-${random_string.random.result}a"
  location                 = azurerm_resource_group.rg.location
  resource_group_name      = azurerm_resource_group.rg.name
  tenant_id                = data.azurerm_client_config.current.tenant_id
  purge_protection_enabled = false
  sku_name                 = "standard"

}

resource "azurerm_key_vault_access_policy" "allowtesteraccess" {
  key_vault_id = azurerm_key_vault.keyvault.id
  tenant_id = data.azurerm_client_config.current.tenant_id
  object_id = data.azurerm_client_config.current.object_id

  certificate_permissions = [
    "Get",
    "List",
    "Update",
    "Create",
    "Delete",
    "Import",
  ]

}

resource "azurerm_key_vault_access_policy" "allowclusteraccess" {
  key_vault_id = azurerm_key_vault.keyvault.id
  tenant_id    = data.azurerm_client_config.current.tenant_id
  object_id    = data.azurerm_user_assigned_identity.clusteridentity.principal_id

  certificate_permissions = [
    "Get",
  ]

  secret_permissions = [
    "Get",
  ]
}

resource "azurerm_key_vault_certificate" "testcert" {
  name         = "generated-cert"
  key_vault_id = azurerm_key_vault.keyvault.id
  depends_on = [azurerm_key_vault_access_policy.allowtesteraccess]

  certificate_policy {
    issuer_parameters {
      name = "Self"
    }

    key_properties {
      exportable = true
      key_size   = 2048
      key_type   = "RSA"
      reuse_key  = true
    }

    lifetime_action {
      action {
        action_type = "AutoRenew"
      }

      trigger {
        days_before_expiry = 30
      }
    }

    secret_properties {
      content_type = "application/x-pem-file"
    }

    x509_certificate_properties {
      key_usage = [
        "cRLSign",
        "dataEncipherment",
        "digitalSignature",
        "keyAgreement",
        "keyCertSign",
        "keyEncipherment",
      ]

      subject_alternative_names {
        dns_names = ["*.${var.domain}"]
      }

      subject            = "CN=testcert"
      validity_in_months = 12
    }
  }
}


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
    Kubeconfig        = "${abspath(path.module)}/state/kubeconfig"
    CertID            = azurerm_key_vault_certificate.testcert.id
    CertVersionlessID = azurerm_key_vault_certificate.testcert.versionless_id
    DNSZoneDomain     = var.domain
  })
  filename = "${path.module}/../state/e2e.json"
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
