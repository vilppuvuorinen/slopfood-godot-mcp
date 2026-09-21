package tools

import (
	"regexp"
	"strings"
)

// validPath reports whether a user-supplied path is safe to use: it must be
// non-empty and must not contain a parent-directory reference. This mirrors the
// reference server's validatePath.
func validPath(path string) bool {
	return path != "" && !strings.Contains(path, "..")
}

// classNamePattern matches simple Godot identifiers such as "Node2D" while
// rejecting paths, extensions, and resource URIs.
var classNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// validClassName reports whether name is a simple class identifier, mirroring
// the reference server's validateClassName.
func validClassName(name string) bool {
	return classNamePattern.MatchString(name)
}
