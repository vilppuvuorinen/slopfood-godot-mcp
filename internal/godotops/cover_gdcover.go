//go:build gdcover

package godotops

import (
	"os"
	"strconv"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/gdcover"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func init() {
	InstrumentBridge = func(src string) (string, error) {
		result, err := gdcover.InstrumentEnv([]byte(src), OperationsHitsEnv)
		if err != nil {
			return "", err
		}
		return string(result.Source), nil
	}
	InstrumentValidate = func(src string) (string, error) {
		result, err := gdcover.InstrumentEnv([]byte(src), ValidateHitsEnv)
		if err != nil {
			return "", err
		}
		return string(result.Source), nil
	}
	godot.EngineEnv = suffixHitsWithPID
}

func suffixHitsWithPID(env []string) []string {
	return gdcover.KeyEnv(env, strconv.Itoa(os.Getpid()), OperationsHitsEnv, ValidateHitsEnv)
}
