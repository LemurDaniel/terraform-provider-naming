# Terraform Provider: naming

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A schema-driven Terraform provider for consistent Azure resource naming, following Microsoft's [Cloud Adoption Framework (CAF)](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations) conventions. Define your naming convention once in YAML — patterns, abbreviations, mappings, and settings — and resolve it natively in Terraform, without any HCL modules in the middle.

## 🚀 Quick Start

```hcl
terraform {
  required_providers {
    naming = {
      source  = "LemurDaniel/naming"
      version = "~> 1.0"
    }
  }
}

provider "naming" {}

resource "naming_schema" "this" {
  convention = "default"

  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}

output "storage_account_abbreviation" {
  value = naming_schema.this.abbreviations.Azure["Microsoft.Storage/storageAccounts::default"]
  # "st"
}

output "storage_account_pattern" {
  value = naming_schema.this.patterns.Azure["Microsoft.Storage/storageAccounts"].default
  # "<TYPE><LOCATION><ENVIRONMENT><NAME><INDEX;%02s>"
}
```

A runnable version of this lives under [`examples/basic/`](examples/basic/).

> [!NOTE]
> This provider resolves the naming **schema** — the merged abbreviations, patterns, and mappings tables plus a stable unique id. It does not itself render a final name string from a pattern; that's on the consumer (e.g. via `replace()`/`format()` in HCL, or a future companion resource/function). See [Pattern Token Syntax](#pattern-token-syntax) below for the token format the patterns use.

## 🙏 Acknowledgments

- 🟣 [HashiCorp](https://www.hashicorp.com/) for Terraform and the [terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework)
- ☁️ Microsoft for Azure and the extensive resource provider ecosystem
- 🔤 [CAF Naming Conventions](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations) for the abbreviation guidelines the bundled convention is built around
- 💖 Open Source contributors and the community for inspiration

## Why This Provider?

- **Centralized Naming** — maintain conventions in one place, not scattered across modules.
- **Consistency** — every consumer resolves names from the same schema, regardless of resource type.
- **Extensible** — add abbreviations, mappings, or patterns without touching any Go code.
- **YAML-driven** — swap the entire schema between environments without changing any resource code.
- **Partial overrides** — supply only the keys you want to change; everything else falls back to the bundled convention.
- **No module indirection** — a single provider-native resource, no `module` blocks or output plumbing required.

## 📦 The `naming_schema` Resource

```hcl
resource "naming_schema" "this" {
  convention  = "default"                      # bundled convention name (default: "default")
  naming      = "${path.module}/naming.yaml"   # optional partial override file
  random_seed = "myapp-prod"                   # optional, pins the generated id deterministically

  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}
```

### Inputs

| Attribute | Type | Required | Description |
|---|---|---|---|
| `convention` | `string` | No (default `"default"`) | Name of the bundled convention to load from `internal/schemaengine/convention/<name>.naming.yaml`. |
| `naming` | `string` | No | Path to a custom YAML file that partially overrides the bundled convention. |
| `parameters` | `dynamic` | No | Runtime parameters (`location`, `environment`, `name`, ...) available to consumers for rendering names. |
| `random_seed` | `string` | No | Seed for the generated id. The same seed always yields the same (deterministic) UUID. Unset means a random id is generated once on create and kept stable across updates. |

### Computed outputs

| Attribute | Description |
|---|---|
| `abbreviations` | The merged abbreviations table. |
| `mappings` | The merged mappings table (e.g. location/environment shortnames). |
| `patterns` | The resolved patterns table. |
| `index_modifier` | The resolved index offset (e.g. `1` so indices start at `01`). |
| `enforce_lower_case` | The resolved lower-casing settings. |
| `default_parameters` | Normalized copy of `parameters` (an empty object if `parameters` was left unset). |
| `random.uuid` | A random (or seeded) UUID, generated once and kept stable across updates. |
| `id` | Same value as `random.uuid`. |

All computed attributes are resolved at plan time (not just apply), as long as `convention`, `naming`, and `parameters` are already known — so `terraform plan` shows the real values instead of `(known after apply)`.

## 🧬 Using a Custom Convention

Provide your own YAML via `naming`, partially overriding the bundled convention:

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
abbreviations:
  Azure:
    Microsoft.Storage/storageAccounts::default: mystg

patterns:
  default: "<TYPE>-<LOCATION>-<ENVIRONMENT>-<NAME>-<INDEX;%02s>"
```

Merge semantics, per top-level key:

- **`abbreviations` / `mappings`** — deep-merged; your override wins per key, anything you don't touch falls back to the bundled convention.
- **`patterns`** — replaced **wholesale** when set, not merged key by key. Copy the full block from the bundled convention (see [`examples/basic/naming.full.yaml`](examples/basic/naming.full.yaml) for a full reference copy) if you only want to tweak one entry.
- **`index_modifier` / `enforce_lower_case`** — your override wins outright if set, otherwise falls back to the bundled convention's value.

See [`examples/basic/`](examples/basic/) for both a minimal partial override ([`naming.yaml`](examples/basic/naming.yaml)) and a full reference copy of the bundled convention ([`naming.full.yaml`](examples/basic/naming.full.yaml)).

## 📐 Naming Schema YAML

The bundled `"default"` convention lives at [`internal/schemaengine/convention/default.naming.yaml`](internal/schemaengine/convention/default.naming.yaml). It has these top-level keys:

- **`index_modifier`** — global default index offset (`1`, so indices start at `01`).
- **`enforce_lower_case`** — per-provider/resource-type lower-casing rules, cascading the same way as `patterns` (see below).
- **`patterns`** — per resource type/kind pattern strings; falls back up the hierarchy to a global default.
- **`abbreviations`** — per resource type/kind abbreviation strings, keyed as `"<Provider>"."<Namespace/Type>::<kind>"` (e.g. `Azure."Microsoft.Compute/disks::os"`).
- **`mappings`** — case-insensitive lookup tables (e.g. `location: { "West Europe": "euwe" }`, `environment: { development: "dev" }`).

To add a new Azure resource type, add an entry under `abbreviations` and optionally `patterns`.

### Pattern Token Syntax

Patterns use angle-bracket tokens:

| Token | Meaning |
|---|---|
| `<PARAM>` | Required parameter — errors if missing |
| `<?PARAM;-separator%s>` | Optional parameter with inline separator |
| `<PARAM;%02s>` | Printf-formatted parameter |
| `<TYPE>` | Resolved abbreviation for the resource |
| `<INDEX;%02s>` | Numeric index (shifted by `index_modifier`) |
| `<UNIQUE_ID_n>` | First *n* chars of a random UUID (`random.uuid` / `id`) |

### Resolution Hierarchy

Both abbreviations and patterns resolve via the same cascade:

1. `resource_type::naming_id` (most specific)
2. `resource_type::kind`
3. `resource_type::default`
4. `resource_type` (bare)
5. Provider-level default → global default (patterns only)

## 🛠️ Local Development

The provider isn't installed from the registry when hacking on it locally — Terraform needs to be pointed at your own build via [`dev_overrides`](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers).

1. Build the provider binary into your `GOBIN` (`go env GOPATH`/bin by default):

   ```bash
   go build -o ~/go/bin/terraform-provider-naming .
   ```

2. Add a `dev_overrides` block to `~/.terraformrc`:

   ```hcl
   provider_installation {
     dev_overrides {
       "registry.terraform.io/LemurDaniel/naming" = "/home/<you>/go/bin"
     }
     direct {}
   }
   ```

3. Run Terraform against [`examples/basic/`](examples/basic/) — no `terraform init` needed while the override is active:

   ```bash
   cd examples/basic
   terraform plan
   ```

Rebuild after every code change (step 1) before re-running `plan`/`apply`.

## ✅ Tests

```bash
go build ./...
go vet ./...
```
