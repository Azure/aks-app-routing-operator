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

resource "azurerm_key_vault_certificate" "testcert-private" {
  for_each             = toset(local.privatednsdomains)
  name         = "generated-cert-${each.value}"
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
        dns_names = ["*.${each.value}"]
      }

      subject            = "CN=testcert"
      validity_in_months = 12
    }
  }
}

resource "azurerm_key_vault_certificate" "testcert-public" {
  for_each             = toset(local.publicdnsdomains)
  name         = "generated-cert-${each.value}"
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
        dns_names = ["*.${each.value}"]
      }

      subject            = "CN=testcert"
      validity_in_months = 12
    }
  }
}