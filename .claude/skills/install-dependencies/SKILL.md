---
name: install-dependencies
description: Install and verify the toolchain this provider needs — Go, Terraform, and the optional gh/goreleaser/gpg tools. Use when a build, vet, test, fmt or plan command fails because a tool is missing ("go is not recognized", "command not found", "go: Der Begriff ... wird nicht als Name eines Cmdlets erkannt"), when setting up a fresh machine or a new clone, or when the user asks to install dependencies / set up the dev environment ("install go", "abhängigkeiten installieren", "dev setup", "was brauche ich hier").
---

# Installing the toolchain

Everything in this repo is built with Go and verified with Terraform. Nothing
is vendored and no container is used, so the tools have to exist on the machine.

**Check before installing. Install only what is missing. Never install
unprompted** — an installation changes the machine outside the repository, so it
needs the user's go-ahead unless they already asked for it.

## 1. Find out what is missing

```powershell
foreach ($t in "go","gofmt","terraform","git","gh","goreleaser","gpg") {
  $c = Get-Command $t -ErrorAction SilentlyContinue
  "{0,-12} {1}" -f $t, ($(if ($c) { $c.Source } else { "MISSING" }))
}
```

A tool can also be installed but not on `PATH` — check the usual Go locations
before concluding it is absent:

```powershell
"C:\Program Files\Go\bin\go.exe","C:\Go\bin\go.exe","$env:LOCALAPPDATA\Programs\Go\bin\go.exe" |
  ForEach-Object { "$_ -> $(Test-Path $_)" }
```

If a binary is there but not resolvable, the fix is the `PATH`, not another
install — see section 4.

## 2. What each tool is for

| Tool | Needed for | Required? |
|---|---|---|
| `go` (ships `gofmt`) | `go build`, `go vet`, `go test`, building the provider binary for `dev_overrides` | **yes** — nothing in [go-tests](../go-tests/SKILL.md) works without it |
| `terraform` | running `examples/basic/` against a local build | **yes** for any real verification |
| `git` | the repo itself | **yes** |
| `gh` | watching the release workflow, `gh release view` | optional — [release](../release/SKILL.md) degrades to "unverified" without it |
| `goreleaser` | `goreleaser check` on `.goreleaser.yml` | optional — CI runs the real thing |
| `gpg` | only CI signs release artifacts (via repo secrets) | not needed locally |

Note that the **release workflow installs its own Go** through
`actions/setup-go` with `go-version-file: go.mod`, so CI never depends on the
local install. A missing local Go blocks verification, not the release itself.

## 3. Install

### Windows (winget — preferred)

`winget` ships with Windows 11 and needs no setup:

```powershell
winget install --id GoLang.Go --source winget
winget install --id Hashicorp.Terraform --source winget
winget install --id GitHub.cli --source winget          # optional
winget install --id GoReleaser.GoReleaser --source winget  # optional
```

Add `--accept-source-agreements --accept-package-agreements` when running
non-interactively, or winget blocks on a prompt. If an exact id fails, find the
real one with `winget search golang` rather than guessing.

Prefer `winget` over downloading an MSI by hand: it registers the package, so
upgrades later are `winget upgrade --id GoLang.Go`.

If `winget` itself is unavailable, the fallbacks in order are `scoop install go`,
`choco install golang` (needs an elevated shell), or the official MSI from
<https://go.dev/dl/>.

Terraform in particular is often managed by hand rather than by a package
manager — a versioned layout such as `~/bin/terraform/<version>` with an
`active` symlink is common. Always resolve `(Get-Command terraform).Source`
first (section 1) and leave such an install alone; a second copy from `winget`
would shadow the user's own and switch versions silently.

### Linux / macOS

```bash
# macOS
brew install go terraform gh

# Debian/Ubuntu — the distro's golang package is usually too old for go.mod's
# directive; prefer the tarball from https://go.dev/dl/
sudo apt-get install -y git
```

Check `go.mod`'s `go` directive and install at least that minor version. A
toolchain older than the directive fails the build with a version error rather
than a missing-tool error.

## 4. Make it visible to the current session

**A fresh install does not appear in an already-running shell.** The installer
updates the machine `PATH`, but this session's environment was captured when it
started, so `go` keeps reporting "not recognized" until the `PATH` is reloaded:

```powershell
$env:PATH = [System.Environment]::GetEnvironmentVariable("PATH","Machine") + ";" +
            [System.Environment]::GetEnvironmentVariable("PATH","User")
```

This repo's shell state does **not** persist between tool calls, so that line
has to be part of the *same* command as whatever uses `go`:

```powershell
$env:PATH = [System.Environment]::GetEnvironmentVariable("PATH","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("PATH","User"); go version
```

If that still fails, the install needs a new terminal — say so and stop rather
than retrying in a loop. Do not edit the user's persistent `PATH` to work
around it.

## 5. Verify, then report

```powershell
go version          # must satisfy go.mod's `go` directive
go env GOPATH       # where `go build -o` puts the provider binary
terraform version
```

Then confirm the toolchain actually works on this repo:

```powershell
go build ./...
go vet ./...
```

Report what was installed, at which version, and whether the current session can
see it. **If a tool is still missing, say the gate "could not run" — never as a
pass, and never quietly skip it.** That rule is what the other skills rely on.

## 6. What this does not install

- **The GPG signing key** for releases. Artifacts are signed in CI from the
  `GPG_PRIVATE_KEY` / `PASSPHRASE` repository secrets, not locally. A local
  `gpg` is irrelevant to that.
- **The provider itself into Terraform.** Local use goes through
  `dev_overrides` in the Terraform CLI config, which is a per-user config change
  rather than a dependency — the steps are in
  [go-tests](../go-tests/SKILL.md).
- **`terraform-plugin-testing`**, needed for acceptance tests. That is a Go
  module dependency (`go get`), so it belongs in a commit, not in machine setup.

## Related

- [go-tests](../go-tests/SKILL.md) — what to run once the toolchain is there.
- [release](../release/SKILL.md) — the gates that need it, and how to report a
  release that could not be verified locally.
