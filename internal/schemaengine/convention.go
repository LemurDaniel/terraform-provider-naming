package schemaengine

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed convention/*.naming.yaml
var conventionFS embed.FS

// LoadConvention loads a bundled naming convention by name (e.g. "default")
// from convention/<name>.naming.yaml and decodes it into a generic map.
func LoadConvention(name string) (map[string]interface{}, error) {
	path := fmt.Sprintf("convention/%s.naming.yaml", name)

	raw, err := conventionFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("naming convention %q was not found", name)
	}

	var decoded map[string]interface{}
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("failed to parse convention %q: %w", name, err)
	}

	return decoded, nil
}
