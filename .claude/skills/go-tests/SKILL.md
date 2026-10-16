---
name: go-tests
description: Write, run, and fix the tests for this Terraform provider — Go unit tests for the schema engine, acceptance tests for the naming_schema resource, and the dev_overrides loop for verifying a real terraform plan. Use when adding a test for a merge behaviour, a convention key or a resource attribute, when `go test` or a plan fails, when a resolved schema value needs to be asserted, or when the user asks to test / verify / validate the provider ("add a test", "test schreiben", "tests laufen lassen", "warum failt der test", "läuft das überhaupt").
---

# Testing the naming provider

## The two layers

| Layer | Package | What it covers | Needs Terraform |
|---|---|---|---|
| Unit | `internal/schemaengine` | convention loading, the merge contract, the rendering engine — `schema_test.go` and `generate_test.go` | no |
| Acceptance | `main` (root) | the provider config and data sources through a real plan | yes (`TF_ACC=1`) |

The unit layer exists and is green. **The root package has no tests**, so
`go test ./...` reports "no test files" for it — that is not a pass for the
provider surface (schema wiring, `Configure`, attribute plumbing), only for the
engine.

### The parity guard

`generate_test.go` carries the expected names from the predecessor modules'
own `.tftest.hcl` fixtures, verbatim. Its job is to prove the Go engine renders
exactly what `LemurDaniel/naming/random` rendered, because a changed name
forces resource replacement for every consumer.

**Treat a failure there as a regression in the engine, not a stale
expectation.** Only change one of those expected names if the intent really is
to change what consumers get — and then say so in the commit body and bump
accordingly ([release](../release/SKILL.md)).

```bash
gofmt -l .          # must print nothing
go build ./...
go vet ./...
go test ./...
go test ./... -run TestDeepMerge -v    # one test, verbose
```

> [!IMPORTANT]
> If any of these reports "not recognized" / "command not found", the Go
> toolchain is missing or not on `PATH` — **invoke the
> [install-dependencies](../install-dependencies/SKILL.md) skill** instead of
> working around it. Without Go, the `dev_overrides` loop below is unavailable
> too, since it needs `go build` to produce the binary. Report an unrun gate as
> **"could not run"** — never as a pass, and never quietly skip it.

> [!NOTE]
> `gofmt -l .` lists `main.go` and the older files under
> `internal/schemaengine/` even when nothing is wrong: git checks them out with
> CRLF line endings (`core.autocrlf`) and gofmt wants LF, while the committed
> content is LF. Run `gofmt -l` on the files you actually touched, and leave the
> rest — `gofmt -w` on them produces a whole-file diff that buries the real
> change. Check with `gofmt -d <file>`: an all-lines diff is the line-ending
> artifact, not formatting drift.

## Unit tests: start with the schema engine

`internal/schemaengine` is pure Go over `map[string]interface{}` with no
Terraform types in three of its four files — the cheapest and highest-value
place to test. Build the inputs as literal maps, exactly the shape
`yaml.Unmarshal` into `interface{}` produces:

```go
func TestDeepMergeUserWinsKeepsSiblings(t *testing.T) {
	base := map[string]interface{}{
		"Azure": map[string]interface{}{
			"Microsoft.Storage/storageAccounts::default": "st",
			"Microsoft.Network/virtualNetworks::default": "vnet",
		},
	}
	override := map[string]interface{}{
		"Azure": map[string]interface{}{
			"Microsoft.Storage/storageAccounts::default": "mystg",
		},
	}

	got := schemaengine.DeepMergeUserWins(base, override)
	azure := got["Azure"].(map[string]interface{})

	if azure["Microsoft.Storage/storageAccounts::default"] != "mystg" {
		t.Errorf("override lost: got %#v", azure)
	}
	if azure["Microsoft.Network/virtualNetworks::default"] != "vnet" {
		t.Errorf("sibling dropped: got %#v", azure)
	}
}
```

Behaviours worth one test each — these are the ones that were genuinely wrong
in the predecessor modules, so they earn their keep:

- **The override wins, the base does not.** The module version originally passed
  the default *last* to `merge()` and silently ignored every custom value.
- **Siblings survive a nested override** (the test above). A shallow merge drops
  the whole `Azure` table.
- **A nil override is a no-op**, not a wipe: `LoadOverrideFile("")` returns
  `(nil, nil)` and `DeepMergeUserWins(base, nil)` must return `base`'s content.
- **`GetKey` on a nil map returns nil** rather than panicking — that nil map is
  the normal "no `naming` file" case, not an edge case.
- **`Coalesce` picks the first non-nil**, and a present-but-falsy value wins:
  `index_modifier: 0` in an override must beat the convention's `1`. A test that
  only uses truthy values would pass against a buggy truthiness check.
- **`LoadConvention` rejects an unknown name** with the "was not found" error,
  and `LoadConvention("default")` succeeds from the embedded FS. The `//go:embed
  convention/*.naming.yaml` directive means a renamed or moved YAML fails at
  **build** time, but a *missing* name only fails at runtime.
- **The bundled convention still decodes and holds what consumers rely on** —
  `index_modifier == 1`, `enforce_lower_case.default == true`, a known
  abbreviation, a known mapping. This doubles as a regression guard on the YAML
  itself, which nothing else checks.
- **`ToDynamic` handles every type the YAML produces** — nested maps, lists,
  strings, bools, ints, floats, nil — and **returns an error for an unsupported
  type** instead of panicking. Note `yaml.v3` decodes integers as `int`, so the
  `int` case is the one that actually fires for `index_modifier`; `float64` is
  reachable but rarer.
- **`DeterministicUUID` is stable and seed-dependent**: the same seed yields the
  same UUID across calls, two different seeds differ, and the value is a
  well-formed v5 UUID. This is the whole contract of `random_seed`.
- **The rendering cascades each fall back one step at a time** — the
  abbreviation (`::naming_id` → `::kind` → `::default` → bare), the pattern
  (type/naming_id → type/kind → type/default → provider default → global
  default) and the lower-case resolution. Cover each level, not just the
  specific and the global one.

Keep each fixture **minimal and local to what it asserts** — a handful of keys,
not a copy of the whole convention. A failing test should point at one
behaviour. Use `t.Run` subtests for table cases, and always put the **actual**
value in the failure message (`%#v` for maps) — a bare `t.Fail()` tells you
nothing.

## Verify a new test can fail

A test that passes on its first run has not been shown to test anything. Mutate
the behaviour it targets, confirm that run — ideally only that run — goes red,
then restore. Copy the file first (`Copy-Item ... $env:TEMP`) so the restore is
exact.

The merge direction, the merge depth, the index modifier and the optional-token
handling each make specific, different runs fail today, so the suite is known to
bite. One mutation taught a real lesson worth repeating:

> Removing the `*` → `.*` expansion in `LowerCaseFor` changed **nothing**,
> because matching is unanchored and `"virtualMachines*"` already matches as a
> raw regex (`s*` = zero or more `s`). The obvious test case proved nothing. A
> bare `"*"` catch-all key is the only real discriminator — as a raw regex it is
> invalid and matches nothing, expanded it matches everything.

So pick the fixture that *distinguishes* the behaviour, not merely one that
exercises it.

## Acceptance tests: the provider through a real plan

`provider.go` and the two data sources cannot be meaningfully unit-tested
through the framework's types by hand; they need `terraform-plugin-testing`,
which is **not yet a dependency**:

```bash
go get github.com/hashicorp/terraform-plugin-testing
```

That is a `go.mod` change and belongs in the commit that adds the first
acceptance test (see [git-commit-conventions](../git-commit-conventions/SKILL.md)).

Acceptance tests run a real Terraform binary, so they only execute when
`TF_ACC` is set — without it they skip silently, which is easy to misread as a
pass:

```bash
TF_ACC=1 go test ./... -v          # bash
$env:TF_ACC = "1"; go test ./... -v   # PowerShell
```

What the acceptance layer is for — the things no unit test can see:

- **A name resolves during `plan`, not as `(known after apply)`.** Everything is
  computed in `Configure`/`Read` from embedded YAML, so a plan must already show
  the finished name. Assert it with `ConfigPlanChecks` /
  `plancheck.ExpectKnownValue`, not with a post-apply state check — an apply
  passes either way, so a state-only assertion cannot detect the regression.
- **The provider's `parameters` reach the data source**, and a data source's own
  `parameters` win over them. That layering crosses the `ProviderData` boundary,
  which is exactly what unit tests stub out.
- **An invalid `convention` and an unreadable `naming` path produce attribute
  errors** on the right attribute — use `ExpectError` with a regexp against the
  message from `LoadConvention` / `LoadOverrideFile`.
- **Unknown provider configuration produces the documented error** rather than a
  panic or a silent empty schema. Derive `naming` from a resource attribute to
  trigger it.
- **`at_index` is a list**, so `index = { count = 3 }` yields three elements a
  `knownvalue.ListExact` can check — one of the reasons for moving off the
  module's tuple output.

Point the test at a temp-directory YAML file for the `naming` attribute rather
than at `examples/basic/naming.yaml`, so the test does not break when the
example is edited.

## Verifying a real plan by hand (dev_overrides)

This is the fallback while the suite is thin, and the mandatory release gate in
[release](../release/SKILL.md). The provider is **not** installed from the
Registry when hacking locally; Terraform has to be pointed at your own build.

1. Build the binary into your `GOBIN` (`go env GOPATH`/bin by default):

   ```bash
   go build -o "$(go env GOPATH)/bin/terraform-provider-naming" .
   ```

2. Write a `dev_overrides` block. The path must be the **directory**, not the
   binary:

   ```hcl
   provider_installation {
     dev_overrides {
       "registry.terraform.io/LemurDaniel/naming" = "C:/Users/<you>/go/bin"
     }
     direct {}
   }
   ```

   **Prefer `TF_CLI_CONFIG_FILE` pointing at a scratch file over editing the
   user's CLI config** (`%APPDATA%\terraform.rc` on Windows — *not*
   `~/.terraformrc`, which is the Unix path the README shows). The env var
   leaves their global Terraform setup untouched, which matters because a
   `dev_overrides` entry silently redirects this provider for every project on
   the machine:

   ```powershell
   $env:TF_CLI_CONFIG_FILE = "<scratch>/dev.tfrc"
   ```

3. Run the example — **no `terraform init`** while the override is active, and
   Terraform prints a warning about the override on every command, which is
   expected:

   ```bash
   cd examples/basic
   terraform plan
   ```

**Rebuild after every code change before re-running.** A stale binary is the
single most common cause of "my change did nothing": Terraform silently uses
whatever is in the override directory, so the plan reflects the last build, not
the working tree.

What to look at in the output:

- Every name shows as a **real value**, not `(known after apply)`. Each data
  source also prints its rendered name in its "Read complete" line, which is
  the quickest way to eyeball a whole set at once.
- `examples/basic/main.tf` exercises a single name, an index range, both disk
  kinds, an optional token, a second provider and the resolved schema — so one
  plan covers every cascade.
- A `terraform apply` is safe: the provider declares no resources and reaches
  nothing outside Terraform.
- A plan is also how you confirm a **documented** name: never write a rendered
  name into the README, the docs or an example comment without having seen it
  here.

## Deriving an expected value

Work it out from the YAML, then **run it to confirm** — never commit an expected
value reasoned out by hand. The traps, in the order they bite:

- **`index_modifier` is `1`** in the bundled convention, so the first name ends
  in `01`, not `00`. This is the single most common wrong expectation.
- **`index.count` is how many names; `index.start` only shifts the first
  index.** `{ start = 2, count = 5 }` yields five names numbered `03`–`07`.
  Assert the length as well as the contents.
- **`enforce_lower_case.default` is `true`**, so a name comes out lower-cased
  whatever the input casing. Set it explicitly in a fixture instead of relying
  on the bundled value.
- **Mappings are case-insensitive but not whitespace-insensitive.**
  `environment = "test"` renders as `tst`, while `location = "West Europe"`
  does *not* match the key `westeurope` and passes through unchanged.
- **Every table is deep-merged per key**, `patterns` included, at all three of
  its levels. An override setting one pattern keeps the siblings — so a fixture
  asserting "the override replaced the block" is asserting a bug.
- **A literal `--` is collapsed to `-`.** Do not write a test that depends on
  that; test the optional-token form (`<?SUBNAME;-%s>`) instead.
- **A missing abbreviation or an unset required parameter is an error**, not an
  empty segment, so a fixture must define an abbreviation for every resource it
  names.
- **`<UNIQUE_ID_n>` needs a seed.** In a `Schema` literal set `UniqueID`
  directly to keep it deterministic; through `Resolve` it comes from
  `RandomSeed`, and an unseeded schema makes any pattern containing the token
  fail.

## Related

- A test change is its own commit — see
  [git-commit-conventions](../git-commit-conventions/SKILL.md).
- Build, vet, test and the manual plan are all gates before tagging a version —
  see [release](../release/SKILL.md).
