package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func validPath(path string) bool {
	return path != "" && !strings.Contains(path, "..")
}

// Godot resolves these virtual roots against the project directory, not the
// host filesystem.
var godotSchemes = []string{"res://", "user://"}

const resScheme = "res://"

// Rewrites a res:// path as project-relative; other paths are returned unchanged.
func projectRelativePath(path string) string {
	return strings.TrimPrefix(path, resScheme)
}

// The check is lexical and deliberately not a security boundary; the external
// sandbox remains the real line of defense.
func confinePath(root, sub string) (string, bool) {
	if root == "" || sub == "" {
		return "", false
	}
	for _, scheme := range godotSchemes {
		if strings.HasPrefix(sub, scheme) {
			return "", false
		}
	}
	cleaned := filepath.Clean(sub)
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", false
	}
	full := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(filepath.Clean(root), full)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

// Like confinePath but permits legitimate absolute destinations such as a Godot
// export output path; the check is still lexical, not a security boundary.
func confineAbsPath(root, sub string) (string, bool) {
	if root == "" || sub == "" {
		return "", false
	}
	for _, scheme := range godotSchemes {
		if strings.HasPrefix(sub, scheme) {
			return "", false
		}
	}
	cleaned := filepath.Clean(sub)
	if filepath.IsAbs(cleaned) {
		return cleaned, true
	}
	return confinePath(root, sub)
}

var classNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validClassName(name string) bool {
	return classNamePattern.MatchString(name)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
