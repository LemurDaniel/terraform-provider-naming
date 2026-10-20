# Terraform Provider: naming

> [!NOTE]
> **AI disclaimer:** Yes, an AI helped write this. No, it did not go rogue and name your storage account `skynet-prod-001`. The code is tested, the names are checked against the original modules. 🤖

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A schema-driven Terraform provider for consistent Azure resource naming, following Microsoft's [Cloud Adoption Framework (CAF)](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations) conventions. Define your naming convention once in YAML — patterns, abbreviations, mappings, and settings — and render names natively in Terraform, without any HCL modules in the middle.

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

# The convention is declared once, on the provider.
provider "naming" {
  convention = "default"

  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}

# ... and names are rendered from it wherever they are needed.
data "naming_name" "storage_account" {
  resource = "Azure::Microsoft.Storage/storageAccounts"
}

resource "azurerm_storage_account" "this" {
  name = data.naming_name.storage_account.name
  # "stwedevmyapp01"
}
```

A runnable version of this, exercising every feature, lives under [`examples/basic/`](examples/basic/).

## Why This Provider?

- **Centralized naming** — the convention lives in one place, not scattered across modules.
- **Consistency** — every consumer renders names from the same schema, regardless of resource type.
- **YAML-driven** — add abbreviations, mappings or patterns without touching any Go code, and swap the whole schema between environments without changing resource code.
- **Partial overrides** — supply only the keys you want to change; everything else falls back to the bundled convention.
- **Plan-time and stateless** — embedded YAML merged in memory. No API calls, nothing in state, every name known during `terraform plan`.

## 📦 What's in the Provider

| | Purpose |
|---|---|
| [`provider "naming"`](docs/index.md) | The convention: a bundled convention, an optional YAML override, the parameters, and the `random_seed` for `<UNIQUE_ID_n>`. |
| [`data "naming_name"`](docs/data-sources/name.md) | Renders one or more names from it. |
| [`data "naming_schema"`](docs/data-sources/schema.md) | The resolved abbreviations, patterns, mappings and settings, for inspection. |

### Provider arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `convention` | `string` | No (default `"default"`) | Name of the bundled convention to load. |
| `naming` | `string` | No | Path to a YAML file partially overriding the bundled convention. |
| `parameters` | `map(string)` | No | Runtime parameters (`location`, `environment`, `name`, ...) that patterns render from. |
| `random_seed` | `string` | No | Seed for `<UNIQUE_ID_n>`. The same seed always yields the same id, so names stay stable without state. |

### `naming_name` arguments and outputs

| Argument | Type | Required | Description |
|---|---|---|---|
| `resource` | `string` | **Yes** | `"<Provider>::<Namespace>/<Type>[::<kind>]"`, e.g. `"Azure::Microsoft.Compute/disks::os"`. `kind` defaults to `default`. |
| `naming_id` | `string` | No | Selects a named variant; takes precedence over `kind`. |
| `index` | `object({ start, count })` | No | `count` is **how many** names to render, `start` the first index. Defaults to one name at `0`. |
| `parameters` | `map(string)` | No | Parameters for this call, layered over the provider's. |

Outputs: `name` / `result` (the first name), `at_index` / `by_index` (all names).

## 🧬 Using a Custom Convention

```hcl
provider "naming" {
  convention = "default"
  naming     = "${path.module}/naming.yaml"
}
```

```yaml
# naming.yaml - changes one abbreviation and one pattern, keeps everything else
abbreviations:
  Azure:
    Microsoft.Storage/storageAccounts::default: mystg

patterns:
  Azure:
    Microsoft.Compute/disks:
      os: "<TYPE>-<NAME>-<ENVIRONMENT>"
```

Every table is merged **per key, with your value winning**. Keys you do not mention keep the bundled convention's value, at every level — including `patterns`, which is three levels deep, so overriding one kind keeps its sibling kinds and the provider-level default. `index_modifier` and a scalar `enforce_lower_case` are replaced outright when set.

`default_parameters` has three layers, each winning over the last: the bundled convention, the override file, then the provider's `parameters`.

See [`examples/basic/naming.yaml`](examples/basic/naming.yaml) for a minimal override and [`examples/basic/naming.full.yaml`](examples/basic/naming.full.yaml) for a full reference copy of the bundled convention.

## 📐 Naming Schema YAML

The bundled `"default"` convention is embedded in the provider binary; its source is [`internal/schemaengine/convention/default.naming.yaml`](internal/schemaengine/convention/default.naming.yaml). Top-level keys:

- **`index_modifier`** — global index offset (`1`, so indices start at `01`).
- **`enforce_lower_case`** — a global default plus optional per-provider and per-resource-type overrides.
- **`patterns`** — the pattern strings, falling back up the hierarchy to a global default.
- **`abbreviations`** — keyed `<provider>` → `"<Namespace>/<Type>::<kind>"` → abbreviation. The `::<kind>` suffix is how one type carries several abbreviations.
- **`mappings`** — case-insensitive lookup tables applied before a pattern renders (`westeurope` → `we`, `development` → `dev`). Unmapped values pass through unchanged.
- **`default_parameters`** — parameter defaults the provider's `parameters` override.

### Pattern Token Syntax

| Token | Meaning |
|---|---|
| `<PARAM>` | Required parameter — errors if missing |
| `<?PARAM;-%s>` | Optional parameter, dropped when unset, with its separator inside the token |
| `<PARAM;%02s>` | Printf-formatted parameter |
| `<TYPE>` | Resolved abbreviation for the resource (also `<ABBREVIATION>`) |
| `<NAMING_ID>` | The `naming_id`, falling back to `kind` |
| `<INDEX;%02s>` | Numeric index, shifted by `index_modifier` |
| `<UNIQUE_ID_n>` | First *n* characters of the `random_seed` UUID |

### Resolution Hierarchy

Abbreviations, first match winning: `<type>::<naming_id>` → `<type>::<kind>` → `<type>::default` → `<type>`.

Patterns, under `patterns.<provider>`: `<type>.<naming_id>` → `<type>.<kind>` → `<type>.default` → `<provider>.default` → `patterns.default`.

Lower-casing: `<provider>.<type pattern>` → `<provider>.default` → `default`. A key under a provider is a pattern rather than an exact type name — `*` stands for any characters, and a prefix such as `Microsoft.Storage/` already covers every type in that namespace. See [the provider docs](docs/index.md#resolution-hierarchy) for the details.

## 🔁 Migrating from the Modules

This provider supersedes two Terraform modules, which remain published but are no longer where new work happens:

- `LemurDaniel/naming-schema/random` — resolved the schema ([terraform-random-naming-schema](https://github.com/LemurDaniel/terraform-random-naming-schema))
- `LemurDaniel/naming/random` — rendered the names ([terraform-random-naming](https://github.com/LemurDaniel/terraform-random-naming))

```hcl
# before
module "schema" {
  source  = "LemurDaniel/naming-schema/random"
  version = ">= 1.1.0"

  naming     = yamldecode(file("${path.module}/naming.yaml"))
  parameters = { location = "westeurope", environment = "DEVELOPMENT", name = "TEST" }
}

module "naming_storage_account" {
  source  = "LemurDaniel/naming/random"
  version = ">= 2.0.0"

  schema   = module.schema
  resource = "Azure::Microsoft.Storage/storageAccounts"
}
# module.naming_storage_account.name

# after
provider "naming" {
  naming     = "${path.module}/naming.yaml"
  parameters = { location = "westeurope", environment = "DEVELOPMENT", name = "TEST" }
}

data "naming_name" "storage_account" {
  resource = "Azure::Microsoft.Storage/storageAccounts"
}
# data.naming_name.storage_account.name
```

Notable differences:

- `naming` takes a **file path**, not a decoded object — the provider reads and parses the YAML itself, so no `yamldecode(file(...))`.
- `<UNIQUE_ID_n>` comes from `random_seed` instead of a `random_uuid` resource, so it is reproducible and needs no state.
- `.at_index` is a real list rather than a tuple, so comparing it to a list literal works.
- `patterns` are merged per key, like the other tables. The `naming_schema` **resource** from `v0.0.2` is now a data source.

**Rendered names are unchanged.** The engine is a port of the modules' logic and is tested against their own fixtures — see [`internal/schemaengine/generate_test.go`](internal/schemaengine/generate_test.go), which carries the expected names from the modules' `.tftest.hcl` suites.

## 🛠️ Local Development

The provider isn't installed from the registry when hacking on it locally — Terraform needs to be pointed at your own build via [`dev_overrides`](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers).

1. Build the provider binary into your `GOBIN` (`go env GOPATH`/bin by default):

   ```bash
   go build -o "$(go env GOPATH)/bin/terraform-provider-naming" .
   ```

2. Add a `dev_overrides` block to the Terraform CLI config — `~/.terraformrc` on Linux/macOS, `%APPDATA%\terraform.rc` on Windows. The path is the **directory**, not the binary:

   ```hcl
   provider_installation {
     dev_overrides {
       "registry.terraform.io/LemurDaniel/naming" = "/home/<you>/go/bin"
     }
     direct {}
   }
   ```

   To avoid touching your global config, put that in any file and point `TF_CLI_CONFIG_FILE` at it instead.

3. Run Terraform against [`examples/basic/`](examples/basic/) — no `terraform init` needed while the override is active:

   ```bash
   cd examples/basic
   terraform plan
   ```

Rebuild after every code change (step 1) before re-running `plan` — Terraform silently uses whatever is in the override directory, so a stale binary looks like a change that did nothing.

## ✅ Tests

```bash
gofmt -l .
go build ./...
go vet ./...
go test ./...
```

The suite covers the merge contract and the rendering engine, including the predecessor modules' own fixtures as a parity guard.

## 🙏 Acknowledgments

- 🟣 [HashiCorp](https://www.hashicorp.com/) for Terraform and the [terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework)
- ☁️ Microsoft for Azure and the extensive resource provider ecosystem
- 🔤 [CAF Naming Conventions](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations) for the abbreviation guidelines the bundled convention is built around
- 💖 Open Source contributors and the community for inspiration
