# The naming schema resolved from the provider's configuration - the bundled
# convention with any override merged over it.
data "naming_schema" "this" {}

output "storage_account_abbreviation" {
  value = data.naming_schema.this.abbreviations.Azure["Microsoft.Storage/storageAccounts::default"]
  # "st"
}

output "storage_account_pattern" {
  value = data.naming_schema.this.patterns.Azure["Microsoft.Storage/storageAccounts"].default
  # "<TYPE><LOCATION><ENVIRONMENT><NAME><INDEX;%02s>"
}

output "index_modifier" {
  value = data.naming_schema.this.index_modifier
  # 1, so rendered indices start at 01
}
