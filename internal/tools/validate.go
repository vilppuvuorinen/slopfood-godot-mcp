package tools

import "strings"

// validPath reports whether a user-supplied path is safe to use: it must be
// non-empty and must not contain a parent-directory reference. This mirrors the
// reference server's validatePath.
func validPath(path string) bool {
	return path != "" && !strings.Contains(path, "..")
}
