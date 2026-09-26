//go:build gdcover

package godotops

import "github.com/vilppuvuorinen/go-dot-mcp/internal/gdcover"

// Linking the instrumenter is opt-in so the default server binary stays
// dependency-free. `make e2e-cover` builds with `-tags gdcover`.
func init() {
	instrumentBridge = func(src string) (string, error) {
		result, err := gdcover.Instrument([]byte(src))
		if err != nil {
			return "", err
		}
		return string(result.Source), nil
	}
}
