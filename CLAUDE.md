# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Module Does

This is a schema-driven Terraform module for generating consistent Azure resource names following Microsoft's Cloud Adoption Framework (CAF) conventions. A YAML schema drives all naming logic — patterns, abbreviations, mappings, and settings — so new resource types can be added without changing HCL.

## Commands

```bash
# Run all tests (requires Terraform 1.8+)
terraform test

# Run tests in a specific module
terraform test -chdir=modules/naming-generator

# Initialize before first use
terraform init
```

## Module Architecture

Three modules compose the system:

**`modules/naming-schema/`** — Loads and merges YAML naming conventions. Accepts a built-in convention name (`"default"`) or raw YAML as a string, plus a `parameters` map (location, environment, name, etc.). Outputs the full resolved schema object (abbreviations, patterns, mappings, settings) that naming-generator consumes.

**`modules/naming-generator/`** — Consumes a schema object and a `resource` identifier string to produce one or more names. Key inputs:
- `schema` — output of naming-schema
- `resource` — format `"Provider::Type::kind"` (e.g., `"Azure::Microsoft.Compute/disks::os"`)
- `naming_id` — optional variant selector for alternate patterns/abbreviations
- `index` / `index_end` — range for generating multiple names

**`modules/resourceGroup/`** — Example consumer module; builds an `azurerm_resource_group` using `naming-generator`, kept in sync with its variable/output structure.

The repository root has no root module — this is a Terraform Registry module published as a collection of submodules under `modules/`, per the [standard module structure](https://developer.hashicorp.com/terraform/registry/modules/publish#standard-module-structure). `examples/basic01/`, `examples/basic02/`, `examples/basic03/` are runnable scratchpad configs exercising the modules together, not consumable modules themselves.

## Naming Schema YAML

The default schema lives at `modules/naming-schema/convention/default.naming.yaml`. It has four top-level keys:

- **`settings`** — global defaults (`separator`, `enforce_lower_case`, `index_modifier: 1` so indices start at 01)
- **`abbreviations`** — per resource type/kind/naming_id abbreviation strings; also carries `enforce_lower_case` and `separator` overrides
- **`patterns`** — per resource type/kind/naming_id pattern strings; falls back up the hierarchy to a global default
- **`mappings`** — case-insensitive lookup tables (e.g., "West Europe" → "euwe", "development" → "dev")

To add a new Azure resource type, add entries under `abbreviations` and optionally `patterns`.

## Pattern Token Syntax

Patterns use angle-bracket tokens rendered by `locals.generate.tf`:

| Token | Meaning |
|---|---|
| `<PARAM>` | Required parameter — errors if missing |
| `<?PARAM;-separator%s>` | Optional parameter with inline separator |
| `<PARAM;%02s>` | Printf-formatted parameter |
| `<TYPE>` | Resolved abbreviation for the resource |
| `<INDEX;%02s>` | Numeric index (shifted by `index_modifier`) |
| `<UNIQUE_ID_n>` | First *n* chars of a random UUID |

## Resolution Hierarchy

Both abbreviations and patterns resolve via the same cascade:

1. `resource_type::naming_id` (most specific)
2. `resource_type::kind`
3. `resource_type::default`
4. `resource_type` (bare)
5. Provider-level default → global default (patterns only)

`resource` identifier parsing happens in `locals.parameter.tf`: `Provider::Namespace/type::kind` splits into provider, resource type, and kind.

## Tests

Tests use the Terraform 1.8+ native test framework (`.tftest.hcl`). The three test files in `modules/naming-generator/tests/` cover vnet, subnet, and storage account naming (the latter verifies no-separator behavior). Add new `.tftest.hcl` files there for new resource types.

## Updating Abbreviations

`modules/naming-schema/generate.ps1` scrapes Microsoft's CAF documentation to regenerate abbreviations in the YAML. Run it in PowerShell when Microsoft updates their naming recommendations.
