# NOTE:
#   Requires: PowerShell 7 and network access to raw.githubusercontent.com.
#
#   ./scripts/Update-MicrosoftNamingRules.ps1 -Tag 0.4.4 -WhatIf
#   ./scripts/Update-MicrosoftNamingRules.ps1 -Tag 0.4.4
#
#   Replaces the vendored resourceDefinition.json (and its LICENSE) with the one of an
#   Azure/terraform-azurerm-naming release, and reports what changed: entries added, removed or
#   with other limits, and every `microsoft:` reference in azure.rules.yaml that no longer
#   exists. Run `go test ./...` afterwards - TestBundledConventionPassesItsRules is the check
#   that a new limit does not reject a name the bundled convention renders.

#
#   Chapters:
#     LOAD BOTH DEFINITIONS   Validations, the vendored file and the release's file.
#     COMPARE DEFINITIONS     Added, removed and changed entries, broken references.
#     WRITE DEFINITIONS       Writes the release's files and the tag into the README.
#

[CmdletBinding(SupportsShouldProcess = $true)]
param (
    <#
    [Required]
    The release tag of Azure/terraform-azurerm-naming, e.g. '0.4.4' (no 'v' prefix).
    #>
    [Parameter(
        Mandatory = $true
    )]
    [ValidatePattern('^\d+\.\d+\.\d+$')]
    [System.String]
    $Tag
)

Set-StrictMode -Version 1.0
$ErrorActionPreference = 'Stop'

$SOURCE_REPOSITORY = 'https://raw.githubusercontent.com/Azure/terraform-azurerm-naming'
$RULES_FOLDER      = Join-Path -Path $PSScriptRoot -ChildPath '../internal/schemaengine/rules'
$VENDOR_FOLDER     = Join-Path -Path $RULES_FOLDER -ChildPath 'microsoft'
$DEFINITIONS_FILE  = Join-Path -Path $VENDOR_FOLDER -ChildPath 'resourceDefinition.json'
$LICENSE_FILE      = Join-Path -Path $VENDOR_FOLDER -ChildPath 'LICENSE'
$README_FILE       = Join-Path -Path $VENDOR_FOLDER -ChildPath 'README.md'
$RULES_FILE        = Join-Path -Path $RULES_FOLDER -ChildPath 'azure.rules.yaml'

# ===========================================================================
# =========================   LOAD BOTH DEFINITIONS   =======================
# ===========================================================================
#
#   1. Validations   Fails early when the vendored files are not where expected.
#   2. Download      Loads the release's definitions and license.
#

# ---------------------------------------------------------------------------
# Validations
#   Fails early when the vendored files are not where expected.
# ---------------------------------------------------------------------------

foreach ($path in @($DEFINITIONS_FILE, $RULES_FILE, $README_FILE)) {
    if (-NOT (Test-Path -Path $path)) {
        throw [System.IO.FileNotFoundException]::new("'$path' must exist - run the script from a checkout of terraform-provider-naming.")
    }
}

# ---------------------------------------------------------------------------
# Download
#   Loads the release's definitions and license.
# ---------------------------------------------------------------------------

Write-Verbose "[Update-MicrosoftNamingRules] Downloading resourceDefinition.json of $Tag."
$ReleaseJson    = (Invoke-WebRequest -Uri "$SOURCE_REPOSITORY/$Tag/resourceDefinition.json").Content
$ReleaseLicense = (Invoke-WebRequest -Uri "$SOURCE_REPOSITORY/$Tag/LICENSE").Content

# Keyed by name, so both sides compare entry by entry.
$Vendored = @{}
foreach ($entry in (Get-Content -Path $DEFINITIONS_FILE -Raw | ConvertFrom-Json)) {
    $Vendored[$entry.name] = $entry
}
$Release = @{}
foreach ($entry in ($ReleaseJson | ConvertFrom-Json)) {
    $Release[$entry.name] = $entry
}

# ===========================================================================
# ==========================   COMPARE DEFINITIONS   ========================
# ===========================================================================
#
#   1. Entries      Added, removed and changed entries between both files.
#   2. References   Every microsoft: reference the release no longer has.
#

# ---------------------------------------------------------------------------
# Entries
#   Added, removed and changed entries between both files.
# ---------------------------------------------------------------------------

$Changes = [System.Collections.Generic.List[PSCustomObject]]::new()

foreach ($name in ($Release.Keys | Sort-Object)) {
    if (-NOT $Vendored.ContainsKey($name)) {
        $Changes.Add([PSCustomObject]@{ Entry = $name; Change = 'added'; Detail = '' })
        continue
    }

    # Only what a rule reads: length, regex and scope.
    $before = $Vendored[$name]
    $after  = $Release[$name]
    $fields = @(
        foreach ($field in @('min', 'max')) {
            if ($before.length.$field -NE $after.length.$field) {
                "$field $($before.length.$field) -> $($after.length.$field)"
            }
        }
        foreach ($field in @('regex', 'scope')) {
            if ($before.$field -CNE $after.$field) {
                "$field '$($before.$field)' -> '$($after.$field)'"
            }
        }
    )
    if ($fields.Count -GT 0) {
        $Changes.Add([PSCustomObject]@{ Entry = $name; Change = 'changed'; Detail = $fields -join '; ' })
    }
}

foreach ($name in ($Vendored.Keys | Sort-Object)) {
    if (-NOT $Release.ContainsKey($name)) {
        $Changes.Add([PSCustomObject]@{ Entry = $name; Change = 'removed'; Detail = '' })
    }
}

# ---------------------------------------------------------------------------
# References
#   Every microsoft: reference the release no longer has.
# ---------------------------------------------------------------------------

# '    microsoft: storage_account' => storage_account
$References = Select-String -Path $RULES_FILE -Pattern '^\s+microsoft:\s*(\S+)\s*$'
| ForEach-Object -Process { $_.Matches[0].Groups[1].Value }
| Sort-Object -Unique

$BrokenReferences = @($References | Where-Object -FilterScript { -NOT $Release.ContainsKey($_) })

# ===========================================================================
# ===========================   WRITE DEFINITIONS   =========================
# ===========================================================================
#
#   1. Files    Writes the release's definitions, license and tag.
#   2. Report   Returns the changes and the broken references.
#

# ---------------------------------------------------------------------------
# Files
#   Writes the release's definitions, license and tag.
# ---------------------------------------------------------------------------

if ($PSCmdlet.ShouldProcess($VENDOR_FOLDER, "Replace the vendored definitions with $Tag")) {
    Set-Content -Path $DEFINITIONS_FILE -Value $ReleaseJson -NoNewline
    Set-Content -Path $LICENSE_FILE -Value $ReleaseLicense -NoNewline

    # The README names the vendored tag on one line: 'Release: `0.4.4`'.
    $readme = (Get-Content -Path $README_FILE -Raw) -replace 'Release: `[^`]+`', "Release: ``$Tag``"
    Set-Content -Path $README_FILE -Value $readme -NoNewline
}

# ---------------------------------------------------------------------------
# Report
#   Returns the changes and the broken references.
# ---------------------------------------------------------------------------

if ($BrokenReferences.Count -GT 0) {
    Write-Warning "[Update-MicrosoftNamingRules] azure.rules.yaml refers to entries $Tag does not have: $($BrokenReferences -join ', ')."
}

return [PSCustomObject]@{
    Tag              = $Tag
    Changes          = $Changes
    BrokenReferences = $BrokenReferences
}
