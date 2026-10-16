terraform {
  required_providers {
    naming = {
      source = "LemurDaniel/naming"
    }
  }
}

# ----------------------------------------------------------------------------
# The naming convention is declared once, on the provider: a bundled
# convention, an optional YAML override merged over it, and the parameters
# every pattern renders from.
# ----------------------------------------------------------------------------

provider "naming" {
  convention = "default"
  naming     = "${path.module}/naming.yaml"

  # Pins <UNIQUE_ID_n> deterministically - the same seed always yields the
  # same id, so names stay stable without any state.
  random_seed = "example-dev"

  parameters = {
    location    = "westeurope"
    environment = "DEVELOPMENT"
    name        = "TEST"
  }
}

# ----------------------------------------------------------------------------
# Example 1: a single name.
#
# The resource string is "<PROVIDER>::<RESOURCE>[::<KIND>]". Storage accounts
# allow no hyphens, so their pattern has no separators.
# ----------------------------------------------------------------------------

data "naming_name" "storage_account" {
  resource = "Azure::Microsoft.Storage/storageAccounts"
}

output "storage_account" {
  value = data.naming_name.storage_account.name
}

# ----------------------------------------------------------------------------
# Example 2: an index range.
#
# index.count is how many names to render; index.start only shifts the first
# index. The convention's index_modifier (1) shifts every index on top of
# that, so start = 0 numbers the first name 01.
# ----------------------------------------------------------------------------

data "naming_name" "virtual_networks" {
  resource = "Azure::Microsoft.Network/virtualNetworks"

  index = {
    start = 0
    count = 3
  }
}

output "virtual_networks" {
  # ["vnet-we-dev-test-01", "vnet-we-dev-test-02", "vnet-we-dev-test-03"]
  value = data.naming_name.virtual_networks.at_index
}

# ----------------------------------------------------------------------------
# Example 3: kind and naming_id select a variant of the same resource type.
#
#   abbreviations, most specific first:
#     1. <type>::<naming_id>    <- naming_id has the highest priority
#     2. <type>::<kind>
#     3. <type>::default
#     4. <type>
#
#   patterns, under patterns.<provider>:
#     1. <type>.<naming_id>     <- naming_id has the highest priority
#     2. <type>.<kind>
#     3. <type>.default
#     4. <provider>.default
#     5. default
# ----------------------------------------------------------------------------

data "naming_name" "os_disk" {
  resource = "Azure::Microsoft.Compute/disks::os"
}

data "naming_name" "data_disk" {
  resource = "Azure::Microsoft.Compute/disks::data"
}

output "disks" {
  # kind picks a different abbreviation for the same type:
  # osdisk-... vs. disk-...
  value = {
    os   = data.naming_name.os_disk.name
    data = data.naming_name.data_disk.name
  }
}

# ----------------------------------------------------------------------------
# Example 4: per-call parameters and an optional token.
#
# Parameters given here are layered over the provider's. The <?SUBNAME;-%s>
# token carries its separator inside the token, so an absent subname leaves
# no stray hyphen behind.
# ----------------------------------------------------------------------------

data "naming_name" "app_service" {
  resource = "Azure::Microsoft.Web/sites"

  parameters = {
    name    = "frontend"
    subname = "api"
  }
}

output "app_service" {
  value = data.naming_name.app_service.name
}

# ----------------------------------------------------------------------------
# Example 5: a provider with no location.
#
# AzureAD resources are global, so that provider's pattern omits <LOCATION>.
# ----------------------------------------------------------------------------

data "naming_name" "aad_group" {
  resource = "AzureAD::Groups::security"
}

output "aad_group" {
  value = data.naming_name.aad_group.name
}

# ----------------------------------------------------------------------------
# Example 6: the resolved schema itself, for inspection.
# ----------------------------------------------------------------------------

data "naming_schema" "this" {}

output "resolved_schema" {
  value = {
    index_modifier = data.naming_schema.this.index_modifier
    storage_account = {
      abbreviation = data.naming_schema.this.abbreviations.Azure["Microsoft.Storage/storageAccounts::default"]
      pattern      = data.naming_schema.this.patterns.Azure["Microsoft.Storage/storageAccounts"].default
    }
  }
}
