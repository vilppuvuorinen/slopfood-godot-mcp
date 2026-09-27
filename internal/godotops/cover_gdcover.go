//go:build gdcover

package godotops

import (
	"os"
	"strconv"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/gdcover"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

// Linking the instrumenter is opt-in so the default server binary stays
// dependency-free. `make e2e-cover` builds with `-tags gdcover`. Both embedded
// scripts are instrumented, each against its own hit-output variable so their
// traces can be told apart.
func init() {
	instrumentBridge = func(src string) (string, error) {
		result, err := gdcover.InstrumentEnv([]byte(src), OperationsHitsEnv)
		if err != nil {
			return "", err
		}
		return string(result.Source), nil
	}
	instrumentValidate = func(src string) (string, error) {
		result, err := gdcover.InstrumentEnv([]byte(src), ValidateHitsEnv)
		if err != nil {
			return "", err
		}
		return string(result.Source), nil
	}
	godot.SetEngineEnv(suffixHitsWithPID)
}

// suffixHitsWithPID appends this server's PID to each hit-trace path in the
// engine environment, so every server's engines write their own trace and
// concurrent servers never race appending to one file. `make e2e-cover` merges
// the per-server traces by globbing the paths.
func suffixHitsWithPID(env []string) []string {
	return gdcover.KeyEnv(env, strconv.Itoa(os.Getpid()), OperationsHitsEnv, ValidateHitsEnv)
}
