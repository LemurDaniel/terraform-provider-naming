package schemaengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These cover the merge contract the predecessor schema module
// (terraform-random-naming-schema) pinned down in tests/overlay.tftest.hcl:
// the caller's override wins at every level, and untouched siblings survive.
// Both directions were real bugs there - a shallow merge dropped siblings,
// and merging the other way round ignored the override entirely.

// writeOverride puts a YAML override in a temp dir and returns its path.
func writeOverride(t *testing.T, yaml string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "naming.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("failed to write the override file: %v", err)
	}

	return path
}

// resolve is a test helper resolving the bundled convention with an optional
// override and parameters.
func resolve(t *testing.T, namingFile string, parameters map[string]interface{}) *Schema {
	t.Helper()

	schema, err := Resolve(Config{
		Convention: "default",
		NamingFile: namingFile,
		Parameters: parameters,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	return schema
}

// azureAbbreviation looks up one entry of the merged Azure abbreviations.
func azureAbbreviation(t *testing.T, schema *Schema, key string) string {
	t.Helper()

	value, ok := GetKey(AsMap(GetKey(schema.Abbreviations, "Azure")), key).(string)
	if !ok {
		t.Fatalf("abbreviations.Azure[%q] is missing or not a string", key)
	}

	return value
}

func TestResolveBundledConvention(t *testing.T) {
	schema := resolve(t, "", nil)

	// These are the values consumers' names are built from, so the bundled
	// convention is worth guarding directly - nothing else checks the YAML.
	if schema.IndexModifier != 1 {
		t.Errorf("index_modifier = %d, want 1 (so indices start at 01)", schema.IndexModifier)
	}
	if lowerCase, ok := GetKey(schema.EnforceLowerCase, "default").(bool); !ok || !lowerCase {
		t.Errorf("enforce_lower_case.default = %v, want true", GetKey(schema.EnforceLowerCase, "default"))
	}
	if got := azureAbbreviation(t, schema, "Microsoft.Storage/storageAccounts::default"); got != "st" {
		t.Errorf("storage account abbreviation = %q, want \"st\"", got)
	}
	// The ::kind suffix is how one type carries several abbreviations.
	if got := azureAbbreviation(t, schema, "Microsoft.Compute/disks::os"); got != "osdisk" {
		t.Errorf("os disk abbreviation = %q, want \"osdisk\"", got)
	}
	if GetKey(schema.Patterns, "default") == nil {
		t.Error("patterns.default is missing")
	}
	if schema.UniqueID != "" {
		t.Errorf("unique id = %q, want empty without a random_seed", schema.UniqueID)
	}
}

func TestResolveOverrideWinsAndSiblingsSurvive(t *testing.T) {
	// A single abbreviation, deliberately one the convention already defines.
	override := writeOverride(t, `
abbreviations:
  Azure:
    Microsoft.Storage/storageAccounts::default: mystg
`)

	schema := resolve(t, override, nil)

	if got := azureAbbreviation(t, schema, "Microsoft.Storage/storageAccounts::default"); got != "mystg" {
		t.Errorf("override lost: got %q, want \"mystg\"", got)
	}
	// A shallow merge would have dropped the whole Azure table here.
	if got := azureAbbreviation(t, schema, "Microsoft.Network/virtualNetworks::default"); got != "vnet" {
		t.Errorf("sibling dropped: got %q, want \"vnet\"", got)
	}
	// So would dropping the untouched providers.
	if GetKey(schema.Abbreviations, "AzureAD") == nil {
		t.Error("the AzureAD provider was dropped by the merge")
	}
	// An override that mentions only abbreviations must still yield the
	// bundled patterns rather than nothing.
	if GetKey(schema.Patterns, "default") == nil {
		t.Error("patterns fell away although the override did not mention them")
	}
}

func TestResolvePatternsMergePerLevel(t *testing.T) {
	// One leaf at each of the three levels patterns is deep.
	override := writeOverride(t, `
patterns:
  default: "GLOBAL-<NAME>"
  Azure:
    Microsoft.Compute/disks:
      os: "<TYPE>-custom-<NAME>"
`)

	schema := resolve(t, override, nil)

	if got := GetKey(schema.Patterns, "default"); got != "GLOBAL-<NAME>" {
		t.Errorf("global pattern = %v, want the override", got)
	}

	azure := AsMap(GetKey(schema.Patterns, "Azure"))
	if GetKey(azure, "default") == nil {
		t.Error("the provider-level default was dropped by a type-level override")
	}

	disks := AsMap(GetKey(azure, "Microsoft.Compute/disks"))
	if got := GetKey(disks, "os"); got != "<TYPE>-custom-<NAME>" {
		t.Errorf("per-kind override lost: got %v", got)
	}
	// An untouched sibling type must survive a type-level override.
	if GetKey(azure, "Microsoft.Storage/storageAccounts") == nil {
		t.Error("a sibling resource type was dropped by the merge")
	}
}

func TestResolveEnforceLowerCaseMixesScalarsAndObjects(t *testing.T) {
	// enforce_lower_case holds a bool and a nested object at the same level,
	// which is what makes it awkward to merge.
	override := writeOverride(t, `
enforce_lower_case:
  Azure:
    "Microsoft.Compute/virtualMachines*": false
`)

	schema := resolve(t, override, nil)

	if lowerCase, ok := GetKey(schema.EnforceLowerCase, "default").(bool); !ok || !lowerCase {
		t.Error("the scalar default was lost when a nested provider block was added")
	}

	azure := AsMap(GetKey(schema.EnforceLowerCase, "Azure"))
	if lowerCase, ok := GetKey(azure, "Microsoft.Compute/virtualMachines*").(bool); !ok || lowerCase {
		t.Error("the nested override was lost")
	}
}

func TestResolveDefaultParameterPrecedence(t *testing.T) {
	// Three layers, each winning over the one before it.
	override := writeOverride(t, `
default_parameters:
  location: northeurope
  environment: staging
`)

	schema := resolve(t, override, map[string]interface{}{
		"location": "westeurope",
		"name":     "myapp",
	})

	if got := schema.DefaultParameters["location"]; got != "westeurope" {
		t.Errorf("location = %v, want the provider's \"westeurope\"", got)
	}
	if got := schema.DefaultParameters["environment"]; got != "staging" {
		t.Errorf("environment = %v, want the override file's \"staging\"", got)
	}
	if got := schema.DefaultParameters["name"]; got != "myapp" {
		t.Errorf("name = %v, want the provider's \"myapp\"", got)
	}
}

func TestResolveIndexModifier(t *testing.T) {
	// A custom 0 is a legal override, so this cannot be a truthiness check.
	zero := writeOverride(t, "index_modifier: 0\n")
	if schema := resolve(t, zero, nil); schema.IndexModifier != 0 {
		t.Errorf("index_modifier = %d, want the override's 0", schema.IndexModifier)
	}

	// An override that does not mention it falls back to the convention.
	other := writeOverride(t, "abbreviations:\n  Azure:\n    Custom/type::default: cst\n")
	if schema := resolve(t, other, nil); schema.IndexModifier != 1 {
		t.Errorf("index_modifier = %d, want the bundled 1", schema.IndexModifier)
	}
}

func TestResolveRandomSeedIsDeterministic(t *testing.T) {
	first, err := Resolve(Config{Convention: "default", RandomSeed: "myapp-prod"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	second, err := Resolve(Config{Convention: "default", RandomSeed: "myapp-prod"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	other, err := Resolve(Config{Convention: "default", RandomSeed: "myapp-dev"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if first.UniqueID != second.UniqueID {
		t.Errorf("the same seed produced %q and %q", first.UniqueID, second.UniqueID)
	}
	if first.UniqueID == other.UniqueID {
		t.Errorf("two different seeds produced the same id %q", first.UniqueID)
	}
	if len(first.UniqueID) != 36 {
		t.Errorf("id = %q, want a 36-character uuid", first.UniqueID)
	}
}

func TestResolveRejectsUnknownConvention(t *testing.T) {
	if _, err := Resolve(Config{Convention: "nonexistent"}); err == nil {
		t.Error("expected an error for a convention with no bundled file")
	}

	// An empty convention name is the documented default rather than an
	// error, since the provider's `convention` attribute is optional.
	if _, err := Resolve(Config{}); err != nil {
		t.Errorf("an empty convention should default to \"default\", got %v", err)
	}
}

func TestResolveRejectsBrokenOverrideFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.yaml")
	if _, err := Resolve(Config{Convention: "default", NamingFile: missing}); err == nil {
		t.Error("expected an error for a missing override file")
	}

	malformed := writeOverride(t, "abbreviations: [this is not a map\n")
	_, err := Resolve(Config{Convention: "default", NamingFile: malformed})
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected a parse error, got %v", err)
	}
}

func TestDeepMergeUserWins(t *testing.T) {
	t.Run("nil_override_is_a_no_op", func(t *testing.T) {
		base := map[string]interface{}{"a": "1"}
		if got := DeepMergeUserWins(base, nil); got["a"] != "1" {
			t.Errorf("a nil override wiped the base: %#v", got)
		}
	})

	t.Run("nil_base_yields_the_override", func(t *testing.T) {
		if got := DeepMergeUserWins(nil, map[string]interface{}{"a": "1"}); got["a"] != "1" {
			t.Errorf("got %#v", got)
		}
	})

	t.Run("both_nil_yields_an_empty_map", func(t *testing.T) {
		if got := DeepMergeUserWins(nil, nil); got == nil || len(got) != 0 {
			t.Errorf("got %#v, want an empty map", got)
		}
	})

	t.Run("a_scalar_override_replaces_a_map", func(t *testing.T) {
		base := map[string]interface{}{"a": map[string]interface{}{"b": "1"}}
		override := map[string]interface{}{"a": "flat"}

		if got := DeepMergeUserWins(base, override); got["a"] != "flat" {
			t.Errorf("got %#v", got)
		}
	})
}

func TestGetKeyAndCoalesce(t *testing.T) {
	// A nil map is the normal "no override file" case, not an edge case.
	if got := GetKey(nil, "anything"); got != nil {
		t.Errorf("GetKey(nil, ...) = %v, want nil", got)
	}

	// Coalesce has to pick a present-but-falsy value over a later fallback,
	// which is what makes index_modifier: 0 a legal override.
	if got := Coalesce(0, 1); got != 0 {
		t.Errorf("Coalesce(0, 1) = %v, want 0", got)
	}
	if got := Coalesce(nil, nil, "third"); got != "third" {
		t.Errorf("Coalesce(nil, nil, \"third\") = %v", got)
	}
	if got := Coalesce(nil, nil); got != nil {
		t.Errorf("Coalesce(nil, nil) = %v, want nil", got)
	}
}
