
locals {
  parent_path    = abspath(format("%s/convention/%s.naming.yaml", path.module, var.convention))
  default_naming = yamldecode(file(local.parent_path))

  # This is to bypass consistent-type errors with coalesce and inline-if.
  naming = [
    var.naming,
    local.default_naming
  ][var.naming == null ? 1 : 0]
}

resource "random_uuid" "id" {}

output "random" {
  description = "Outputs a random source for unique naming ids."
  value = {
    uuid = random_uuid.id.result
  }
}


output "index_modifier" {
  description = "Output the index modifier"
  value       = coalesce(local.naming.index_modifier,  local.default_naming.index_modifier)
}

output "enforce_lower_case" {
  description = "Output lowercase settings."
  value = coalesce(local.naming.enforce_lower_case,  local.default_naming.enforce_lower_case)
}



output "abbreviations" {
  description = "Output the resources part of the schema."
  value       = merge(coalesce(local.naming.abbreviations, {}), local.default_naming.abbreviations)
}

output "mappings" {
  description = "Output the mappings for resources."
  value       = merge(coalesce(local.naming.mappings, {}), local.default_naming.mappings)
}

output "patterns" {
  description = "Output the patterns part of the schema."
  value       = local.naming.patterns
}



output "default_parameters" {
  description = "Output the default parameters for the schema."
  value       = merge(coalesce(local.naming.default_parameters, {}), var.parameters)
}
