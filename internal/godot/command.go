package godot

import (
	"context"
	"os"
	"os/exec"
)

// engineEnv, when non-nil, rewrites the environment every spawned engine process
// inherits. A coverage build installs it so each server's engines write their
// GDScript hit traces to files keyed by that server's process, letting the e2e
// suite run in parallel without engines racing on one trace.
var engineEnv func([]string) []string

// SetEngineEnv installs env, which is applied to the environment of every engine
// process this package starts. Passing nil restores plain inheritance of the
// server's environment. It is used by the gdcover coverage build.
func SetEngineEnv(env func([]string) []string) {
	engineEnv = env
}

// newCommand builds an exec.Cmd for an engine invocation and applies the engine
// environment hook, if any. Without the hook the child inherits the server's
// environment, as before.
func newCommand(ctx context.Context, executable string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...) //nolint:gosec // executable is operator-controlled via GODOT_PATH.
	if engineEnv != nil {
		cmd.Env = engineEnv(os.Environ())
	}
	return cmd
}
