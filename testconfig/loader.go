// Package testconfig provides runtime-configurable test payloads loaded from
// JSON files in the data/ directory. Edit a JSON file and re-run tests —
// no code change or commit required.
package testconfig

import (
	"encoding/json"
	"fmt"
	"os"
)

// Load reads the JSON file at path and unmarshals it into dst.
// dst must be a pointer to a struct.
func Load(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("testconfig: read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("testconfig: parse %s: %w", path, err)
	}
	return nil
}
