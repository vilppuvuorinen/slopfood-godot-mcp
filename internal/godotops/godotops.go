package godotops

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/fsutil"
)

//go:embed godot_operations.gd
var operationsScript string

//go:embed validate_script.gd
var validateScript string

//go:embed project_probe.gd
var projectProbeScript string

// cachedScript materializes one embedded operation script at most once and
// remembers the resulting path, matching the previous per-script sync.Once.
type cachedScript struct {
	once       sync.Once
	label      string // used in the instrument error message
	base       string // materialize base name
	content    string
	instrument func(string) (string, error) // nil to skip instrumentation
	path       string
	err        error
}

func (c *cachedScript) get() (string, error) {
	c.once.Do(func() {
		content := c.content
		if c.instrument != nil {
			rewritten, instrumentErr := c.instrument(content)
			if instrumentErr != nil {
				c.err = fmt.Errorf("instrument %s script: %w", c.label, instrumentErr)
				return
			}
			content = rewritten
		}
		c.path, c.err = materialize(c.base, content)
	})
	return c.path, c.err
}

// InstrumentBridge and InstrumentValidate are the coverage-build instrumentation
// hooks; both are nil outside coverage builds, leaving the embedded script as is.
var InstrumentBridge func(string) (string, error)
var InstrumentValidate func(string) (string, error)

func instrumentOperations(content string) (string, error) {
	if InstrumentBridge == nil {
		return content, nil
	}
	return InstrumentBridge(content)
}

func instrumentValidateScript(content string) (string, error) {
	if InstrumentValidate == nil {
		return content, nil
	}
	return InstrumentValidate(content)
}

var operationsCached = cachedScript{label: "operations", base: "godot_operations", content: operationsScript, instrument: instrumentOperations}
var validateCached = cachedScript{label: "validate", base: "validate_script", content: validateScript, instrument: instrumentValidateScript}
var probeCached = cachedScript{label: "project_probe", base: "project_probe", content: projectProbeScript}

var userCacheDir = os.UserCacheDir

const (
	OperationsHitsEnv = "GDCOVER_FILE"
	ValidateHitsEnv   = "GDCOVER_VALIDATE_FILE"
)

func OperationsScript() (string, error)   { return operationsCached.get() }
func ValidateScript() (string, error)     { return validateCached.get() }
func ProjectProbeScript() (string, error) { return probeCached.get() }

func materialize(name, content string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(content))
	dst := filepath.Join(dir, name+"-"+hex.EncodeToString(sum[:8])+".gd")
	if complete(dst, content) {
		return dst, nil
	}

	if err := fsutil.Write(dst, []byte(content), 0o644); err != nil {
		// On Windows a lost rename race fails instead of overwriting; accept
		// the winner's file when it already landed complete.
		if complete(dst, content) {
			return dst, nil
		}
		return "", fmt.Errorf("install %s script: %w", name, err)
	}
	return dst, nil
}

func complete(path, content string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == int64(len(content))
}

func userToken() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "default"
	}
	sum := sha256.Sum256([]byte(home))
	return hex.EncodeToString(sum[:6])
}

func cacheDir() (string, error) {
	dirs := make([]string, 0, 2)
	if base, err := userCacheDir(); err == nil {
		dirs = append(dirs, filepath.Join(base, "godot-mcp", "scripts"))
	}
	dirs = append(dirs, filepath.Join(os.TempDir(), "godot-mcp-scripts-"+userToken()))

	var lastErr error
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			lastErr = err
			continue
		}
		return dir, nil
	}
	return "", fmt.Errorf("create script cache dir: %w", lastErr)
}
