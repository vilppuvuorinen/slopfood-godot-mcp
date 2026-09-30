package gdcover

import (
	"slices"
	"strings"
)

// KeyEnv suffixes each named variable's value with "." + key; empty or unset
// values are left alone and env is not modified.
func KeyEnv(env []string, key string, names ...string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && value != "" && slices.Contains(names, name) {
			entry = name + "=" + value + "." + key
		}
		out = append(out, entry)
	}
	return out
}
