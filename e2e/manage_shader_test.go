package e2e

import (
	"path/filepath"
	"testing"
)

// TestManageShaderSchema ports the reference's tool-definition expectations for
// manage_shader over the real stdio surface.
func TestManageShaderSchema(t *testing.T) {
	schema := requireSchema(t, "manage_shader", "Create or read .gdshader files")
	schema.requireRequired(t, "projectPath", "shaderPath", "action")
	schema.requirePropCount(t, 5)
	schema.requireStringProps(t, "projectPath", "shaderPath", "action", "shaderType", "source")
}

// TestManageShaderTool drives the real manage_shader tool without Godot.
func TestManageShaderTool(t *testing.T) {
	session := connect(t)

	t.Run("creates with the default template", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		text := requireToolText(t, session, "manage_shader", map[string]any{
			"projectPath": root,
			"shaderPath":  "shaders/effect.gdshader",
			"action":      "create",
		})
		if want := "Shader created at shaders/effect.gdshader"; text != want {
			t.Errorf("message = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "shaders", "effect.gdshader"))
		want := "shader_type spatial;\n\nvoid fragment() {\n\t// Called for every pixel the material is visible on.\n}\n"
		if content != want {
			t.Errorf("content = %q, want %q", content, want)
		}
	})

	t.Run("creates with an explicit shader type", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		requireToolText(t, session, "manage_shader", map[string]any{
			"projectPath": root,
			"shaderPath":  "ui.gdshader",
			"action":      "create",
			"shaderType":  "canvas_item",
		})
		content := readFile(t, filepath.Join(root, "ui.gdshader"))
		want := "shader_type canvas_item;\n\nvoid fragment() {\n\t// Called for every pixel the material is visible on.\n}\n"
		if content != want {
			t.Errorf("content = %q, want %q", content, want)
		}
	})

	t.Run("writes a custom source verbatim", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		source := "shader_type spatial;\n\nvoid vertex() {\n\tVERTEX.y += 1.0;\n}\n"
		requireToolText(t, session, "manage_shader", map[string]any{
			"projectPath": root,
			"shaderPath":  "custom.gdshader",
			"action":      "create",
			"source":      source,
		})
		content := readFile(t, filepath.Join(root, "custom.gdshader"))
		if content != source {
			t.Errorf("content = %q, want %q", content, source)
		}
	})

	t.Run("reads the shader back", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		source := "shader_type sky;\n"
		writeE2EFile(t, filepath.Join(root, "read.gdshader"), source)

		text := requireToolText(t, session, "manage_shader", map[string]any{
			"projectPath": root,
			"shaderPath":  "read.gdshader",
			"action":      "read",
		})
		if text != source {
			t.Errorf("text = %q, want %q", text, source)
		}
	})
}

// TestManageShaderErrors covers the reference's validation messages.
func TestManageShaderErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, session, "manage_shader", map[string]any{},
			"projectPath, shaderPath, and action are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "manage_shader", map[string]any{
			"projectPath": "../escape",
			"shaderPath":  "a.gdshader",
			"action":      "read",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		requireToolError(t, session, "manage_shader", map[string]any{
			"projectPath": root,
			"shaderPath":  "a.gdshader",
			"action":      "read",
		}, "Not a valid Godot project: "+root)
	})

	t.Run("read missing shader", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_shader", map[string]any{
			"projectPath": root,
			"shaderPath":  "missing.gdshader",
			"action":      "read",
		}, "Shader not found: missing.gdshader")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		requireToolError(t, session, "manage_shader", map[string]any{
			"projectPath": root,
			"shaderPath":  "a.gdshader",
			"action":      "bogus",
		}, "Unknown action: bogus")
	})
}
