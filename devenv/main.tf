provider "azurerm" {
  features {
    key_vault {
      purge_soft_delete_on_destroy = true
    }
  }
}

provider "kubernetes" {
  host                   = azurerm_kubernetes_cluster.cluster.kube_config.0.host
  client_certificate     = base64decode(azurerm_kubernetes_cluster.cluster.kube_config.0.client_certificate)
  client_key             = base64decode(azurerm_kubernetes_cluster.cluster.kube_config.0.client_key)
  cluster_ca_certificate = base64decode(azurerm_kubernetes_cluster.cluster.kube_config.0.cluster_ca_certificate)
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

resource "azurerm_kubernetes_cluster" "cluster" {
  name                      = "cluster"
  location                  = azurerm_resource_group.rg.location
  resource_group_name       = azurerm_resource_group.rg.name
  dns_prefix                = "approutingdev"
  azure_policy_enabled      = true
  open_service_mesh_enabled = true
  oidc_issuer_enabled       = true

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

resource "azurerm_dns_zone" "dnszone" {
  name                = var.domain
  resource_group_name = azurerm_resource_group.rg.name
}

resource "azurerm_role_assignment" "approutingdnszone" {
  scope                = azurerm_dns_zone.dnszone.id
  role_definition_name = "Contributor"
  principal_id         = data.azurerm_user_assigned_identity.clusteridentity.principal_id
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

resource "kubernetes_deployment_v1" "operator" {
  wait_for_rollout = false

  lifecycle {
    ignore_changes = [spec.0.template.0.spec.0.container.0.image]
  }

  metadata {
    name      = "app-routing-operator"
    namespace = "kube-system"
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = "app-routing-operator"
      }
    }

    template {
      metadata {
        labels = {
          app = "app-routing-operator"
        }
      }

      spec {
        container {
          name  = "operator"
          image = "mcr.microsoft.com/oss/kubernetes/pause:3.6-hotfix.20220114"
          command = [
            "/aks-app-routing-operator",
            "--msi", "${data.azurerm_user_assigned_identity.clusteridentity.client_id}",
            "--tenant-id", "${data.azurerm_client_config.current.tenant_id}",
            "--location", "${azurerm_resource_group.rg.location}",
            "--dns-zone-resource-group", "${azurerm_dns_zone.dnszone.resource_group_name}",
            "--dns-zone-subscription", "${data.azurerm_subscription.current.subscription_id}",
            "--dns-zone-domain", "${var.domain}"
          ]
        }
      }
    }
  }
}

resource "kubernetes_cluster_role_binding_v1" "defaultadmin" {
  metadata {
    name = "default-admin"
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "cluster-admin"
  }

  subject {
    kind      = "ServiceAccount"
    name      = "default"
    namespace = "kube-system"
  }
}

resource "local_sensitive_file" "kubeconfig" {
  content  = azurerm_kubernetes_cluster.cluster.kube_config_raw
  filename = "${path.module}/state/kubeconfig"
}

resource "local_file" "e2econf" {
  content = jsonencode({
    TestNamservers    = azurerm_dns_zone.dnszone.name_servers
    Kubeconfig        = "${abspath(path.module)}/state/kubeconfig"
    CertID            = azurerm_key_vault_certificate.testcert.id
    CertVersionlessID = azurerm_key_vault_certificate.testcert.versionless_id
    DNSZoneDomain     = var.domain
  })
  filename = "${path.module}/state/e2e.json"
}

resource "local_file" "registryconf" {
  content  = azurerm_container_registry.acr.login_server
  filename = "${path.module}/state/registry.txt"
}
