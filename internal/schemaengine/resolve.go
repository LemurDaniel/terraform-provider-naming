package schemaengine


// GetKey returns m[key], or nil if m is nil (a missing/unset override).
func GetKey(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return nil
	}
	return m[key]
}

// Coalesce returns the first non-nil value, in order - the same "override
// falls back to base" behavior as Terraform's own coalesce().
func Coalesce(values ...interface{}) interface{} {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

// AsMap type-asserts v to a map[string]interface{}, returning nil if it
// isn't one (e.g. the key was unset or held a different YAML type).
func AsMap(v interface{}) map[string]interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}

// DeepMergeUserWins recursively merges override into base. For a shared key
// where both sides are maps, the merge recurses; otherwise the override
// value replaces the base value outright. Keys present only in base are
// kept as-is.
func DeepMergeUserWins(base, override map[string]interface{}) map[string]interface{} {
	if base == nil && override == nil {
		return map[string]interface{}{}
	}

	merged := make(map[string]interface{}, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}

	for k, overrideValue := range override {
		baseValue, exists := merged[k]
		if exists {
			baseChild, baseIsMap := baseValue.(map[string]interface{})
			overrideChild, overrideIsMap := overrideValue.(map[string]interface{})
			if baseIsMap && overrideIsMap {
				merged[k] = DeepMergeUserWins(baseChild, overrideChild)
				continue
			}
		}
		merged[k] = overrideValue
	}

	return merged
}
