package e2e

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSettingsListMatchesReadBack proves the specialized list actions and
// read_project_settings share one value contract: the engine-free list payload
// must equal the setting the real Godot engine reports for the same file. The
// list calls themselves never touch Godot; only the read-back probe is gated.
func TestSettingsListMatchesReadBack(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	writeE2EFile(t, filepath.Join(root, "game.gd"), "extends Node\n")
	writeE2EFile(t, filepath.Join(root, "project.godot"), `config_version=5

[autoload]

Game="*res://game.gd"

[input]

move_left={"deadzone": 0.5}

[internationalization]

translations=PackedStringArray("res://en.translation", "res://fi.translation")
`)

	sections := requireProjectSettingsRoundTrip(t, session, root)

	t.Run("autoloads", func(t *testing.T) {
		var list map[string]any
		text := requireToolText(t, session, "manage_autoloads", map[string]any{
			"projectPath": root,
			"action":      "list",
		})
		if err := json.Unmarshal([]byte(text), &list); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if !reflect.DeepEqual(list["Game"], sections["autoload"]["Game"]) {
			t.Errorf("autoload Game list = %#v, read = %#v", list["Game"], sections["autoload"]["Game"])
		}
	})

	t.Run("input map", func(t *testing.T) {
		var list map[string]any
		text := requireToolText(t, session, "manage_input_map", map[string]any{
			"projectPath": root,
			"action":      "list",
		})
		if err := json.Unmarshal([]byte(text), &list); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if !reflect.DeepEqual(list["move_left"], sections["input"]["move_left"]) {
			t.Errorf("input move_left list = %#v, read = %#v", list["move_left"], sections["input"]["move_left"])
		}
	})

	t.Run("translations", func(t *testing.T) {
		var list map[string]any
		text := requireToolText(t, session, "manage_translations", map[string]any{
			"projectPath": root,
			"action":      "list",
		})
		if err := json.Unmarshal([]byte(text), &list); err != nil {
			t.Fatalf("unmarshal %q: %v", text, err)
		}
		if !reflect.DeepEqual(list["translations"], sections["internationalization"]["translations"]) {
			t.Errorf("translations list = %#v, read = %#v", list["translations"], sections["internationalization"]["translations"])
		}
	})
}
