package schemaengine

import (
	"sort"
	"strings"
	"testing"
)

// These cover the naming rules: that the bundled rules file and Microsoft's
// vendored definitions load and compile, that a rule resolves along its
// cascade with explicit values over the Microsoft entry, and - the guard that
// matters most - that every name the bundled convention renders passes its
// own rule. A rule that rejects the convention's own names would fail every
// consumer's plan after an upgrade.

// sampleParameters are the parameters the bundled convention is rendered
// with: the README's example stage.
var sampleParameters = map[string]interface{}{
	"location":    "westeurope",
	"environment": "production",
	"name":        "payments",
}

// ruleSchema is a minimal schema with one rule per cascade level.
func ruleSchema() *Schema {
	return &Schema{
		Validation: ValidationError,
		Rules: map[string]interface{}{
			"Azure": map[string]interface{}{
				"Microsoft.Storage/storageAccounts": map[string]interface{}{
					"microsoft": "storage_account",
				},
				"Microsoft.Compute/disks": map[string]interface{}{
					"max_length": 10,
					"regex":      "^[a-z-]+$",
				},
				"Microsoft.Compute/disks::os": map[string]interface{}{
					"max_length": 6,
				},
				"Microsoft.KeyVault/vaults": map[string]interface{}{
					"microsoft":  "key_vault",
					"max_length": 12,
				},
			},
		},
	}
}

func request(t *testing.T, resource, namingID string) Request {
	t.Helper()

	id, err := ParseResourceID(resource)
	if err != nil {
		t.Fatalf("ParseResourceID(%q): %v", resource, err)
	}

	return Request{Resource: id, NamingID: namingID, IndexCount: 1}
}

func TestUnescapeHCL(t *testing.T) {
	cases := map[string]string{
		`[a-zA-Z0-9-._\\(\\)]+`:   `[a-zA-Z0-9-._\(\)]+`,
		`[^\\/\"\\[\\]:|<>]`:      `[^\/"\[\]:|<>]`,
		`^(?=.{3,24}$)[a-z0-9]+$`: `^(?=.{3,24}$)[a-z0-9]+$`,
		`trailing\`:               `trailing\`,
	}

	for input, want := range cases {
		if got := unescapeHCL(input); got != want {
			t.Errorf("unescapeHCL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMicrosoftDefinitionsCompile(t *testing.T) {
	definitions, err := LoadMicrosoftDefinitions()
	if err != nil {
		t.Fatalf("LoadMicrosoftDefinitions: %v", err)
	}
	if len(definitions) < 200 {
		t.Fatalf("expected the full resourceDefinition.json, got %d entries", len(definitions))
	}

	for name, definition := range definitions {
		if _, err := compileRule(definition.Regex); err != nil {
			t.Errorf("%s: regex %q does not compile: %v", name, definition.Regex, err)
		}
	}

	// The unescaped regex accepts what the HCL module accepts.
	vm := &Rule{Key: "vm", Regex: definitions["windows_virtual_machine"].Regex}
	if violations, err := vm.Check("vmprodpay01"); err != nil || violations != nil {
		t.Errorf("windows_virtual_machine rejected vmprodpay01: %v %v", violations, err)
	}
	if violations, _ := vm.Check("vm/prod"); violations == nil {
		t.Error("windows_virtual_machine accepted a slash")
	}
}

func TestBundledRulesResolve(t *testing.T) {
	schema := resolve(t, "", nil)
	table := AsMap(GetKey(schema.Rules, "Azure"))
	if len(table) == 0 {
		t.Fatal("the bundled rules have no Azure table")
	}

	for key := range table {
		id, err := ParseResourceID("Azure::" + key)
		if err != nil {
			t.Fatalf("rules key %q: %v", key, err)
		}
		rule, err := schema.Rule(Request{Resource: id})
		if err != nil {
			t.Errorf("rules.Azure.%q: %v", key, err)
			continue
		}
		if rule == nil || rule.MaxLength == 0 || rule.Regex == "" {
			t.Errorf("rules.Azure.%q resolved without limits: %#v", key, rule)
			continue
		}
		if _, err := compileRule(rule.Regex); err != nil {
			t.Errorf("rules.Azure.%q: regex %q does not compile: %v", key, rule.Regex, err)
		}
	}
}

// bundledSchema resolves the bundled convention with the sample parameters
// and a fixed seed, so <UNIQUE_ID_n> is deterministic.
func bundledSchema(t *testing.T) *Schema {
	t.Helper()

	schema, err := Resolve(Config{
		Convention: "default",
		Parameters: sampleParameters,
		RandomSeed: "rules-test",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	return schema
}

// bundledAzureKeys lists every Azure abbreviation key of a schema, sorted.
func bundledAzureKeys(schema *Schema) []string {
	keys := make([]string, 0)
	for key, abbreviation := range AsMap(GetKey(schema.Abbreviations, "Azure")) {
		// A type without an abbreviation (a DNS zone is named by its domain)
		// is never rendered from a pattern.
		if abbreviation == nil {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

// TestBundledConventionPassesItsRules renders every abbreviation of the
// bundled convention, without and with the optional purpose, and checks each
// name against its rule. The purpose run uses a shorter name: Kusto (22),
// Service Fabric (23) and Synapse Spark pools (15) leave no room for
// "payments" plus a purpose, and the rule rightly rejects that name.
func TestBundledConventionPassesItsRules(t *testing.T) {
	schema := bundledSchema(t)
	keys := bundledAzureKeys(schema)

	for _, parameters := range []map[string]interface{}{nil, {"name": "pay", "purpose": "api"}} {
		checked := 0
		for _, key := range keys {
			req := request(t, "Azure::"+key, "")
			req.Parameters = parameters

			names, err := schema.Generate(req)
			if err != nil {
				t.Errorf("%s: %v", key, err)
				continue
			}

			rule, err := schema.Rule(req)
			if err != nil {
				t.Errorf("%s: %v", key, err)
				continue
			}
			if rule == nil {
				continue
			}
			checked++

			violations, err := schema.Validate(req, names)
			if err != nil {
				t.Errorf("%s: %v", key, err)
				continue
			}
			for _, violation := range violations {
				t.Errorf("the bundled convention breaks its own rule (parameters %v): %s", parameters, violation)
			}
		}

		if checked < 100 {
			t.Errorf("expected most abbreviations to have a rule, only %d of %d did", checked, len(keys))
		}
	}
}

// TestBundledGlobalNamesCarryUniqueID asserts that every type whose rule is
// unique across Azure renders <UNIQUE_ID_3> and no <INDEX>: an index alone
// collides with another tenant's name.
func TestBundledGlobalNamesCarryUniqueID(t *testing.T) {
	schema := bundledSchema(t)

	global := 0
	for _, key := range bundledAzureKeys(schema) {
		req := request(t, "Azure::"+key, "")

		rule, err := schema.Rule(req)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if rule == nil || rule.Scope != "global" {
			continue
		}
		global++

		pattern, err := schema.Pattern(req)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if !strings.Contains(pattern, "<UNIQUE_ID_3>") || strings.Contains(pattern, "<INDEX") {
			t.Errorf("%s is unique across Azure, but its pattern is %q", key, pattern)
		}
	}

	if global < 20 {
		t.Errorf("expected the globally unique types to be checked, only %d were", global)
	}
}

// TestBundledConventionNames pins what the bundled convention renders for a
// few types, without and with a purpose.
func TestBundledConventionNames(t *testing.T) {
	schema := bundledSchema(t)

	cases := []struct {
		resource, purpose, want string
	}{
		{"Azure::Microsoft.Resources/resourceGroups", "", "rg-we-prod-payments-01"},
		{"Azure::Microsoft.Resources/resourceGroups", "api", "rg-we-prod-payments-api-01"},
		{"Azure::Microsoft.Network/virtualNetworks/subnets", "api", "snet-prod-payments-api-01"},
		{"Azure::Microsoft.Compute/virtualMachines", "api", "vmprodpaymentsapi01"},
		{"Azure::Microsoft.Storage/storageAccounts", "", "stweprodpayments899"},
		{"Azure::Microsoft.Storage/storageAccounts", "api", "stweprodpaymentsapi899"},
		{"Azure::Microsoft.KeyVault/vaults", "", "kv-prod-payments-899"},
		{"Azure::Microsoft.Web/sites::function", "api", "func-we-prod-payments-api-899"},
	}

	for _, c := range cases {
		req := request(t, c.resource, "")
		if c.purpose != "" {
			req.Parameters = map[string]interface{}{"purpose": c.purpose}
		}

		names, err := schema.Generate(req)
		if err != nil {
			t.Fatalf("%s: %v", c.resource, err)
		}
		if names[0] != c.want {
			t.Errorf("%s (purpose %q) = %q, want %q", c.resource, c.purpose, names[0], c.want)
		}
	}
}

func TestRuleResolutionOrder(t *testing.T) {
	schema := ruleSchema()

	cases := []struct {
		resource, namingID string
		wantKey            string
		wantMax            int
	}{
		// <type>::<naming_id> is not set, <type>::<kind> wins.
		{"Azure::Microsoft.Compute/disks::os", "", "Microsoft.Compute/disks::os", 6},
		// No rule for the kind, the type's rule applies.
		{"Azure::Microsoft.Compute/disks::data", "", "Microsoft.Compute/disks", 10},
		// The naming_id wins over the kind.
		{"Azure::Microsoft.Compute/disks::data", "os", "Microsoft.Compute/disks::os", 6},
	}

	for _, c := range cases {
		rule, err := schema.Rule(request(t, c.resource, c.namingID))
		if err != nil {
			t.Fatalf("%s: %v", c.resource, err)
		}
		if rule == nil || rule.Key != c.wantKey || rule.MaxLength != c.wantMax {
			t.Errorf("%s (naming_id %q): got %#v, want key %q max %d", c.resource, c.namingID, rule, c.wantKey, c.wantMax)
		}
	}

	// A kind rule is a whole rule: it does not inherit the type's regex.
	osRule, _ := schema.Rule(request(t, "Azure::Microsoft.Compute/disks::os", ""))
	if osRule.Regex != "" {
		t.Errorf("the kind rule inherited the type's regex %q", osRule.Regex)
	}

	if rule, err := schema.Rule(request(t, "Azure::Microsoft.Web/sites", "")); err != nil || rule != nil {
		t.Errorf("a type without a rule must resolve to nil, got %#v %v", rule, err)
	}
}

func TestRuleExplicitValuesWinOverMicrosoft(t *testing.T) {
	rule, err := ruleSchema().Rule(request(t, "Azure::Microsoft.KeyVault/vaults", ""))
	if err != nil {
		t.Fatalf("Rule: %v", err)
	}

	if rule.Microsoft != "key_vault" || rule.MinLength != 3 || rule.Scope != "global" {
		t.Errorf("the Microsoft entry was not applied: %#v", rule)
	}
	if rule.MaxLength != 12 {
		t.Errorf("max_length = %d, want the explicit 12 over Microsoft's 24", rule.MaxLength)
	}
}

func TestRuleCheck(t *testing.T) {
	schema := ruleSchema()

	cases := []struct {
		resource, name string
		wantViolations []string
	}{
		{"Azure::Microsoft.Storage/storageAccounts", "stweprodpay01", nil},
		{"Azure::Microsoft.Storage/storageAccounts", "st", []string{"minimum is 3"}},
		{"Azure::Microsoft.Storage/storageAccounts", "st-we-prod", []string{"does not match"}},
		{"Azure::Microsoft.Storage/storageAccounts", "stweprodpaymentsworkload01", []string{"maximum is 24"}},
		// Microsoft's key vault regex forbids "--" with a negative lookahead.
		{"Azure::Microsoft.KeyVault/vaults", "kv--prod", []string{"does not match"}},
		{"Azure::Microsoft.KeyVault/vaults", "kv-we-prod", nil},
		// Length is counted in characters, not bytes.
		{"Azure::Microsoft.Compute/disks::data", "äöüäöüäöüä", []string{"does not match"}},
		{"Azure::Microsoft.Compute/disks::data", "osdisk-prod-01", []string{"maximum is 10", "does not match"}},
	}

	for _, c := range cases {
		messages, err := schema.Validate(request(t, c.resource, ""), []string{c.name})
		if err != nil {
			t.Fatalf("%s %q: %v", c.resource, c.name, err)
		}
		if c.wantViolations == nil {
			if len(messages) != 0 {
				t.Errorf("%s %q: expected no violation, got %v", c.resource, c.name, messages)
			}
			continue
		}
		if len(messages) != 1 {
			t.Errorf("%s %q: expected one message, got %v", c.resource, c.name, messages)
			continue
		}
		for _, want := range c.wantViolations {
			if !strings.Contains(messages[0], want) {
				t.Errorf("%s %q: message %q lacks %q", c.resource, c.name, messages[0], want)
			}
		}
	}

	// Ten characters, ten bytes more: the length check must pass.
	diskRule, _ := schema.Rule(request(t, "Azure::Microsoft.Compute/disks::data", ""))
	diskRule.Regex = ""
	if violations, _ := diskRule.Check("äöüäöüäöüä"); violations != nil {
		t.Errorf("ten characters counted as more than ten: %v", violations)
	}
}

func TestValidateOffSkipsTheCheck(t *testing.T) {
	schema := ruleSchema()
	schema.Validation = ValidationOff

	messages, err := schema.Validate(request(t, "Azure::Microsoft.Storage/storageAccounts", ""), []string{"x"})
	if err != nil || messages != nil {
		t.Errorf("validation off must not check, got %v %v", messages, err)
	}
}

func TestRuleUnknownMicrosoftEntryIsAnError(t *testing.T) {
	schema := ruleSchema()
	AsMap(GetKey(schema.Rules, "Azure"))["Microsoft.Web/sites"] = map[string]interface{}{"microsoft": "no_such_entry"}

	_, err := schema.Rule(request(t, "Azure::Microsoft.Web/sites", ""))
	if err == nil || !strings.Contains(err.Error(), "no_such_entry") {
		t.Errorf("expected an error naming the missing entry, got %v", err)
	}
}

func TestResolveRulesOverrideWinsAndSiblingsSurvive(t *testing.T) {
	path := writeOverride(t, `
rules:
  Azure:
    Microsoft.Storage/storageAccounts:
      max_length: 20
`)
	schema := resolve(t, path, nil)

	storage, err := schema.Rule(request(t, "Azure::Microsoft.Storage/storageAccounts", ""))
	if err != nil {
		t.Fatalf("Rule: %v", err)
	}
	if storage.MaxLength != 20 || storage.Microsoft != "storage_account" {
		t.Errorf("the override must merge into the bundled entry, got %#v", storage)
	}

	vault, err := schema.Rule(request(t, "Azure::Microsoft.KeyVault/vaults", ""))
	if err != nil || vault == nil || vault.MaxLength != 24 {
		t.Errorf("a sibling rule was dropped: %#v %v", vault, err)
	}
}

func TestResolveValidation(t *testing.T) {
	if schema := resolve(t, "", nil); schema.Validation != ValidationError {
		t.Errorf("default validation = %q, want %q", schema.Validation, ValidationError)
	}

	for _, mode := range ValidationModes {
		schema, err := Resolve(Config{Convention: "default", Validation: mode})
		if err != nil || schema.Validation != mode {
			t.Errorf("validation %q: got %#v %v", mode, schema, err)
		}
	}

	if _, err := Resolve(Config{Convention: "default", Validation: "strict"}); err == nil {
		t.Error("expected an error for an unknown validation mode")
	}
}
