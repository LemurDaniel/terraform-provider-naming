package schemaengine

import (
	"fmt"

	"github.com/google/uuid"
)

// seedNamespace is an arbitrary, fixed namespace used to derive deterministic
// (v5) UUIDs from a user-supplied random_seed. It has no meaning beyond making
// seeded UUIDs unique to this provider.
var seedNamespace = uuid.MustParse("d29c7fb1-4c07-4f3c-9d90-14a765d5f2f0")

// DeterministicUUID derives a stable (v5) UUID from seed, so the same
// random_seed always resolves to the same id across plans and applies.
func DeterministicUUID(seed string) string {
	return uuid.NewSHA1(seedNamespace, []byte(seed)).String()
}

// Schema is a fully resolved naming schema: a bundled convention with the
// caller's override merged over it, plus the runtime parameters and the unique
// id that <UNIQUE_ID_n> renders from.
//
// The nested tables stay as map[string]interface{} because they are
// YAML-shaped by design - mixing strings and nested objects at the same level
// (see Patterns) - and the provider hands them straight back to Terraform as
// dynamic values.
type Schema struct {
	IndexModifier     int
	EnforceLowerCase  map[string]interface{}
	Abbreviations     map[string]interface{}
	Mappings          map[string]interface{}
	Patterns          map[string]interface{}
	DefaultParameters map[string]interface{}

	// UniqueID is the resolved random_seed UUID, or "" when no seed was set.
	// Rendering a pattern that contains <UNIQUE_ID_n> without one is an error
	// rather than a silently empty segment.
	UniqueID string
}

// Config is the provider-level configuration a Schema is resolved from.
type Config struct {
	// Convention names a bundled convention file, e.g. "default".
	Convention string
	// NamingFile is an optional path to a YAML file partially overriding it.
	NamingFile string
	// Parameters are the runtime parameters (location, environment, name, ...)
	// merged on top of the convention's own default_parameters.
	Parameters map[string]interface{}
	// RandomSeed pins <UNIQUE_ID_n> deterministically. Empty means unset.
	RandomSeed string
}

// Resolve loads the bundled convention named by cfg.Convention, deep-merges
// cfg.NamingFile's contents over it, and layers the parameters on top.
//
// Every table is merged per key with the CALLER's value winning, so an
// override may set a single abbreviation or a single pattern without
// restating the block around it; keys it does not mention keep the bundled
// value. A shallow merge would instead drop every sibling under the key it
// touches, and merging the other way round would silently ignore the
// override - both were real bugs in the predecessor modules.
func Resolve(cfg Config) (*Schema, error) {
	convention := cfg.Convention
	if convention == "" {
		convention = "default"
	}

	conventionMap, err := LoadConvention(convention)
	if err != nil {
		return nil, err
	}

	overrideMap, err := LoadOverrideFile(cfg.NamingFile)
	if err != nil {
		return nil, err
	}

	indexModifier, err := toInt(Coalesce(
		GetKey(overrideMap, "index_modifier"),
		GetKey(conventionMap, "index_modifier"),
	))
	if err != nil {
		return nil, fmt.Errorf("index_modifier: %w", err)
	}

	schema := &Schema{
		IndexModifier: indexModifier,

		// enforce_lower_case mixes a scalar default with nested per-provider
		// objects at the same level; the recursive merge handles both, since a
		// scalar override replaces a scalar and a map override merges into a
		// map.
		EnforceLowerCase: deepMerge(conventionMap, overrideMap, "enforce_lower_case"),
		Abbreviations:    deepMerge(conventionMap, overrideMap, "abbreviations"),
		Mappings:         deepMerge(conventionMap, overrideMap, "mappings"),

		// patterns is three levels deep (global string, provider string,
		// per-type object of kind => pattern) and is merged at every one of
		// them, so overriding one kind keeps its sibling kinds.
		Patterns: deepMerge(conventionMap, overrideMap, "patterns"),

		// default_parameters has three layers, each winning over the last:
		// the bundled convention, the override file, then the provider's own
		// parameters.
		DefaultParameters: DeepMergeUserWins(
			deepMerge(conventionMap, overrideMap, "default_parameters"),
			cfg.Parameters,
		),
	}

	if cfg.RandomSeed != "" {
		schema.UniqueID = DeterministicUUID(cfg.RandomSeed)
	}

	return schema, nil
}

// deepMerge merges the override's value for key over the convention's,
// tolerating either side being absent or holding a non-map value.
func deepMerge(convention, override map[string]interface{}, key string) map[string]interface{} {
	return DeepMergeUserWins(
		AsMap(GetKey(convention, key)),
		AsMap(GetKey(override, key)),
	)
}

// toInt coerces a YAML-decoded number to an int. yaml.v3 decodes integers as
// int, but a convention written as "1.0" decodes as float64, and a quoted one
// as a string - all three are meant to be the same index modifier.
func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case nil:
		return 0, nil
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	case string:
		var parsed int
		if _, err := fmt.Sscanf(val, "%d", &parsed); err != nil {
			return 0, fmt.Errorf("%q is not a number", val)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}
