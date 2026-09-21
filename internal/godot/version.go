package godot

import (
	"regexp"
	"strconv"
	"strings"
)

// versionPattern captures the major and minor components of a Godot version.
var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)`)

// UIDCapable reports whether a Godot version string is 4.4 or later, when
// resource UIDs were introduced. Unparseable versions are not UID-capable,
// matching the reference's isGodot44OrLater.
func UIDCapable(version string) bool {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return false
	}
	return major > 4 || (major == 4 && minor >= 4)
}
