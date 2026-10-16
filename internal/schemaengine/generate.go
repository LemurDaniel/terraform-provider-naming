package schemaengine

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ResourceID is a parsed resource identifier of the form
// "<Provider>::<Namespace>/<Type>[::<kind>]", e.g.
// "Azure::Microsoft.Compute/disks::os".
//
// The kind is a modifier selecting a variant of the same resource type - it
// picks both the abbreviation ("disks::os" => osdisk, "disks::data" => disk)
// and, optionally, a pattern of its own. An omitted kind is "default".
type ResourceID struct {
	Full     string
	Provider string
	Type     string
	Kind     string
}

// ParseResourceID splits a resource identifier into its three parts.
func ParseResourceID(s string) (ResourceID, error) {
	parts := strings.Split(s, "::")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ResourceID{}, fmt.Errorf(
			"%q is not a valid resource identifier - expected \"<Provider>::<Namespace>/<Type>[::<kind>]\", e.g. \"Azure::Microsoft.Storage/storageAccounts\" or \"Azure::Microsoft.Compute/disks::os\"",
			s,
		)
	}

	id := ResourceID{
		Full:     s,
		Provider: parts[0],
		Type:     parts[1],
		Kind:     "default",
	}
	if len(parts) > 2 && parts[2] != "" {
		id.Kind = parts[2]
	}

	return id, nil
}

// Request is a single name-generation call: which resource to name, which
// variant of it, how many names, and the call-specific parameters layered
// over the schema's.
type Request struct {
	Resource   ResourceID
	NamingID   string
	IndexStart int
	IndexCount int
	Parameters map[string]interface{}
}

// Abbreviation resolves the abbreviation for the request's resource, trying
// the most specific key first:
//
//	<type>::<naming_id>  ->  <type>::<kind>  ->  <type>::default  ->  <type>
//
// A resource with no abbreviation anywhere is an error rather than an empty
// segment in the finished name.
func (s *Schema) Abbreviation(req Request) (string, error) {
	table := AsMap(GetKey(s.Abbreviations, req.Resource.Provider))

	keys := make([]string, 0, 4)
	if req.NamingID != "" {
		keys = append(keys, req.Resource.Type+"::"+req.NamingID)
	}
	keys = append(keys,
		req.Resource.Type+"::"+req.Resource.Kind,
		req.Resource.Type+"::default",
		req.Resource.Type,
	)

	for _, key := range keys {
		if abbreviation, ok := GetKey(table, key).(string); ok && abbreviation != "" {
			return abbreviation, nil
		}
	}

	return "", fmt.Errorf(
		"no abbreviation found for %q - add one under abbreviations.%s as %q (tried %s)",
		req.Resource.Full, req.Resource.Provider, req.Resource.Type+"::"+req.Resource.Kind,
		strings.Join(quoteAll(keys), ", "),
	)
}

// LowerCaseFor reports whether the finished name is lower-cased, trying:
//
//	<provider>.<resource type pattern>  ->  <provider>.default  ->  default
//
// Keys below a provider are matched against the resource type as regular
// expressions with "*" expanded to ".*", so "Microsoft.Compute/*" covers
// every type in that namespace. Keys are tried in sorted order so a type
// matching several patterns always resolves the same way.
func (s *Schema) LowerCaseFor(id ResourceID) bool {
	table := AsMap(GetKey(s.EnforceLowerCase, id.Provider))

	patterns := make([]string, 0, len(table))
	for key := range table {
		if key != "default" {
			patterns = append(patterns, key)
		}
	}
	sort.Strings(patterns)

	for _, pattern := range patterns {
		expanded := strings.ReplaceAll(pattern, "*", ".*")
		if matched, err := regexp.MatchString(expanded, id.Type); err == nil && matched {
			if value, ok := GetKey(table, pattern).(bool); ok {
				return value
			}
		}
	}

	if value, ok := GetKey(table, "default").(bool); ok {
		return value
	}
	if value, ok := GetKey(s.EnforceLowerCase, "default").(bool); ok {
		return value
	}

	return false
}

// parameters assembles the parameter table a pattern renders from: the
// schema's default parameters with the request's layered over them, each
// value passed through the schema's mappings, plus the derived naming_id,
// abbreviation and type entries.
//
// Every key is lower-cased, because a pattern token resolves by its
// lower-cased name (<LOCATION> -> "location").
func (s *Schema) parameters(req Request, abbreviation string) map[string]string {
	merged := DeepMergeUserWins(s.DefaultParameters, req.Parameters)

	resolved := map[string]string{
		// A parameter named "index" overrides the request's start below.
		"index": strconv.Itoa(req.IndexStart),
	}

	for name, value := range merged {
		resolved[strings.ToLower(name)] = s.mapValue(name, stringify(value))
	}

	// These are derived from the resource identifier, so they are set last
	// and cannot be shadowed by a parameter of the same name.
	namingID := req.NamingID
	if namingID == "" {
		namingID = req.Resource.Kind
	}
	resolved["naming_id"] = namingID
	resolved["abbreviation"] = abbreviation
	resolved["type"] = abbreviation

	return resolved
}

// mapValue translates a parameter value through the schema's mappings table
// (e.g. location "westeurope" -> "we"). Both the parameter name and the value
// are matched case-insensitively; an unmapped value passes through unchanged.
//
// Matching is case-insensitive but not whitespace-insensitive: the bundled
// convention lists "westeurope" and "West Europe" as separate keys precisely
// because "West Europe" does not match the former.
func (s *Schema) mapValue(name, value string) string {
	for mappedName, mappings := range s.Mappings {
		if !strings.EqualFold(mappedName, name) {
			continue
		}

		table := AsMap(mappings)
		keys := make([]string, 0, len(table))
		for key := range table {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if strings.EqualFold(key, value) {
				return stringify(table[key])
			}
		}
	}

	return value
}

// Pattern resolves the pattern string for the request, trying the most
// specific key first:
//
//	<type>.<naming_id>  ->  <type>.<kind>  ->  <type>.default
//	  ->  <provider>.default  ->  default
//
// Unlike the lower-case settings, pattern keys are matched exactly - a "*"
// in a pattern key is not expanded.
func (s *Schema) Pattern(req Request) (string, error) {
	providerTable := AsMap(GetKey(s.Patterns, req.Resource.Provider))
	typeEntry := GetKey(providerTable, req.Resource.Type)

	// A type may carry a bare pattern string instead of a map of kinds, in
	// which case it is that type's default.
	if pattern, ok := typeEntry.(string); ok && pattern != "" {
		return pattern, nil
	}

	typeTable := AsMap(typeEntry)

	candidates := make([]interface{}, 0, 5)
	if req.NamingID != "" {
		candidates = append(candidates, GetKey(typeTable, req.NamingID))
	}
	candidates = append(candidates,
		GetKey(typeTable, req.Resource.Kind),
		GetKey(typeTable, "default"),
		GetKey(providerTable, "default"),
		GetKey(s.Patterns, "default"),
	)

	for _, candidate := range candidates {
		if pattern, ok := candidate.(string); ok && pattern != "" {
			return pattern, nil
		}
	}

	return "", fmt.Errorf(
		"no pattern found for %q - add one under patterns.%s, or a patterns.default",
		req.Resource.Full, req.Resource.Provider,
	)
}

// component is one piece of a split pattern: either a literal run of text or
// a single <TOKEN>.
type component struct {
	// Value is the token without its angle brackets, or the literal text.
	Value string
	// ParamName is the lower-cased parameter the token resolves from, with
	// the optional marker and the format suffix stripped.
	ParamName string
	// Format is the printf format applied to the value, "%s" by default.
	Format string

	IsParameter bool
	IsRequired  bool
	IsIndex     bool
	IsUniqueID  bool
	// UniqueIDLength is the number of UUID characters a <UNIQUE_ID_n> token
	// renders.
	UniqueIDLength int
}

// splitPattern breaks a pattern into literal and token components.
//
// Tokens are delimited by angle brackets, so wrapping every bracket in a
// marker and splitting on it yields the components in order: "a<B>c" becomes
// "a", "<B>", "c".
func splitPattern(pattern string) []component {
	const marker = "\x00"

	wrapped := strings.ReplaceAll(pattern, "<", marker+"<")
	wrapped = strings.ReplaceAll(wrapped, ">", ">"+marker)

	components := make([]component, 0, 8)
	for _, raw := range strings.Split(wrapped, marker) {
		if raw == "" {
			continue
		}

		isParameter := strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">")
		value := strings.TrimSuffix(strings.TrimPrefix(raw, "<"), ">")

		c := component{
			Value:       value,
			IsParameter: isParameter,
			// A "?" marks the token optional: it is dropped from the name
			// when its parameter is absent, instead of failing.
			IsRequired: !strings.Contains(raw, "?"),
			Format:     "%s",
		}

		name := value
		if separator := strings.Index(value, ";"); separator >= 0 {
			name = value[:separator]
			c.Format = value[separator+1:]
		}
		c.ParamName = strings.ToLower(strings.ReplaceAll(name, "?", ""))

		if isParameter {
			lowered := strings.ToLower(value)
			c.IsIndex = strings.Contains(lowered, "index")
			c.IsUniqueID = strings.Contains(lowered, "unique_id")
			if c.IsUniqueID {
				// <UNIQUE_ID_4> -> 4. An unparseable length renders nothing,
				// which Generate reports as an error.
				length, err := strconv.Atoi(strings.TrimPrefix(c.ParamName, "unique_id_"))
				if err == nil {
					c.UniqueIDLength = length
				}
			}
		}

		components = append(components, c)
	}

	return components
}

// formatSpec matches a single printf conversion, e.g. the "%02s" in "-%02s".
var formatSpec = regexp.MustCompile(`%([-+ #0]*)(\d*)([sdvf])`)

// applyFormat renders value through a pattern's printf format, keeping any
// literal text around the conversion (so "-%s" yields "-api").
//
// Terraform's format() zero-pads a string conversion, e.g.
// format("%02s", 1) == "01", which is what makes <INDEX;%02s> produce "01"
// rather than " 1". Go's fmt pads strings with spaces instead, so the padding
// is applied here rather than delegated to Sprintf.
func applyFormat(format, value string) string {
	match := formatSpec.FindStringSubmatchIndex(format)
	if match == nil {
		// No conversion at all - the format is literal text.
		return format
	}

	flags := format[match[2]:match[3]]
	width, _ := strconv.Atoi(format[match[4]:match[5]])

	padded := value
	if pad := width - len([]rune(value)); pad > 0 {
		filler := " "
		if strings.Contains(flags, "0") {
			filler = "0"
		}
		if strings.Contains(flags, "-") {
			padded = value + strings.Repeat(" ", pad)
		} else {
			padded = strings.Repeat(filler, pad) + value
		}
	}

	return format[:match[0]] + padded + format[match[1]:]
}

// Generate renders the names for a request - IndexCount of them, starting at
// IndexStart shifted by the schema's index modifier.
func (s *Schema) Generate(req Request) ([]string, error) {
	if req.IndexCount < 1 {
		return nil, fmt.Errorf("index.count must be at least 1 - it is the number of names to generate")
	}
	if req.IndexStart < 0 {
		return nil, fmt.Errorf("index.start must not be negative")
	}

	abbreviation, err := s.Abbreviation(req)
	if err != nil {
		return nil, err
	}

	pattern, err := s.Pattern(req)
	if err != nil {
		return nil, err
	}

	parameters := s.parameters(req, abbreviation)

	start, err := strconv.Atoi(parameters["index"])
	if err != nil {
		return nil, fmt.Errorf("parameter \"index\" must be a number, got %q", parameters["index"])
	}
	// The index modifier shifts every index, so a convention with
	// index_modifier: 1 numbers the first name 01 rather than 00.
	start += s.IndexModifier

	uniqueID := strings.ReplaceAll(s.UniqueID, "-", "")
	components := splitPattern(pattern)
	lowerCase := s.LowerCaseFor(req.Resource)

	names := make([]string, 0, req.IndexCount)
	for offset := 0; offset < req.IndexCount; offset++ {
		var builder strings.Builder

		for _, c := range components {
			if !c.IsParameter {
				builder.WriteString(c.Value)
				continue
			}

			if c.IsIndex {
				builder.WriteString(applyFormat(c.Format, strconv.Itoa(start+offset)))
				continue
			}

			if c.IsUniqueID {
				if uniqueID == "" {
					return nil, fmt.Errorf(
						"pattern %q for %q contains <%s>, but no random_seed is configured - set random_seed on the provider to resolve it deterministically",
						pattern, req.Resource.Full, c.Value,
					)
				}
				if c.UniqueIDLength <= 0 || c.UniqueIDLength > len(uniqueID) {
					return nil, fmt.Errorf(
						"pattern %q for %q contains <%s>, but %q is not a usable length (1-%d)",
						pattern, req.Resource.Full, c.Value,
						strings.TrimPrefix(c.ParamName, "unique_id_"), len(uniqueID),
					)
				}
				builder.WriteString(applyFormat(c.Format, uniqueID[:c.UniqueIDLength]))
				continue
			}

			value, ok := parameters[c.ParamName]
			if !ok || value == "" {
				if c.IsRequired {
					return nil, fmt.Errorf(
						"pattern %q for %q requires the parameter %q - set it in the provider's parameters, the convention's default_parameters, or this call's parameters",
						pattern, req.Resource.Full, c.ParamName,
					)
				}
				// An optional token with no value contributes nothing, which
				// is why its separator belongs inside the token
				// (<?SUBNAME;-%s>) rather than next to it.
				continue
			}

			builder.WriteString(applyFormat(c.Format, value))
		}

		name := builder.String()
		if lowerCase {
			name = strings.ToLower(name)
		}
		// A doubled separator is collapsed, which keeps a pattern readable
		// when a middle token resolves to nothing.
		names = append(names, strings.ReplaceAll(name, "--", "-"))
	}

	return names, nil
}

// stringify renders a YAML-decoded scalar as the string a pattern uses.
func stringify(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return quoted
}
