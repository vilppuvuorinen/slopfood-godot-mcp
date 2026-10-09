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

var (
	probeOnce sync.Once
	probePath string
	probeErr  error
)

var userCacheDir = os.UserCacheDir

const (
	OperationsHitsEnv = "GDCOVER_FILE"
	ValidateHitsEnv   = "GDCOVER_VALIDATE_FILE"
)

func OperationsScript() (string, error) {
	once.Do(func() {
		content, instrumentErr := instrumentOperations(operationsScript)
		if instrumentErr != nil {
			err = fmt.Errorf("instrument operations script: %w", instrumentErr)
			return
		}
		path, err = materialize("godot_operations", content)
	})
	return path, err
}

func ValidateScript() (string, error) {
	validateOnce.Do(func() {
		content, instrumentErr := instrumentValidateScript(validateScript)
		if instrumentErr != nil {
			validateErr = fmt.Errorf("instrument validate script: %w", instrumentErr)
			return
		}
		validatePath, validateErr = materialize("validate_script", content)
	})
	return validatePath, validateErr
}

func ProjectProbeScript() (string, error) {
	probeOnce.Do(func() {
		probePath, probeErr = materialize("project_probe", projectProbeScript)
	})
	return probePath, probeErr
}

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
