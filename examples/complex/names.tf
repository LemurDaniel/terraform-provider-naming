# A single name per resource, rendered from the provider's parameters.
data "naming_name" "resource_group" {
  resource = "Azure::Microsoft.Resources/resourceGroups"
}

data "naming_name" "virtual_network" {
  resource = "Azure::Microsoft.Network/virtualNetworks"
}

data "naming_name" "key_vault" {
  resource = "Azure::Microsoft.KeyVault/vaults"
}

# A type added by naming.yaml, no provider change needed.
data "naming_name" "redis" {
  resource = "Azure::Microsoft.Cache/redis"
}

# for_each over a variable: the subname parameter fills the pattern's optional
# <?SUBNAME;-%s> token.
data "naming_name" "subnet" {
  for_each = var.subnets
  resource = "Azure::Microsoft.Network/virtualNetworks/subnets"

  parameters = {
    subname = each.key
  }
}

# An index range: count is how many names, start only shifts the first index.
data "naming_name" "virtual_machine" {
  resource = "Azure::Microsoft.Compute/virtualMachines"

  index = {
    start = 0
    count = var.vm_count
  }
}

# kind picks a variant of the same type, here a different abbreviation ...
data "naming_name" "os_disk" {
  resource = "Azure::Microsoft.Compute/disks::os"
  index    = { count = var.vm_count }
}

data "naming_name" "data_disk" {
  resource = "Azure::Microsoft.Compute/disks::data"
  index    = { count = var.vm_count }
}

# ... and may pick its own pattern (see naming.yaml).
data "naming_name" "log_storage" {
  resource = "Azure::Microsoft.Storage/storageAccounts::logs"
}

data "naming_name" "session_redis" {
  resource = "Azure::Microsoft.Cache/redis::session"
}

# naming_id selects a named pattern and takes precedence over kind.
data "naming_name" "vm_storage" {
  resource  = "Azure::Microsoft.Storage/storageAccounts::vm"
  naming_id = "vm_pattern"
}

# Per-call parameters are layered over the provider's.
data "naming_name" "other_region_vnet" {
  resource = "Azure::Microsoft.Network/virtualNetworks"

  parameters = {
    location = "germanywestcentral"
  }
}

# A provider without a location: AzureAD resources are global.
data "naming_name" "aad_group" {
  resource = "AzureAD::Groups::security"
}
