//go:build !gdcover

package godot

import "os/exec"

// applyEngineEnv is a no-op outside coverage builds.
func applyEngineEnv(*exec.Cmd) {}
