provider "azurerm" {
  features {
    key_vault {
      purge_soft_delete_on_destroy = true
    }
  }
}

resource "random_string" "random" {
  length  = 12
  upper   = false
  special = false
}

resource "time_static" "provisiontime" {}

variable "example_ingress_domain" {
  default = "ingress.dev"
}

variable "example_ingress_host" {
  default = "test.ingress.dev"
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

resource "azurerm_kubernetes_cluster" "cluster" {
  name                      = "cluster"
  location                  = azurerm_resource_group.rg.location
  resource_group_name       = azurerm_resource_group.rg.name
  dns_prefix                = "approutingdev"
  azure_policy_enabled      = true
  open_service_mesh_enabled = true

  default_node_pool {
    name       = "default"
    node_count = 2
    vm_size    = "Standard_DS2_v2"
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
  resource_group_name = azurerm_kubernetes_cluster.cluster.node_resource_group
}

resource "azurerm_key_vault" "keyvault" {
  name                     = "dev-${random_string.random.result}a"
  location                 = azurerm_resource_group.rg.location
  resource_group_name      = azurerm_resource_group.rg.name
  tenant_id                = data.azurerm_client_config.current.tenant_id
  purge_protection_enabled = false
  sku_name                 = "standard"

  access_policy {
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

  access_policy {
    tenant_id = data.azurerm_client_config.current.tenant_id
    object_id = data.azurerm_user_assigned_identity.clusteridentity.principal_id

    certificate_permissions = [
      "Get",
    ]

    secret_permissions = [
      "Get",
    ]
  }
}

resource "azurerm_key_vault_certificate" "testcert" {
  name         = "generated-cert"
  key_vault_id = azurerm_key_vault.keyvault.id

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
        dns_names = ["${var.example_ingress_host}"]
      }

      subject            = "CN=testcert"
      validity_in_months = 12
    }
  }
}

resource "azurerm_dns_zone" "dnszone" {
  name                = var.example_ingress_domain
  resource_group_name = azurerm_resource_group.rg.name
}

resource "azurerm_role_assignment" "approutingdnszone" {
  scope                = azurerm_dns_zone.dnszone.id
  role_definition_name = "Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
}

resource "local_sensitive_file" "kubeconfig" {
  content  = azurerm_kubernetes_cluster.cluster.kube_config_raw
  filename = "${path.module}/state/kubeconfig"
}

resource "local_file" "envscript" {
  content  = <<EOF
    export KUBECONFIG="${abspath(path.module)}/state/kubeconfig"

    function run() {
      cd ${abspath("${path.module}/../")}
      go run . \
        --msi ${data.azurerm_user_assigned_identity.clusteridentity.client_id} \
        --tenant-id ${data.azurerm_client_config.current.tenant_id} \
        --location ${azurerm_resource_group.rg.location} \
        --dns-zone-resource-group ${azurerm_dns_zone.dnszone.resource_group_name} \
        --dns-zone-subscription ${data.azurerm_subscription.current.subscription_id} \
        --dns-zone-domain ${var.example_ingress_domain}
    }
  EOF
  filename = "${path.module}/state/env.sh"
}
