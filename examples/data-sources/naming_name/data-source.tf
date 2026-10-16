# A single name.
data "naming_name" "storage_account" {
  resource = "Azure::Microsoft.Storage/storageAccounts"
}

# A variant of the same resource type: kind picks a different abbreviation
# ("disks::os" => osdisk, "disks::data" => disk) and may pick its own pattern.
data "naming_name" "os_disk" {
  resource = "Azure::Microsoft.Compute/disks::os"
}

# naming_id selects a named variant and takes precedence over kind.
data "naming_name" "vm_storage_account" {
  resource  = "Azure::Microsoft.Storage/storageAccounts::vm"
  naming_id = "vm_pattern"
}

# A set of sequentially numbered names. count is how many to render, start
# only shifts the first index.
data "naming_name" "virtual_networks" {
  resource = "Azure::Microsoft.Network/virtualNetworks"

  index = {
    start = 0
    count = 3
  }
}

# Per-call parameters are layered over the provider's.
data "naming_name" "app_service" {
  resource = "Azure::Microsoft.Web/sites"

  parameters = {
    name    = "frontend"
    subname = "api"
  }
}

output "storage_account" {
  value = data.naming_name.storage_account.name
}

output "virtual_networks" {
  value = data.naming_name.virtual_networks.at_index
}
