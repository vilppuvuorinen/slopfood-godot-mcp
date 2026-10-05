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

// res:// anchors at the project directory; user:// points at the per-user data
// directory, which is outside the project root.
const (
	resScheme  = "res://"
	userScheme = "user://"
)

// pathError reports why a caller-supplied path was rejected. The reason and
// hints are surfaced through the tool result so callers can correct the path.
type pathError struct {
	reason string
	hints  []string
}

func (e *pathError) Error() string { return e.reason }

func invalidPathError() *pathError {
	return &pathError{
		reason: "Invalid path.",
		hints:  []string{`Provide a valid path without ".." or other potentially unsafe characters`},
	}
}

func userSchemeError() *pathError {
	return &pathError{
		reason: "user:// paths are not supported.",
		hints: []string{
			"user:// resolves to the user data directory, outside the project root",
			"Use a project-relative path, an absolute path, or res:// instead",
		},
	}
}

func absolutePathError() *pathError {
	return &pathError{
		reason: "Absolute paths are not allowed.",
		hints:  []string{"Use a project-relative path or res:// (which anchors to the project root)"},
	}
}

func escapesRootError() *pathError {
	return &pathError{
		reason: "Path escapes the project root.",
		hints:  []string{"Provide a path inside the project root"},
	}
}

// Rewrites a res:// path as project-relative; other paths are returned unchanged.
func projectRelativePath(path string) string {
	return strings.TrimPrefix(path, resScheme)
}

// The check is lexical and deliberately not a security boundary; the external
// sandbox remains the real line of defense. res:// is accepted and anchored at
// root; user:// is rejected because it names a directory outside root.
func confinePath(root, sub string) (string, error) {
	if root == "" || sub == "" {
		return "", invalidPathError()
	}
	if strings.HasPrefix(sub, userScheme) {
		return "", userSchemeError()
	}
	cleaned := filepath.Clean(projectRelativePath(sub))
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return "", absolutePathError()
	}
	full := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(filepath.Clean(root), full)
	if err != nil {
		return "", invalidPathError()
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", escapesRootError()
	}
	return full, nil
}

// Like confinePath but permits legitimate absolute destinations such as a Godot
// export output path; the check is still lexical, not a security boundary.
func confineAbsPath(root, sub string) (string, error) {
	if root == "" || sub == "" {
		return "", invalidPathError()
	}
	if strings.HasPrefix(sub, userScheme) {
		return "", userSchemeError()
	}
	cleaned := filepath.Clean(projectRelativePath(sub))
	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return cleaned, nil
	}
	return confinePath(root, cleaned)
}

var classNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validClassName(name string) bool {
	return classNamePattern.MatchString(name)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
