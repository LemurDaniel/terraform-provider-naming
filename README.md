# Terraform Naming Schema Module

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Terraform Registry](https://img.shields.io/badge/registry-naming--schema%2Frandom-blue)](https://registry.terraform.io/modules/LemurDaniel/naming-schema/random/latest)

A schema-driven approach to consistent resource naming in Terraform. Define your naming conventions once in YAML — patterns, abbreviations, mappings, and settings — and consume the resolved schema from any module or provider.

This module loads and merges a naming convention (a bundled one, or your own YAML) with a set of runtime parameters, and outputs a single resolved schema object that other modules can build resource names from. It only resolves the *schema* — to actually render resource names from it, pair it with the [`LemurDaniel/naming/random`](https://registry.terraform.io/modules/LemurDaniel/naming/random/latest) module, as shown below.

## 🚀 Quick Start

```hcl
module "schema" {
  source  = "LemurDaniel/naming-schema/random"
  version = "~> 1.0"

  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}

module "storage_account_naming" {
  source  = "LemurDaniel/naming/random"
  version = "~> 1.0"

  schema   = module.schema
  resource = "Azure::Microsoft.Storage/storageAccounts"
}

output "storage_account_name" {
  value = module.storage_account_naming.name  # "stwedevmyapp01"
}
```

> [!NOTE]
> This module ([`LemurDaniel/naming-schema/random`](https://registry.terraform.io/modules/LemurDaniel/naming-schema/random/latest)) only produces the resolved schema object. Generating an actual name — like `storage_account_name` above — requires the companion [`LemurDaniel/naming/random`](https://registry.terraform.io/modules/LemurDaniel/naming/random/latest) module, which consumes that schema.

## 🙏 Acknowledgments

- 🟣 [HashiCorp](https://www.hashicorp.com/) for Terraform and the amazing IaC tooling
- ☁️ Microsoft for Azure and the extensive resource provider ecosystem
- 🔤 [CAF Naming Conventions](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations) for the abbreviation guidelines the bundled convention is built around
- 💖 Open Source contributors and the community for inspiration

## Why This Module?

- **Centralized Naming** — maintain conventions in one place, not scattered across modules.
- **Consistency** — every consumer resolves names from the same schema, regardless of provider or resource type.
- **Extensible** — add abbreviations, mappings, or patterns without touching any HCL.
- **YAML-driven** — swap the entire schema between environments without changing any resource code.
- **Partial overrides** — supply only the keys you want to change; everything else falls back to the base convention.

## 📦 Using the Schema Module on Its Own

Reference the module from the Terraform Registry, pinning a version:

```hcl
module "schema" {
  source  = "LemurDaniel/naming-schema/random"
  version = "~> 1.0"
  # ...
}
```

> [!NOTE]
> A full runnable example lives under [`examples/basic/`](examples/basic/). The snippets below use a local `./` source for brevity — replace `source` with the registry address above when consuming this module from another repository.

The module can be consumed in two ways:

1. **Convention** — pick one of the bundled conventions shipped with the module (no YAML file required).
2. **Custom YAML** — supply your own schema via the `naming` variable, partially overriding the bundled one.

### Option 1 — use a bundled convention

```hcl
module "schema" {
  source = "./"

  convention = "default"   # loads convention/default.naming.yaml
  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}
```

### Option 2 — provide a custom YAML

```hcl
module "schema" {
  source = "./"

  convention = "default"
  naming     = yamldecode(file("${path.root}/naming.yaml"))
  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}
```

> [!NOTE]
> The custom YAML overrides the bundled convention per top-level key (`patterns`, `abbreviations`, `mappings`, `enforce_lower_case`, `index_modifier`, `default_parameters`). Any key you leave unset in your custom YAML falls back to the value from `convention`. `patterns` is the one exception — when set, it is replaced wholesale rather than merged key by key, so copy the full block from the base convention if you only want to tweak one entry. This lets you ship a minimal file that only redefines what you actually need to change.

## ⚙️ How It Works

```
                ┌───────────────────────┐
   convention → │                       │
                │   naming-schema       │ →  resolved schema object
   naming     → │                       │    (patterns, abbreviations,
   parameters → │                       │     mappings, settings, ...)
                └───────────────────────┘
```

1. The module loads the bundled YAML for `convention` (e.g. `convention/default.naming.yaml`).
2. If `naming` is set, each of its top-level keys overrides (or, for maps like `abbreviations`/`mappings`, merges into) the bundled convention.
3. `parameters` are merged on top of `default_parameters` and passed straight through, so consumers get one map of resolved runtime values.
4. The result is exposed as a set of outputs describing the fully resolved schema — ready to be handed to a naming-generator or any other module that renders actual resource names from it.

### 🎨 Define your patterns

Patterns describe the shape of a generated name and are resolved by a consuming module (e.g. a naming-generator), not by this module itself. This module only stores and merges them.

```yaml
patterns:
  default: "<TYPE>-<LOCATION>-<ENVIRONMENT>-<NAME>-<INDEX;%02s>-<UNIQUE_ID_4>"

  Azure:
    default: "<TYPE>-<LOCATION>-<ENVIRONMENT>-<NAME><?SUBNAME;-%s>-<INDEX;%02s>"

    Microsoft.KeyVault/vaults:
      default: "<TYPE>-<ENVIRONMENT>-<NAME>-<UNIQUE_ID_4>"
```

Patterns resolve most-specific-first: `resource_type::naming_id` → `resource_type::kind` → `resource_type::default` → provider default → global default.

### 🔤 Define your abbreviations

Abbreviations map a resource type (optionally scoped by `::kind`) to its short form:

```yaml
abbreviations:
  Azure:
    Microsoft.Compute/disks::default: disk
    Microsoft.Compute/disks::os: osdisk
    Microsoft.KeyVault/vaults::default: kv
```

The bundled `default` convention ships with the full set of [CAF resource abbreviations](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations).

### 🗺️ Define your mappings

Mappings are case-insensitive lookup tables, typically used to shorten environment or location names:

```yaml
mappings:
  environment:
    development: dev
    production: prod

  location:
    westeurope: we
    West Europe: we
```

## 📥 Inputs

| Name | Type | Required | Description |
|---|---|---|---|
| `convention` | `string` | No (default `"default"`) | Name of the bundled convention to load from `convention/<name>.naming.yaml`. |
| `naming` | `object` | No | Custom schema, partially overriding the bundled convention. See the [object shape](variables.tf) for `index_modifier`, `enforce_lower_case`, `mappings`, `patterns`, `abbreviations`, `default_parameters`. |
| `parameters` | `map(any)` | No (default `{}`) | Runtime parameters (e.g. `location`, `environment`, `name`) merged on top of `default_parameters`. |

## 📤 Outputs

| Name | Description |
|---|---|
| `random` | A random source (`uuid`) for generating unique naming ids. |
| `index_modifier` | The resolved index offset (e.g. `1` so indices start at `01`). |
| `enforce_lower_case` | The resolved lower-casing settings. |
| `abbreviations` | The merged abbreviations table. |
| `mappings` | The merged mappings table. |
| `patterns` | The resolved patterns table. |
| `default_parameters` | The merged default parameters, combined with `var.parameters`. |

## Examples

A full runnable example lives under [`examples/basic/`](examples/basic/), demonstrating a custom YAML override on top of the bundled `default` convention (see [`naming.basic.yaml`](examples/basic/naming.basic.yaml)), plus a fully annotated reference file covering every configurable key (see [`naming.full.yaml`](examples/basic/naming.full.yaml)).

## License

[MIT](LICENSE)
