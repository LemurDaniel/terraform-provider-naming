resource "naming_schema" "this" {
  convention = "default"

  # Optional partial override - see examples/basic/naming.yaml for the
  # merge semantics (deep-merged abbreviations/mappings, wholesale-replaced
  # patterns).
  # naming = "${path.module}/naming.yaml"

  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}

output "storage_account_abbreviation" {
  value = naming_schema.this.abbreviations.Azure["Microsoft.Storage/storageAccounts::default"]
}

output "storage_account_pattern" {
  value = naming_schema.this.patterns.Azure["Microsoft.Storage/storageAccounts"].default
}
