package schemaengine

import (
	"strings"
	"testing"
)

// The fixtures below are ports of the predecessor modules' test suites
// (terraform-random-naming, tests/fixture.tftest.hcl and
// tests/index.tftest.hcl), with the same expected names. They exist to prove
// the Go engine renders exactly what the released HCL modules rendered, so a
// consumer migrating to the provider does not silently get different names -
// which would force resource replacement.

// fixtureSchema mirrors the `schema` variable of tests/fixture.tftest.hcl.
func fixtureSchema() *Schema {
	return &Schema{
		IndexModifier: 1,
		// The uuid is fixed, so <UNIQUE_ID_n> is deterministic.
		UniqueID: "abcdef12-3456-7890-abcd-ef1234567890",

		EnforceLowerCase: map[string]interface{}{
			"default": true,
			"Azure": map[string]interface{}{
				"default":                            true,
				"Microsoft.Compute/virtualMachines*": false,
			},
		},

		Abbreviations: map[string]interface{}{
			"Azure": map[string]interface{}{
				"Microsoft.Network/virtualNetworks::default": "vnet",
				"Microsoft.Storage/storageAccounts::default": "st",
				"Microsoft.Storage/storageAccounts::vm":      "stvm",
				"Microsoft.Compute/disks::os":                "osdisk",
				"Microsoft.Compute/disks::data":              "disk",
				"Microsoft.Compute/virtualMachines::default": "vm",
				"Microsoft.KeyVault/vaults::default":         "kv",
				"Microsoft.Web/sites::default":               "app",
			},
			"AzureAD": map[string]interface{}{
				"Groups::security": "grp",
			},
		},

		Mappings: map[string]interface{}{
			"location":    map[string]interface{}{"westeurope": "we"},
			"environment": map[string]interface{}{"test": "tst"},
		},

		Patterns: map[string]interface{}{
			// Only reached when the provider has no default of its own.
			"default": "GLOBAL-<NAME>",

			"AzureAD": map[string]interface{}{
				"default": "<TYPE>-<ENVIRONMENT>-<NAME>-<INDEX;%02s>",
			},

			"Azure": map[string]interface{}{
				"default": "<TYPE>-<LOCATION>-<ENVIRONMENT>-<NAME>-<INDEX;%02s>",

				"Microsoft.Storage/storageAccounts": map[string]interface{}{
					"default":    "<TYPE><LOCATION><ENVIRONMENT><NAME><INDEX;%02s>",
					"vm_pattern": "<TYPE><LOCATION><NAME><INDEX;%02s>",
				},

				"Microsoft.Compute/disks": map[string]interface{}{
					"os":   "<TYPE>-<NAME>-<ENVIRONMENT>",
					"data": "<TYPE><INDEX;%02s>-<NAME>-<ENVIRONMENT>",
				},

				"Microsoft.Web/sites": map[string]interface{}{
					"default": "<TYPE>-<NAME><?SUBNAME;-%s>-<INDEX;%02s>",
				},

				"Microsoft.KeyVault/vaults": map[string]interface{}{
					"default": "<TYPE>-<NAME>-<UNIQUE_ID_4>",
				},
			},
		},

		DefaultParameters: map[string]interface{}{
			"location":    "westeurope",
			"environment": "test",
			"name":        "tftest",
		},
	}
}

// generate is a test helper rendering one request against a schema.
func generate(t *testing.T, schema *Schema, resource string, mutate func(*Request)) []string {
	t.Helper()

	id, err := ParseResourceID(resource)
	if err != nil {
		t.Fatalf("ParseResourceID(%q): %v", resource, err)
	}

	req := Request{Resource: id, IndexStart: 0, IndexCount: 1}
	if mutate != nil {
		mutate(&req)
	}

	names, err := schema.Generate(req)
	if err != nil {
		t.Fatalf("Generate(%q): %v", resource, err)
	}

	return names
}

func TestGenerateMatchesModuleFixtures(t *testing.T) {
	schema := fixtureSchema()

	cases := []struct {
		name     string
		resource string
		mutate   func(*Request)
		want     string
	}{
		{
			name:     "provider_default_pattern",
			resource: "Azure::Microsoft.Network/virtualNetworks",
			want:     "vnet-we-tst-tftest-01",
		},
		{
			name:     "type_pattern_without_separators",
			resource: "Azure::Microsoft.Storage/storageAccounts",
			want:     "stwetsttftest01",
		},
		{
			name:     "kind_selects_abbreviation_and_pattern",
			resource: "Azure::Microsoft.Compute/disks::os",
			want:     "osdisk-tftest-tst",
		},
		{
			name:     "other_kind_same_type",
			resource: "Azure::Microsoft.Compute/disks::data",
			want:     "disk01-tftest-tst",
		},
		{
			// The abbreviation comes from ::vm (no ::vm_pattern entry
			// exists), the pattern from the vm_pattern key.
			name:     "naming_id_wins_over_kind",
			resource: "Azure::Microsoft.Storage/storageAccounts::vm",
			mutate:   func(r *Request) { r.NamingID = "vm_pattern" },
			want:     "stvmwetftest01",
		},
		{
			name:     "optional_token_present",
			resource: "Azure::Microsoft.Web/sites",
			mutate: func(r *Request) {
				r.Parameters = map[string]interface{}{"subname": "api"}
			},
			want: "app-tftest-api-01",
		},
		{
			name:     "optional_token_absent_leaves_no_stray_separator",
			resource: "Azure::Microsoft.Web/sites",
			want:     "app-tftest-01",
		},
		{
			name:     "unique_id_is_deterministic",
			resource: "Azure::Microsoft.KeyVault/vaults",
			want:     "kv-tftest-abcd",
		},
		{
			name:     "other_provider_has_own_default",
			resource: "AzureAD::Groups::security",
			want:     "grp-tst-tftest-01",
		},
		{
			name:     "unmapped_parameter_passes_through",
			resource: "Azure::Microsoft.Network/virtualNetworks",
			mutate: func(r *Request) {
				r.Parameters = map[string]interface{}{"environment": "sandbox"}
			},
			want: "vnet-we-sandbox-tftest-01",
		},
		{
			// The mapping key is "westeurope"; the lookup lowercases both
			// sides, so "West Europe" does NOT match and passes through.
			name:     "mapping_is_case_insensitive_but_not_whitespace_insensitive",
			resource: "Azure::Microsoft.Network/virtualNetworks",
			mutate: func(r *Request) {
				r.Parameters = map[string]interface{}{"location": "West Europe"}
			},
			want: "vnet-west europe-tst-tftest-01",
		},
		{
			name:     "lower_case_enforced_by_default",
			resource: "Azure::Microsoft.Network/virtualNetworks",
			mutate: func(r *Request) {
				r.Parameters = map[string]interface{}{"name": "MixedCase"}
			},
			want: "vnet-we-tst-mixedcase-01",
		},
		{
			name:     "lower_case_disabled_by_wildcard_match",
			resource: "Azure::Microsoft.Compute/virtualMachines",
			mutate: func(r *Request) {
				r.Parameters = map[string]interface{}{"name": "MixedCase"}
			},
			want: "vm-we-tst-MixedCase-01",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := generate(t, schema, tc.resource, tc.mutate)
			if got[0] != tc.want {
				t.Errorf("got %q, want %q", got[0], tc.want)
			}
		})
	}
}

// indexSchema mirrors the `schema` variable of tests/index.tftest.hcl.
func indexSchema() *Schema {
	return &Schema{
		IndexModifier:    1,
		EnforceLowerCase: map[string]interface{}{"default": true},
		Abbreviations: map[string]interface{}{
			"Azure": map[string]interface{}{
				"Microsoft.Compute/virtualMachines::default": "vm",
			},
		},
		Mappings: map[string]interface{}{
			"environment": map[string]interface{}{"test": "tst"},
		},
		Patterns: map[string]interface{}{
			"default": "<TYPE>-<NAME>-<INDEX;%02s>",
			"Azure": map[string]interface{}{
				"default": "<TYPE>-<NAME>-<INDEX;%02s>",
			},
		},
		DefaultParameters: map[string]interface{}{
			"environment": "test",
			"name":        "tftest",
		},
	}
}

func TestGenerateIndexRange(t *testing.T) {
	schema := indexSchema()
	const vm = "Azure::Microsoft.Compute/virtualMachines"

	cases := []struct {
		name   string
		mutate func(*Request)
		want   []string
	}{
		{
			name: "default_is_a_single_name",
			want: []string{"vm-tftest-01"},
		},
		{
			name:   "count_is_the_number_of_names",
			mutate: func(r *Request) { r.IndexStart, r.IndexCount = 0, 5 },
			want: []string{
				"vm-tftest-01", "vm-tftest-02", "vm-tftest-03",
				"vm-tftest-04", "vm-tftest-05",
			},
		},
		{
			// start=2 plus index_modifier=1 => first index 03, still 5 names.
			name:   "start_offsets_without_being_counted_twice",
			mutate: func(r *Request) { r.IndexStart, r.IndexCount = 2, 5 },
			want: []string{
				"vm-tftest-03", "vm-tftest-04", "vm-tftest-05",
				"vm-tftest-06", "vm-tftest-07",
			},
		},
		{
			name:   "first_name_of_a_shifted_range",
			mutate: func(r *Request) { r.IndexStart, r.IndexCount = 3, 2 },
			want:   []string{"vm-tftest-04", "vm-tftest-05"},
		},
		{
			name: "index_can_be_overridden_via_parameters",
			mutate: func(r *Request) {
				r.Parameters = map[string]interface{}{"index": 7}
			},
			want: []string{"vm-tftest-08"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := generate(t, schema, vm, tc.mutate)

			if len(got) != len(tc.want) {
				t.Fatalf("expected %d names, got %d: %v", len(tc.want), len(got), got)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseResourceID(t *testing.T) {
	cases := []struct {
		input    string
		provider string
		typeName string
		kind     string
		wantErr  bool
	}{
		{
			input:    "Azure::Microsoft.Storage/storageAccounts",
			provider: "Azure",
			typeName: "Microsoft.Storage/storageAccounts",
			kind:     "default",
		},
		{
			input:    "Azure::Microsoft.Compute/disks::os",
			provider: "Azure",
			typeName: "Microsoft.Compute/disks",
			kind:     "os",
		},
		{
			// An empty third segment falls back to the default kind rather
			// than producing an abbreviation lookup for "type::".
			input:    "AzureAD::Groups::",
			provider: "AzureAD",
			typeName: "Groups",
			kind:     "default",
		},
		{input: "Azure", wantErr: true},
		{input: "", wantErr: true},
		{input: "::Microsoft.Storage/storageAccounts", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseResourceID(tc.input)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Provider != tc.provider || got.Type != tc.typeName || got.Kind != tc.kind {
				t.Errorf("got provider=%q type=%q kind=%q, want %q/%q/%q",
					got.Provider, got.Type, got.Kind, tc.provider, tc.typeName, tc.kind)
			}
		})
	}
}

func TestLowerCaseForWildcardExpansion(t *testing.T) {
	id, err := ParseResourceID("Azure::Microsoft.Network/virtualNetworks")
	if err != nil {
		t.Fatalf("ParseResourceID: %v", err)
	}

	// A bare "*" is only a catch-all once it is expanded to ".*" - as a raw
	// regular expression it is invalid ("missing argument to repetition
	// operator"), so without the expansion it matches nothing and the
	// provider default wins instead.
	//
	// Most wildcard keys ("Microsoft.Compute/virtualMachines*") happen to
	// behave the same expanded or not, because matching is unanchored and
	// "s*" already means "zero or more s". This case is what actually pins
	// the expansion down.
	schema := &Schema{
		EnforceLowerCase: map[string]interface{}{
			"default": true,
			"Azure": map[string]interface{}{
				"default": true,
				"*":       false,
			},
		},
	}

	if schema.LowerCaseFor(id) {
		t.Error("the \"*\" catch-all did not match, so lower-casing was not disabled")
	}
}

func TestLowerCaseForResolutionOrder(t *testing.T) {
	id, err := ParseResourceID("Azure::Microsoft.Network/virtualNetworks")
	if err != nil {
		t.Fatalf("ParseResourceID: %v", err)
	}

	cases := []struct {
		name   string
		schema *Schema
		want   bool
	}{
		{
			name: "type_pattern_beats_provider_default",
			schema: &Schema{EnforceLowerCase: map[string]interface{}{
				"default": true,
				"Azure": map[string]interface{}{
					"default":             true,
					"Microsoft.Network/*": false,
				},
			}},
			want: false,
		},
		{
			name: "provider_default_beats_global_default",
			schema: &Schema{EnforceLowerCase: map[string]interface{}{
				"default": true,
				"Azure":   map[string]interface{}{"default": false},
			}},
			want: false,
		},
		{
			name: "global_default_is_the_last_resort",
			schema: &Schema{EnforceLowerCase: map[string]interface{}{
				"default": true,
			}},
			want: true,
		},
		{
			name:   "nothing_configured_means_no_lower_casing",
			schema: &Schema{EnforceLowerCase: map[string]interface{}{}},
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.schema.LowerCaseFor(id); got != tc.want {
				t.Errorf("LowerCaseFor = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGenerateErrors(t *testing.T) {
	schema := fixtureSchema()

	t.Run("missing_abbreviation_fails", func(t *testing.T) {
		id, _ := ParseResourceID("Azure::Microsoft.Nonexistent/things")
		if _, err := schema.Generate(Request{Resource: id, IndexCount: 1}); err == nil {
			t.Error("expected an error for a resource with no abbreviation")
		}
	})

	t.Run("missing_required_parameter_fails", func(t *testing.T) {
		bare := fixtureSchema()
		bare.DefaultParameters = map[string]interface{}{"name": "tftest"}

		id, _ := ParseResourceID("Azure::Microsoft.Network/virtualNetworks")
		if _, err := bare.Generate(Request{Resource: id, IndexCount: 1}); err == nil {
			t.Error("expected an error for an unset required parameter")
		}
	})

	t.Run("unique_id_without_seed_fails", func(t *testing.T) {
		unseeded := fixtureSchema()
		unseeded.UniqueID = ""

		id, _ := ParseResourceID("Azure::Microsoft.KeyVault/vaults")
		_, err := unseeded.Generate(Request{Resource: id, IndexCount: 1})
		if err == nil || !strings.Contains(err.Error(), "random_seed") {
			t.Errorf("expected a random_seed error, got %v", err)
		}
	})

	t.Run("count_below_one_fails", func(t *testing.T) {
		id, _ := ParseResourceID("Azure::Microsoft.Network/virtualNetworks")
		if _, err := schema.Generate(Request{Resource: id, IndexCount: 0}); err == nil {
			t.Error("expected an error for index.count < 1")
		}
	})
}

func TestApplyFormat(t *testing.T) {
	cases := []struct {
		format string
		value  string
		want   string
	}{
		// Terraform's format() zero-pads a string conversion, so an index
		// renders as "01" rather than " 1".
		{format: "%02s", value: "1", want: "01"},
		{format: "%02s", value: "11", want: "11"},
		{format: "%02s", value: "111", want: "111"},
		{format: "%s", value: "api", want: "api"},
		// An optional token carries its separator inside the format.
		{format: "-%s", value: "api", want: "-api"},
		{format: "%3s", value: "1", want: "  1"},
		{format: "%-3s", value: "1", want: "1  "},
		{format: "%03d", value: "7", want: "007"},
	}

	for _, tc := range cases {
		t.Run(tc.format+"/"+tc.value, func(t *testing.T) {
			if got := applyFormat(tc.format, tc.value); got != tc.want {
				t.Errorf("applyFormat(%q, %q) = %q, want %q", tc.format, tc.value, got, tc.want)
			}
		})
	}
}
