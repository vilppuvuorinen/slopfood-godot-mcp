//go:build gdcover

package godot

import (
	"os"
	"os/exec"
)

// A coverage build installs this so each server's engines key their GDScript hit traces by PID.
var engineEnv func([]string) []string

func SetEngineEnv(env func([]string) []string) { engineEnv = env }

func applyEngineEnv(cmd *exec.Cmd) {
	if engineEnv != nil {
		cmd.Env = engineEnv(os.Environ())
	}
}
