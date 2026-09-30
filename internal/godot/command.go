package godot

import (
	"context"
	"os"
	"os/exec"
)

// A coverage build installs this so each server's engines key their GDScript hit traces by PID.
var engineEnv func([]string) []string

func SetEngineEnv(env func([]string) []string) {
	engineEnv = env
}

func newCommand(ctx context.Context, executable string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
	if engineEnv != nil {
		cmd.Env = engineEnv(os.Environ())
	}
	return cmd
}
