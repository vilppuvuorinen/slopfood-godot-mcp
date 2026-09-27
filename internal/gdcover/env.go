package gdcover

import (
	"slices"
	"strings"
)

// KeyEnv returns env with the value of each named variable suffixed with "."
// and key, so that concurrent processes write to distinct hit traces. A named
// variable that is unset or empty is left alone, as is any other variable. The
// returned slice is new; env is not modified.
//
// It pairs with lcov's glob handling: a server keys its GDCOVER_FILE by PID
// here, and the report merges every resulting trace with a glob such as
// hits.txt.*.
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
