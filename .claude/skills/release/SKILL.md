---
name: release
description: Cut a new version of this Terraform provider and tag it so the GoReleaser workflow builds it and the Terraform Registry picks it up. Use when the user wants to release, publish, cut, bump, or tag a new provider version ("release 1.0.0", "neue version", "bump patch", "tag a release", "publish to registry"), or asks which version is current / what changed since the last tag.
---

# Releasing a new provider version

This repository is published on the Terraform Registry as the **provider**
`LemurDaniel/naming`. Unlike a module, a provider release is not just a tag —
the tag triggers [`.github/workflows/release.yml`](../../../.github/workflows/release.yml),
which runs GoReleaser to cross-compile the binaries, sign the checksums with
GPG, and attach everything plus `terraform-registry-manifest.json` to a GitHub
release. **The Registry serves those signed artifacts**; a tag without a
successful workflow run publishes nothing.

> **Any commit this process creates follows
> [git-commit-conventions](../git-commit-conventions/SKILL.md) — read it before committing.**
> In particular: no `Co-Authored-By` or "Generated with Claude Code" trailers, English
> imperative subject, and a body explaining the why. The same rule applies to the tag's
> annotation message and to any GitHub release notes.

## Tagging rules that are enforced

Get these wrong and the version silently never appears:

- The tag must be **semantic versioning**: `x.y.z`, three numeric segments.
- The tag must be **`v`-prefixed** — this repo uses `v0.0.1`, `v0.0.2`, and the
  workflow only triggers `on: push: tags: ["v*"]`. A bare `1.0.0` tag builds
  nothing at all. (This is stricter than the module repos, where the prefix is
  optional.)
- A pre-release suffix (`v1.0.0-rc1`) is published but never resolved as
  `latest` and is not matched by `~>` constraints. Only cut one if asked.
- The tag must be pushed to the remote; a local tag runs no workflow.
- Never move or delete a published tag. The release assets and their signatures
  are already bound to it and the Registry caches them. A mistake is fixed by
  cutting the next patch version, not by re-tagging.

## Steps

### 1. Establish the current version

```bash
git fetch --tags --prune
git tag --sort=-v:refname | head -5
```

Take the highest tag as the current version. The repo is currently at `v0.0.2`
— i.e. still **pre-1.0**, which matters for the bump decision below. Always bump
from the **highest** existing tag, not from the count; gaps are normal.

### 2. Decide the bump

If the user named a version or a bump level, use it. Otherwise look at what
changed and propose one, stating the reason:

```bash
git log <last-tag>..HEAD --oneline
git diff <last-tag>..HEAD --stat
```

This provider's **public interface** is three things:

1. the **provider configuration and the data sources' schemas**
   (`provider.go`, `data_source_name.go`, `data_source_schema.go`) — attribute
   names, types, and their optional/required/computed markers;
2. the **resolution and rendering semantics** — `Resolve` and `Generate` in
   `internal/schemaengine/` — which schema a given `convention` + `naming` +
   `parameters` combination resolves to, and which **name** it renders;
3. the **bundled convention** in
   `internal/schemaengine/convention/default.naming.yaml`, because consumers
   render resource names from it.

- **major** — an attribute was removed or renamed, an attribute became required,
  its type changed, the provider name or address changed, an existing
  abbreviation/pattern/mapping in the bundled convention **changed value**, or
  the merge semantics changed so that unchanged configuration now resolves to a
  different schema. A changed resolved value changes a generated name, and
  renaming a name a consumer already applied forces resource replacement — so
  treat it as breaking even though no Go signature moved.
- **minor** — a new optional attribute, a new computed attribute, a new resource
  or provider-defined function, a new resource type / abbreviation / pattern /
  mapping **added** to the convention, a new bundled convention file, new
  behaviour that leaves existing configurations resolving the same.
- **patch** — bug fixes with no resolved-value change, docs, comments, examples,
  dependency bumps, release tooling; nothing a consumer's plan would notice.

**While the version is `0.y.z`**, semver puts no compatibility promise on it and
`docs/index.md` carries an explicit work-in-progress warning. A breaking change
can go into a `0.x` bump rather than forcing `v1.0.0` — but say so plainly, and
ask before cutting the **first `v1.0.0`**, since that is the point the interface
becomes a promise. `~> 1.0` constraints in `README.md` do not resolve any `0.x`
release at all; if the user wants the quick-start snippet to work, that is a
reason to go to `v1.0.0`, not an accident to paper over.

Commit bodies written per [git-commit-conventions](../git-commit-conventions/SKILL.md)
state whether a change is breaking and whether it alters a resolved schema value;
read them rather than re-deriving the answer from the diff. Where a body says
nothing (the older history does), fall back to the diff of `provider.go`,
`data_source_*.go`, `internal/schemaengine/` and the convention YAML.

A failing `generate_test.go` is the loudest possible signal here: it holds the
predecessor modules' expected names, so a break there means migrating consumers
would get different names than the modules gave them — **major**, unless the
change is deliberate and documented as such.

### 3. Build, vet and test — mandatory gate

**These run before anything is tagged, and they must pass.** A published release
is immutable and consumers pin against it, so an untested release cannot be
taken back, only superseded.

```bash
gofmt -l .          # must print nothing
go build ./...
go vet ./...
go test ./...
```

- **Green** → continue.
- **Red** → stop and report what failed. Do not tag.
- **`go test` reports "no test files"** → the suite did not run because the repo
  has none yet. Say so explicitly, state that the release is therefore
  **unverified by automated tests**, and fall back to the manual
  `dev_overrides` check in step 3b. Never let an empty suite read as a pass.
- **`go`/`gofmt` not recognized** → the Go toolchain is missing or not on
  `PATH`, so none of these gates can run locally and step 3b cannot either.
  Invoke the [install-dependencies](../install-dependencies/SKILL.md) skill
  first. If it cannot be installed, say that the release is **unverified** and
  ask whether to proceed on CI alone — the release workflow installs its own Go
  via `actions/setup-go` and fails the build there if the code does not
  compile. Do not report a gate you could not run as passed.

See [go-tests](../go-tests/SKILL.md) for what a test here should cover.

### 3b. Verify against a real Terraform run — mandatory gate

Because the Go suite is thin, the release leans on an actual plan. GoReleaser
builds for every platform but runs no provider acceptance tests, so nothing else
in the pipeline proves the provider even starts.

Build the binary, point Terraform at it with `dev_overrides`, and run the
examples — see [go-tests](../go-tests/SKILL.md) for the full setup:

```bash
go build -o <gobin>/terraform-provider-naming .
cd examples/basic && terraform plan
```

Confirm the plan succeeds **and** that the computed attributes show real values
rather than `(known after apply)` — plan-time resolution is a documented
behaviour (`README.md`, `docs/index.md`) and a regression there is invisible to
`go build`. The example renders every cascade, so compare its output against the
names the docs show.

If this cannot be run, say so and state that the release is unverified against a
real Terraform run. Do not substitute `go build` for it.

### 3c. Remaining pre-release checks

All of these must also pass. Report failures and stop; do not tag a broken tree.

```bash
git status --porcelain              # must be empty — tag only committed state
git rev-parse --abbrev-ref HEAD     # must be master
git log origin/master..HEAD --oneline   # must be empty — push commits before tagging
```

Also confirm the release plumbing is intact, since a tag runs it unattended:

- `terraform-registry-manifest.json` exists and declares
  `"protocol_versions": ["6.0"]`.
- `.goreleaser.yml` still has the `signs` block and the `extra_files` entries
  for the manifest — the Registry rejects a release whose `SHA256SUMS` is
  unsigned or whose manifest is missing.
- `go.mod`'s `go` directive is what `actions/setup-go@v5` should use
  (`go-version-file: go.mod`).
- The `GPG_PRIVATE_KEY` and `PASSPHRASE` repository secrets are what the
  workflow signs with. They cannot be checked locally — if signing fails, that
  is where to look first.

### 4. Check docs and constraints

A release is also a documentation change. Before tagging:

- **`docs/` is what the Registry renders**, and it is generated-style content
  that is currently maintained by hand. Check `docs/data-sources/name.md` and
  `docs/data-sources/schema.md` against the actual attributes in
  `data_source_*.go`, and `docs/index.md` against the provider's own config in
  `provider.go`. A release is the point where docs drift becomes
  consumer-visible.
- Check `README.md`'s Inputs / Computed-outputs tables and its merge-semantics
  section against the code. These have drifted before — the README once
  described a deep merge the code did not perform.
- On a bump that crosses a major (including the first `v1.0.0`), update the
  `version = "~> X.0"` constraints in `README.md`, `docs/**` and
  `examples/**/*.tf`. A `~> 1.0` constraint never resolves a `2.x` release, so
  stale docs silently pin readers to the old major.
- The migration tables in `README.md` and `docs/data-sources/*.md` map the old
  module and resource surfaces onto the current one. A release that changes an
  attribute has to update them too, or they document a migration that no longer
  works.
- `examples/basic/naming.full.yaml` is the annotated reference copy of the
  bundled convention — if the release adds a key to it, mirror it there.

Commit any such changes before tagging, so the tag contains them — using
[git-commit-conventions](../git-commit-conventions/SKILL.md) for the message, and
splitting docs changes from code changes as that skill requires. Push the commit
before tagging; a tag on an unpushed commit cannot be built.

### 5. Tag and push

Use an **annotated** tag — `git describe` prefers them and the message serves as
the release note. GoReleaser is configured with `changelog: disable`, so it
generates none itself. Write the annotation the way
[git-commit-conventions](../git-commit-conventions/SKILL.md) describes a commit
message: English, imperative, no attribution trailers.

```bash
git tag -a v0.1.0 -m "v0.1.0

- <change one>
- <change two>"

git push origin v0.1.0
```

Push the tag by name, not with `--tags`, so a stray local tag never triggers a
release by accident.

### 6. Watch the workflow, then confirm

Unlike the module repos, **the release is not done when the tag is pushed** — it
is done when the workflow succeeds:

```bash
gh run list --workflow=release.yml --limit 3
gh run watch <run-id>
```

- **Workflow green** → the GitHub release exists with the zips, the signed
  `SHA256SUMS`, its `.sig`, and the manifest. Verify with
  `gh release view v0.1.0`.
- **Workflow red** → the tag is already published and immutable, so **do not
  re-tag**. Fix the cause, then cut the next patch version. The usual causes are
  a GPG secret problem (signing step), a build failure on one of the
  freebsd/windows/linux/darwin × amd64/386/arm/arm64 targets, or `go mod tidy`
  in the `before` hook dirtying the tree.
- If `gh` is not installed, say that the workflow result is unverified and point
  the user at the Actions tab — do not report the release as complete.

The Registry picks the version up from the GitHub release via its webhook,
usually within a few minutes. It appears at
`https://registry.terraform.io/providers/LemurDaniel/naming/<version>`.

Report the tagged version, the bump level with its reason, the workflow outcome,
and what still needs the user's attention.

## Related repositories

This provider supersedes two Terraform **modules** that are still published and
released by tag alone, with no build step:

- `terraform-random-naming` → `LemurDaniel/naming/random` (default branch `main`)
- `terraform-random-naming-schema` → `LemurDaniel/naming-schema/random` (default
  branch `master`)

Their convention YAML is a separate copy from this provider's embedded one. A
change meant for both is released independently in each repo — check the branch
and the tag style per repo instead of assuming.
