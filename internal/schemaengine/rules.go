package schemaengine

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	"gopkg.in/yaml.v3"
)

// The rules a rendered name is checked against: the bundled rules file, which
// maps each resource type to an entry of Microsoft's resourceDefinition.json
// (vendored from Azure/naming/azurerm) and corrects that entry where it is
// wrong, plus the vendored file itself.
//
//go:embed rules/azure.rules.yaml rules/microsoft/resourceDefinition.json
var rulesFS embed.FS

const (
	rulesPath                 = "rules/azure.rules.yaml"
	microsoftDefinitionsPath  = "rules/microsoft/resourceDefinition.json"
	regexMatchTimeout         = time.Second
	ValidationError           = "error"
	ValidationWarn            = "warn"
	ValidationOff             = "off"
	defaultValidationSeverity = ValidationError
)

// ValidationModes lists the accepted values of the provider's validation
// argument, the default first.
var ValidationModes = []string{ValidationError, ValidationWarn, ValidationOff}

// MicrosoftDefinition is one entry of Microsoft's resourceDefinition.json.
//
// Regex is stored as Microsoft's generator writes it into HCL - a string
// literal with its backslashes and quotes escaped once more - so it is
// unescaped once before it is compiled (see unescapeHCL).
type MicrosoftDefinition struct {
	Name   string `json:"name"`
	Length struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"length"`
	Regex  string `json:"regex"`
	Scope  string `json:"scope"`
	Slug   string `json:"slug"`
	Dashes bool   `json:"dashes"`
}

var (
	microsoftOnce        sync.Once
	microsoftDefinitions map[string]MicrosoftDefinition
	microsoftErr         error
)

// LoadMicrosoftDefinitions decodes the vendored resourceDefinition.json once
// and returns it keyed by its azurerm-style name (e.g. "storage_account").
func LoadMicrosoftDefinitions() (map[string]MicrosoftDefinition, error) {
	microsoftOnce.Do(func() {
		raw, err := rulesFS.ReadFile(microsoftDefinitionsPath)
		if err != nil {
			microsoftErr = fmt.Errorf("the bundled Microsoft naming definitions are missing: %w", err)
			return
		}

		var list []MicrosoftDefinition
		if err := json.Unmarshal(raw, &list); err != nil {
			microsoftErr = fmt.Errorf("failed to parse the bundled Microsoft naming definitions: %w", err)
			return
		}

		microsoftDefinitions = make(map[string]MicrosoftDefinition, len(list))
		for _, definition := range list {
			definition.Regex = unescapeHCL(definition.Regex)
			microsoftDefinitions[definition.Name] = definition
		}
	})

	return microsoftDefinitions, microsoftErr
}

// LoadRules decodes the bundled rules file into a generic map, the shape a
// `rules:` block of a convention or a naming file is merged over.
func LoadRules() (map[string]interface{}, error) {
	raw, err := rulesFS.ReadFile(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("the bundled naming rules are missing: %w", err)
	}

	var decoded map[string]interface{}
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("failed to parse the bundled naming rules: %w", err)
	}

	return decoded, nil
}

// unescapeHCL removes the one level of HCL string escaping Microsoft's
// resourceDefinition.json carries, so the regex is the one Terraform evaluates:
//
//	[a-zA-Z0-9-._\\(\\)]   =>  [a-zA-Z0-9-._\(\)]
//	[^\\/\"\\[\\]:|]       =>  [^\/"\[\]:|]
//
// Only "\\" and "\"" are HCL escapes in these strings; any other backslash is
// kept as it is.
func unescapeHCL(s string) string {
	var builder strings.Builder
	builder.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '\\' || s[i+1] == '"') {
			builder.WriteByte(s[i+1])
			i++
			continue
		}
		builder.WriteByte(s[i])
	}

	return builder.String()
}

// Rule is the resolved naming rule for one resource: Microsoft's entry with
// the rules file's and the caller's explicit values over it.
type Rule struct {
	// Key is the rules key that matched, e.g. "Microsoft.Compute/disks::os".
	Key string
	// Microsoft is the resourceDefinition.json entry the limits come from,
	// empty for a rule written out in full.
	Microsoft string
	MinLength int
	MaxLength int
	Regex     string
	// Scope is where the name must be unique: global, subscription,
	// resourceGroup, parent, ...
	Scope string
}

// Rule resolves the naming rule for the request's resource, trying the most
// specific key first:
//
//	<type>::<naming_id>  ->  <type>::<kind>  ->  <type>
//
// A resource without a rule returns nil and is not checked. A rule naming a
// Microsoft entry that does not exist is an error, not a silently skipped
// check.
func (s *Schema) Rule(req Request) (*Rule, error) {
	table := AsMap(GetKey(s.Rules, req.Resource.Provider))

	keys := make([]string, 0, 3)
	if req.NamingID != "" {
		keys = append(keys, req.Resource.Type+"::"+req.NamingID)
	}
	keys = append(keys, req.Resource.Type+"::"+req.Resource.Kind, req.Resource.Type)

	for _, key := range keys {
		entry := AsMap(GetKey(table, key))
		if entry == nil {
			continue
		}
		return buildRule(req.Resource.Provider, key, entry)
	}

	return nil, nil
}

// buildRule fills a rule from its Microsoft entry first, then lets every
// explicit field of the rules entry win over it.
func buildRule(provider, key string, entry map[string]interface{}) (*Rule, error) {
	rule := &Rule{Key: key}

	if name, ok := GetKey(entry, "microsoft").(string); ok && name != "" {
		definitions, err := LoadMicrosoftDefinitions()
		if err != nil {
			return nil, err
		}
		definition, found := definitions[name]
		if !found {
			return nil, fmt.Errorf(
				"rules.%s.%q refers to the Microsoft definition %q, which does not exist in resourceDefinition.json",
				provider, key, name,
			)
		}
		rule.Microsoft = name
		rule.MinLength = definition.Length.Min
		rule.MaxLength = definition.Length.Max
		rule.Regex = definition.Regex
		rule.Scope = definition.Scope
	}

	for field, target := range map[string]*int{"min_length": &rule.MinLength, "max_length": &rule.MaxLength} {
		value := GetKey(entry, field)
		if value == nil {
			continue
		}
		parsed, err := toInt(value)
		if err != nil {
			return nil, fmt.Errorf("rules.%s.%q.%s: %w", provider, key, field, err)
		}
		*target = parsed
	}
	if regex, ok := GetKey(entry, "regex").(string); ok && regex != "" {
		rule.Regex = regex
	}
	if scope, ok := GetKey(entry, "scope").(string); ok && scope != "" {
		rule.Scope = scope
	}

	return rule, nil
}

var compiledRegexes sync.Map // regex source -> *regexp2.Regexp

// compileRule compiles a rule's regex once per process. regexp2 rather than
// Go's regexp, because Microsoft's regexes use lookaheads ((?=.{3,24}$),
// (?!.*--)) that RE2 cannot express.
func compileRule(source string) (*regexp2.Regexp, error) {
	if cached, ok := compiledRegexes.Load(source); ok {
		return cached.(*regexp2.Regexp), nil
	}

	compiled, err := regexp2.Compile(source, regexp2.None)
	if err != nil {
		return nil, err
	}
	// A pathological pattern fails the check instead of hanging the plan.
	compiled.MatchTimeout = regexMatchTimeout

	compiledRegexes.Store(source, compiled)
	return compiled, nil
}

// Check reports every rule the name breaks, or nil when it passes. Length is
// counted in characters, not bytes.
func (r *Rule) Check(name string) ([]string, error) {
	violations := make([]string, 0, 2)

	length := utf8.RuneCountInString(name)
	if r.MinLength > 0 && length < r.MinLength {
		violations = append(violations, fmt.Sprintf("is %d characters long, the minimum is %d", length, r.MinLength))
	}
	if r.MaxLength > 0 && length > r.MaxLength {
		violations = append(violations, fmt.Sprintf("is %d characters long, the maximum is %d", length, r.MaxLength))
	}

	if r.Regex != "" {
		compiled, err := compileRule(r.Regex)
		if err != nil {
			return nil, fmt.Errorf("the rule %q has an invalid regex %q: %w", r.Key, r.Regex, err)
		}
		matched, err := compiled.MatchString(name)
		if err != nil {
			return nil, fmt.Errorf("the rule %q could not be evaluated on %q: %w", r.Key, name, err)
		}
		if !matched {
			violations = append(violations, fmt.Sprintf("does not match %s", r.Regex))
		}
	}

	if len(violations) == 0 {
		return nil, nil
	}
	return violations, nil
}

// Validate checks every name of a request against its resource's rule and
// returns one message per name that breaks it. A resource without a rule, or
// a schema with validation "off", returns nothing.
func (s *Schema) Validate(req Request, names []string) ([]string, error) {
	if s.Validation == ValidationOff {
		return nil, nil
	}

	rule, err := s.Rule(req)
	if err != nil || rule == nil {
		return nil, err
	}

	source := "rules." + req.Resource.Provider + "." + rule.Key
	if rule.Microsoft != "" {
		source += " (Microsoft: " + rule.Microsoft + ")"
	}

	messages := make([]string, 0)
	for _, name := range names {
		violations, err := rule.Check(name)
		if err != nil {
			return nil, err
		}
		if violations == nil {
			continue
		}
		messages = append(messages, fmt.Sprintf(
			"%q for %q %s - rule %s",
			name, req.Resource.Full, strings.Join(violations, " and "), source,
		))
	}

	return messages, nil
}

// resolveValidation checks the provider's validation argument, an empty one
// being the default.
func resolveValidation(mode string) (string, error) {
	if mode == "" {
		return defaultValidationSeverity, nil
	}
	for _, accepted := range ValidationModes {
		if mode == accepted {
			return mode, nil
		}
	}
	return "", fmt.Errorf("validation must be one of %s, got %q", strings.Join(quoteAll(ValidationModes), ", "), mode)
}
