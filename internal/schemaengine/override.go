package schemaengine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadOverrideFile reads a user-supplied YAML file from disk and decodes it
// into a generic map, for use as a partial override of a bundled
// convention. An empty path means "no override" and returns a nil map.
func LoadOverrideFile(path string) (map[string]interface{}, error) {
	if path == "" {
		return nil, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read naming file %q: %w", path, err)
	}

	var decoded map[string]interface{}
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("failed to parse naming file %q: %w", path, err)
	}

	return decoded, nil
}
