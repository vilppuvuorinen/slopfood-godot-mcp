//go:build gdcover

package godotops

import (
	"os"
	"strconv"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/gdcover"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

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

func suffixHitsWithPID(env []string) []string {
	return gdcover.KeyEnv(env, strconv.Itoa(os.Getpid()), OperationsHitsEnv, ValidateHitsEnv)
}
