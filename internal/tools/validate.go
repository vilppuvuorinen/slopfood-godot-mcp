package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// validPath reports whether a user-supplied path is safe to use: it must be
// non-empty and must not contain a parent-directory reference. This mirrors the
// reference server's validatePath.
//
// It is a coarse gate for paths the server hands to Godot or cannot otherwise
// resolve itself. Use confinePath for project-relative paths that the server
// resolves against the local filesystem.
func validPath(path string) bool {
	return path != "" && !strings.Contains(path, "..")
}

// godotSchemes are the engine's virtual path roots. Godot resolves them
// relative to the project directory, not the host filesystem, so they are not
// local paths and cannot be confined here.
var godotSchemes = []string{"res://", "user://"}

// resScheme is Godot's project-virtual path prefix.
const resScheme = "res://"

// projectRelativePath rewrites a Godot resource path ("res://...") as a path
// relative to the project root, so a path-valued result field shares the
// project-relative realm of tool arguments. Values outside res:// (for example
// user:// or an absolute host path) are returned unchanged.
func projectRelativePath(path string) string {
	return strings.TrimPrefix(path, resScheme)
}

// confinePath joins sub onto root and reports whether the result stays within
// root, returning the joined, cleaned path on success. It is used for
// project-relative paths that the server itself resolves against the
// filesystem (existence checks, reads, writes).
//
// It rejects empty paths, Godot virtual paths ("res://", "user://"), absolute
// paths, paths that escape root via "..", and paths on a different volume
// (Windows drive-relative and UNC roots). The containment check uses
// filepath.Rel rather than a string prefix so a root such as "/proj" does not
// spuriously match "/project".
//
// The check is lexical and deliberately not a security boundary: symlinks,
// Windows device names, and NTFS alternate data streams are not resolved. The
// external sandbox remains the real line of defense.
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

// confineAbsPath is confinePath for destinations that may legitimately be
// absolute, such as a Godot export output path. Relative paths are confined to
// root with the same guarantees as confinePath. Absolute paths are cleaned and
// returned unchanged, since Godot resolves them against the host filesystem and
// supports absolute destinations. Empty paths, Godot virtual paths ("res://",
// "user://"), and Windows volume-qualified paths that are not fully absolute
// are still rejected.
//
// Like confinePath, the check is lexical and deliberately not a security
// boundary; the external sandbox remains the real line of defense.
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

// classNamePattern matches simple Godot identifiers such as "Node2D" while
// rejecting paths, extensions, and resource URIs.
var classNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validClassName reports whether name is a simple class identifier, mirroring
// the reference server's validateClassName.
func validClassName(name string) bool {
	return classNamePattern.MatchString(name)
}

// pathExists mirrors the reference server's existsSync check: it reports
// whether anything (file or directory) exists at path.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
