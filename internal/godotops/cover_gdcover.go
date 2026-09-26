//go:build gdcover

package godotops

import "github.com/vilppuvuorinen/slopfood-godot-mcp/internal/gdcover"

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
}
