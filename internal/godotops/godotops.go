// Package godotops embeds the GDScript bridge the scene and UID tools drive
// under `godot --headless --script`, and materializes it for Godot at runtime.
package godotops

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

//go:embed godot_operations.gd
var operationsScript string

//go:embed validate_script.gd
var validateScript string

var (
	once sync.Once
	path string
	err  error
)

var (
	validateOnce sync.Once
	validatePath string
	validateErr  error
)

// OperationsScript materializes the embedded godot_operations.gd to a file and
// returns its path. The file is written once per process; Godot needs a real
// path for --script, and the binary is dependency-free so it cannot ship a
// loose file.
func OperationsScript() (string, error) {
	once.Do(func() {
		dir, mkErr := os.MkdirTemp("", "go-dot-mcp-")
		if mkErr != nil {
			err = fmt.Errorf("create script dir: %w", mkErr)
			return
		}
		scriptPath := filepath.Join(dir, "godot_operations.gd")
		if writeErr := os.WriteFile(scriptPath, []byte(operationsScript), 0o644); writeErr != nil { //nolint:gosec // script is operator-provided and read-only.
			err = fmt.Errorf("write operations script: %w", writeErr)
			return
		}
		path = scriptPath
	})
	return path, err
}

// ValidateScript materializes the embedded validate_script.gd to a file and
// returns its path. Like OperationsScript the file is written once per process,
// because Godot needs a real path for --script.
func ValidateScript() (string, error) {
	validateOnce.Do(func() {
		dir, mkErr := os.MkdirTemp("", "go-dot-mcp-")
		if mkErr != nil {
			validateErr = fmt.Errorf("create script dir: %w", mkErr)
			return
		}
		scriptPath := filepath.Join(dir, "validate_script.gd")
		if writeErr := os.WriteFile(scriptPath, []byte(validateScript), 0o644); writeErr != nil { //nolint:gosec // script is operator-provided and read-only.
			validateErr = fmt.Errorf("write validate script: %w", writeErr)
			return
		}
		validatePath = scriptPath
	})
	return validatePath, validateErr
}
