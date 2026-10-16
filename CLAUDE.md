# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Provider Does

A schema-driven Terraform **provider** for generating consistent Azure resource names following Microsoft's Cloud Adoption Framework (CAF) conventions. A YAML convention drives all naming logic — patterns, abbreviations, mappings and settings — so a new resource type is added without touching Go.

It unifies what used to be two Terraform modules:

| Old module | Registry address | Now |
|---|---|---|
| `terraform-random-naming-schema` | `LemurDaniel/naming-schema/random` | the `provider "naming"` block + `data "naming_schema"` |
| `terraform-random-naming` | `LemurDaniel/naming/random` | `data "naming_name"` |

Both repos are still published and live next to this one (`../terraform-random-naming`, `../terraform-random-naming-schema`), with their own convention YAML — a **separate copy** from this provider's embedded one. Changing one does not change the other.

**Rendered names must stay identical to the modules'.** That is what `internal/schemaengine/generate_test.go` guards: it carries the expected names from the modules' `.tftest.hcl` fixtures verbatim. A change that alters a rendered name forces resource replacement for every consumer, so it is breaking even when no signature moves.

## Commands

```bash
gofmt -l .          # see the line-endings note below
go build ./...
go vet ./...
go test ./...
```

Read [.claude/skills/go-tests/SKILL.md](.claude/skills/go-tests/SKILL.md) before writing or debugging a test, and [.claude/skills/install-dependencies/SKILL.md](.claude/skills/install-dependencies/SKILL.md) if a tool is missing.

> `gofmt -l .` lists `main.go` and the older files under `internal/schemaengine/` even when nothing is wrong: git checks them out with CRLF line endings (`core.autocrlf`), and gofmt wants LF. The committed content is LF. Do not "fix" it by rewriting those files — it produces a whole-file diff that hides the real change.

## Repository Layout

| Path | Role |
|---|---|
| `main.go` | provider server entrypoint |
| `provider.go` | the provider's own config (`convention`, `naming`, `parameters`, `random_seed`); resolves the schema in `Configure` and hands it to every data source |
| `data_source_name.go` | `naming_name` — renders names |
| `data_source_schema.go` | `naming_schema` — exposes the resolved tables |
| `internal/schemaengine/convention.go` | loads a bundled convention from the embedded FS |
| `internal/schemaengine/override.go` | loads the caller's override YAML from disk |
| `internal/schemaengine/resolve.go` | the merge primitives (`DeepMergeUserWins`, `Coalesce`, …) |
| `internal/schemaengine/schema.go` | the resolved `Schema` type and `Resolve` |
| `internal/schemaengine/generate.go` | the rendering engine — resource id, abbreviation, pattern, tokens, format |
| `internal/schemaengine/dynamic.go` | YAML value → `types.Dynamic` conversion |
| `internal/schemaengine/convention/default.naming.yaml` | the bundled convention, embedded via `//go:embed` |
| `docs/` | what the Terraform Registry renders — maintained by hand |
| `examples/` | `basic/` is a runnable scratchpad; `provider/` and `data-sources/*/` are the doc examples |

## Public Interface

Three things, all of them consumer-visible:

1. **The provider config and the data sources' schemas** — attribute names, types, optional/required/computed.
2. **The resolution semantics** — `Resolve` and `Generate`: which value a given convention + override + parameters combination resolves to, and which name it renders.
3. **The bundled convention YAML** — consumers' names are built from it.

A change to any of them is an interface change; see [.claude/skills/release/SKILL.md](.claude/skills/release/SKILL.md) for how that maps to a version bump.

## Architecture

`Configure` resolves the schema **once** and passes `*schemaengine.Schema` to both data sources through `ProviderData`. Resolving is pure and local — embedded YAML plus in-memory merging — so there is no client, no network and nothing in state, and every name is known at plan time.

This is why both surfaces are data sources rather than resources: rendering is deterministic, so there is nothing to persist. `<UNIQUE_ID_n>` comes from `random_seed` via a v5 UUID derived from it, which is what makes the unique id reproducible without state. The `naming_schema` **resource** of `v0.0.2` (which held a `random_uuid` in state) is gone.

## Merge Semantics

An override is merged **per key with the caller's value winning**, recursively (`DeepMergeUserWins`). Keys the override does not mention keep the bundled value.

- `abbreviations`, `mappings`, `patterns`, a nested `enforce_lower_case` — deep merged. `patterns` is three levels deep (global string, provider string, per-type object of kind → pattern) and merges at every level, so overriding one kind keeps its sibling kinds and the provider default.
- `index_modifier`, a scalar `enforce_lower_case` — replaced outright when set. A custom `0` is a legal override, so this is a nil check and never a truthiness check.
- `default_parameters` — three layers, later winning: bundled convention < override file < the provider's `parameters`.

A shallow merge would drop every sibling under the key it touches, and merging the other way round would ignore the override entirely. **Both were real bugs in the predecessor modules**, and both are covered by tests in `schema_test.go` — verified to fail when the merge direction or depth is mutated. Do not "simplify" the recursion away.

## Pattern Token Syntax

| Token | Meaning |
|---|---|
| `<PARAM>` | Required parameter — errors if missing |
| `<?PARAM;-%s>` | Optional parameter, dropped when unset, separator inside the token |
| `<PARAM;%02s>` | Printf-formatted parameter |
| `<TYPE>` | Resolved abbreviation (also `<ABBREVIATION>`) |
| `<NAMING_ID>` | The `naming_id`, falling back to `kind` |
| `<INDEX;%02s>` | Numeric index, shifted by `index_modifier` |
| `<UNIQUE_ID_n>` | First *n* chars of the `random_seed` UUID |

`applyFormat` implements the padding rather than delegating to `fmt.Sprintf`: Terraform's `format("%02s", 1)` yields `01`, while Go's `fmt` pads a string conversion with spaces. Keep it that way, or every `<INDEX;%02s>` silently changes.

## Resolution Hierarchy

Abbreviations (`Schema.Abbreviation`), first match winning: `<type>::<naming_id>` → `<type>::<kind>` → `<type>::default` → `<type>`.

Patterns (`Schema.Pattern`), under `patterns.<provider>`: `<type>.<naming_id>` → `<type>.<kind>` → `<type>.default` → `<provider>.default` → `patterns.default`. Pattern keys are matched **exactly** — a `*` is not expanded here.

Lower-casing (`Schema.LowerCaseFor`): `<provider>.<type pattern>` → `<provider>.default` → `default`, where a type pattern has `*` expanded to `.*` and is matched **unanchored**. Keys are tried in sorted order so a type matching several patterns always resolves the same way.

## Gotchas

- **`index_modifier` shifts every index.** The bundled convention uses `1`, so the first name ends in `01`. Expected names in tests and docs are wrong most often because of this.
- **`index.count` is how many names; `index.start` only shifts the first index.** `{ start = 2, count = 5 }` with `index_modifier: 1` yields five names numbered `03`–`07`. `Generate` iterates a pure offset from 0 because the start is already folded into the base index — adding the start again double-counts it.
- **Mappings are case-insensitive but not whitespace-insensitive.** The convention lists `westeurope` and `West Europe` separately precisely because the latter does not match the former.
- **A literal `--` in a finished name is collapsed to `-`**, which masks a missing optional-token separator.
- **A missing abbreviation or an unset required parameter fails the plan** rather than rendering an empty segment.
- **Most wildcard keys behave identically expanded or not**, because matching is unanchored (`"virtualMachines*"` already matches as a raw regex). Only a bare `"*"` actually distinguishes the two, which is why `TestLowerCaseForWildcardExpansion` uses it — a mutation test showed the obvious case proves nothing.
- **Never document a rendered name you have not rendered.** Build the provider and run a scratch config through `dev_overrides`; `TF_CLI_CONFIG_FILE` keeps that out of the user's global Terraform config.

## Updating Abbreviations

The abbreviations in `internal/schemaengine/convention/default.naming.yaml` are maintained by hand against Microsoft's [CAF abbreviation list](https://learn.microsoft.com/en-us/azure/cloud-adoption-framework/ready/azure-best-practices/resource-abbreviations).

A `generate.ps1` that scraped that page existed in the pre-provider history at `modules/naming-schema/generate.ps1`. It was deleted in `e0f27f8` and never carried over; recover it with `git show e0f27f8^:modules/naming-schema/generate.ps1` if wanted. Do not reference it as if it were present.

## Repository Conventions

Four skills under [.claude/skills/](.claude/skills/) carry the working rules — read the relevant one before acting:

- **[git-commit-conventions](.claude/skills/git-commit-conventions/SKILL.md)** — commits carry **no attribution trailers** (no `Co-Authored-By`, no "Generated with Claude Code"); English imperative subject; one concern per commit; default branch is `master`.
- **[release](.claude/skills/release/SKILL.md)** — a `v*` tag triggers GoReleaser, which builds and GPG-signs the artifacts the Registry serves. The release is not done until that workflow is green.
- **[go-tests](.claude/skills/go-tests/SKILL.md)** — the fixture patterns, the parity guard, and the `dev_overrides` loop.
- **[install-dependencies](.claude/skills/install-dependencies/SKILL.md)** — installing and verifying the toolchain.

The `origin` remote URL embeds a GitHub personal access token — never copy a remote URL into a tracked file, a commit message, or command output you paste back.
