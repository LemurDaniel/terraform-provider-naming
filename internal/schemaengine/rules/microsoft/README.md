# Microsoft naming definitions

`resourceDefinition.json` is copied unchanged from
[Azure/terraform-azurerm-naming](https://github.com/Azure/terraform-azurerm-naming), the
`Azure/naming/azurerm` module, under its MIT license (`LICENSE` beside it).

Release: `0.4.4`

| File | Content |
| --- | --- |
| `resourceDefinition.json` | Length, regex, scope and slug per resource, keyed by azurerm name |
| `LICENSE` | Microsoft's MIT license for it |

The regexes are written as HCL string literals (`\\(` for `\(`); the provider unescapes them
once before compiling. Entries that are wrong are corrected in `../azure.rules.yaml`, never
here, so a refresh with `scripts/Update-MicrosoftNamingRules.ps1` keeps the corrections.
