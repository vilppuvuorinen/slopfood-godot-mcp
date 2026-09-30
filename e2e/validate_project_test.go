package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

type validateProjectPayload struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

func TestValidateProjectTool(t *testing.T) {
	t.Parallel()
	t.Run("valid project", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application]\n\nconfig/name=\"Demo\"\n")
		session := connectProject(t, root)

		text := requireToolText(t, session, "validate_project", map[string]any{})
		var payload validateProjectPayload
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if !payload.Valid {
			t.Errorf("valid = false, errors = %v", payload.Errors)
		}
		if len(payload.Errors) != 0 {
			t.Errorf("errors = %v, want none", payload.Errors)
		}
	})

	t.Run("malformed project", func(t *testing.T) {
		root := t.TempDir()
		writeE2EFile(t, filepath.Join(root, "project.godot"), "config_version=5\n\n[application\n\nconfig/name=\"Demo\"\n")
		session := connectProject(t, root)

		res := callTool(t, session, "validate_project", map[string]any{})
		if res.IsError {
			t.Fatalf("expected structured invalid, got an error: %s", firstText(t, res))
		}
		var payload validateProjectPayload
		if err := json.Unmarshal([]byte(firstText(t, res)), &payload); err != nil {
			t.Fatalf("unmarshal %q: %v", firstText(t, res), err)
		}
		if payload.Valid {
			t.Error("valid = true, want false")
		}
		if len(payload.Errors) == 0 {
			t.Error("errors is empty, want the engine's parse diagnostic")
		}
		requireContains(t, payload.Errors[0], "Error parsing", "project.godot")
	})

	t.Run("engine failure", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		session := connectProjectWithEnv(t, root, "GODOT_PATH="+filepath.Join(t.TempDir(), "missing-godot"))

		res := callTool(t, session, "validate_project", map[string]any{})
		if !res.IsError {
			t.Fatalf("expected an error, got: %s", firstText(t, res))
		}
		requirePrefix(t, firstText(t, res), "Failed to validate project:")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		session := connectProject(t, root)
		requireToolError(t, session, "validate_project", map[string]any{},
			"Not a valid Godot project: "+root)
	})
}
