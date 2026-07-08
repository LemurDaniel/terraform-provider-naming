---
page_title: "naming Provider"
subcategory: ""
description: |-
  Schema-driven Azure/CAF resource naming. The naming_schema resource resolves a naming schema and persists a unique id. This provider takes no configuration.
---

# naming Provider

> [!WARNING]
> **Work in progress.** This provider currently only resolves the naming **schema** (abbreviations, patterns, mappings, a stable id) via `naming_schema` - it does not yet render a final resource name string from a pattern. That part of the old module-based workflow (a `naming-generator` equivalent) has not been ported to this provider yet. Interfaces and resource shapes may still change before the first stable release.

Schema-driven Azure/CAF resource naming, following Microsoft's [Cloud Adoption Framework (CAF)](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations) conventions. The [`naming_schema`](resources/schema.md) resource resolves a naming schema (abbreviations, patterns, mappings) from a bundled convention plus an optional partial override, and persists a unique id for use in generated names.

This provider takes no configuration itself - see the [`naming_schema`](resources/schema.md) resource for all available inputs.

## Example Usage

```terraform
terraform {
  required_providers {
    naming = {
      source = "LemurDaniel/naming"
    }
  }
}

provider "naming" {}
```

## Custom Naming Convention

Pass a custom YAML file via the `naming` attribute on `naming_schema` to partially override the bundled convention. Any key you leave unset falls back to the bundled convention - except `patterns`, which is replaced wholesale when set, not merged key by key.

```hcl
resource "naming_schema" "this" {
  convention = "default"
  naming     = "${path.module}/naming.yaml"

  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}
```

```yaml
# naming.yaml

# Global default index offset, so indices start at 01 instead of 00.
index_modifier: 1

abbreviations:
  Azure:
    Microsoft.Storage/storageAccounts::default: mystg
    Microsoft.KeyVault/vaults::default: mykv

# `patterns` is replaced wholesale when set - include every entry you need,
# not just the one you're changing.
patterns:
  default: "<TYPE>-<LOCATION>-<ENVIRONMENT>-<NAME>-<INDEX;%02s>-<UNIQUE_ID_4>"

  Azure:
    default: "<TYPE>-<LOCATION>-<ENVIRONMENT>-<NAME><?SUBNAME;-%s>-<INDEX;%02s>"

    Microsoft.Storage/storageAccounts:
      default: "<TYPE><LOCATION><ENVIRONMENT><NAME><INDEX;%02s>"

    Microsoft.KeyVault/vaults:
      default: "<TYPE>-<ENVIRONMENT>-<NAME>-<UNIQUE_ID_4>"

    Microsoft.Network/virtualNetworks:
      default: "<TYPE>-<LOCATION>-<ENVIRONMENT>-<NAME>-<INDEX;%02s>"

# Case-insensitive lookup tables consumers can use to shorten parameter
# values before feeding them into a pattern.
mappings:
  location:
    westeurope: we
    West Europe: we
    germanywestcentral: gwc
    Germany West Central: gwc

  environment:
    development: dev
    staging: stg
    production: prod
```

See [`examples/basic/naming.yaml`](https://github.com/LemurDaniel/terraform-provider-naming/blob/master/examples/basic/naming.yaml) for another minimal override, and [`examples/basic/naming.full.yaml`](https://github.com/LemurDaniel/terraform-provider-naming/blob/master/examples/basic/naming.full.yaml) for a full reference copy of the bundled convention to copy from.
