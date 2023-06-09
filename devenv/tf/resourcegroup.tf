resource "azurerm_resource_group" "rg" {
  name     = "app-routing-dev-${random_string.random.result}"
  location = var.location
  tags = {
    deletion_due_time  = time_static.provisiontime.unix + 36000, // keep resources for 10hr
    deletion_marked_by = "gc",
  }
}

resource "azurerm_resource_group" "rg-public" {
  name     = "app-routing-dev-${random_string.random.result}-public"
  location = var.location
  tags = {
    deletion_due_time  = time_static.provisiontime.unix + 36000, // keep resources for 10hr
    deletion_marked_by = "gc",
  }
}

resource "azurerm_resource_group" "rg-private" {
  name     = "app-routing-dev-${random_string.random.result}-public"
  location = var.location
  tags = {
    deletion_due_time  = time_static.provisiontime.unix + 36000, // keep resources for 10hr
    deletion_marked_by = "gc",
  }
}