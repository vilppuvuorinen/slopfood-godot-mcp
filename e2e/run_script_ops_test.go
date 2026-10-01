package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// requireScriptSave runs a fixture script that returns a ResourceSaver error
// code and fails unless the script compiled cleanly and the save returned OK.
func requireScriptSave(t *testing.T, session *mcp.ClientSession, source string) {
	t.Helper()
	doc := requireRunScript(t, session, source)
	if code, ok := doc.Result.(float64); !ok || code != 0 {
		t.Fatalf("script save failed: result = %v, want 0 (errors = %+v)", doc.Result, doc.Errors)
	}
}

func themeTypeByName(payload testThemePayload, name string) map[string]any {
	for _, themeType := range payload.Types {
		if themeType["name"] == name {
			return themeType
		}
	}
	return nil
}

// TestRunScriptEditorOperations smoke tests the editor-mode mutations that the
// removed mutation tools used to wrap. The scripts run in the live editor
// session and the results are read back with the fixed read tools, so a bridge
// change that breaks editor-mode mutation fails here.
func TestRunScriptEditorOperations(t *testing.T) {
	t.Parallel()

	t.Run("tileset source, tile, alternative, and tile data", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedAtlasTexture(t, root)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var tile_set := TileSet.new()
	tile_set.tile_size = Vector2i(16, 16)
	var source := TileSetAtlasSource.new()
	source.texture = load("res://tiles/atlas.tres")
	source.texture_region_size = Vector2i(16, 16)
	tile_set.add_source(source, 0)
	source.create_tile(Vector2i(0, 0))
	source.create_alternative_tile(Vector2i(0, 0))
	var tile_data := source.get_tile_data(Vector2i(0, 0), 0)
	tile_data.z_index = 3
	return ResourceSaver.save(tile_set, "res://tiles/scripted.tres")
`)

		payload := readTilesetPayload(t, session, "tiles/scripted.tres")
		if len(payload.Sources) != 1 || payload.Sources[0]["type"] != "atlas" {
			t.Fatalf("sources = %v, want one atlas source", payload.Sources)
		}
		if payload.Sources[0]["texture"] != "tiles/atlas.tres" {
			t.Errorf("texture = %v", payload.Sources[0]["texture"])
		}
		alternatives := tileAlternatives(t, payload)
		if len(alternatives) != 2 {
			t.Fatalf("alternatives = %v, want two", alternatives)
		}
		data, _ := alternatives[0]["data"].(map[string]any)
		if data["z_index"] != float64(3) {
			t.Errorf("tile data = %v, want z_index 3", data)
		}
	})

	t.Run("animation value track", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		requireScriptSave(t, session, `extends RefCounted

func run():
	DirAccess.make_dir_recursive_absolute("res://anims")
	var animation := Animation.new()
	animation.length = 2.0
	var track := animation.add_track(Animation.TYPE_VALUE)
	animation.track_set_path(track, NodePath("Sprite2D:position"))
	animation.track_set_interpolation_type(track, Animation.INTERPOLATION_CUBIC)
	animation.track_insert_key(track, 0.0, Vector2(10, 20))
	return ResourceSaver.save(animation, "res://anims/scripted.tres")
`)

		payload := readAnimationPayload(t, session, "anims/scripted.tres")
		if payload.Length != 2 || len(payload.Tracks) != 1 {
			t.Fatalf("payload = %+v, want length 2 with one track", payload)
		}
		track := payload.Tracks[0]
		if track["type"] != "value" || track["path"] != "Sprite2D:position" || track["interpolation"] != "cubic" {
			t.Errorf("track = %v", track)
		}
		keys, _ := track["keys"].([]any)
		if len(keys) != 1 {
			t.Fatalf("keys = %v, want one", keys)
		}
		key, _ := keys[0].(map[string]any)
		value, _ := key["value"].(map[string]any)
		if value["$type"] != "Vector2" || value["x"] != float64(10) || value["y"] != float64(20) {
			t.Errorf("key value = %v, want Vector2(10, 20)", key["value"])
		}
	})

	t.Run("animation create and modify properties", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		requireScriptSave(t, session, `extends RefCounted

func run():
	DirAccess.make_dir_recursive_absolute("res://anims")
	var animation := Animation.new()
	animation.length = 2.0
	animation.loop_mode = Animation.LOOP_LINEAR
	animation.step = 0.25
	return ResourceSaver.save(animation, "res://anims/props.tres")
`)

		payload := readAnimationPayload(t, session, "anims/props.tres")
		if payload.Length != 2 || payload.LoopMode != "linear" || payload.Step != 0.25 {
			t.Fatalf("payload = %+v, want length 2 linear step 0.25", payload)
		}

		requireScriptSave(t, session, `extends RefCounted

func run():
	var animation: Animation = load("res://anims/props.tres")
	animation.length = 3.0
	animation.loop_mode = Animation.LOOP_PINGPONG
	return ResourceSaver.save(animation, "res://anims/props.tres")
`)

		payload = readAnimationPayload(t, session, "anims/props.tres")
		if payload.Length != 3 || payload.LoopMode != "pingpong" {
			t.Fatalf("payload = %+v, want length 3 pingpong", payload)
		}
	})

	t.Run("theme item and type variation", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		requireScriptSave(t, session, `extends RefCounted

func run():
	DirAccess.make_dir_recursive_absolute("res://themes")
	var theme := Theme.new()
	theme.set_theme_item(Theme.DATA_TYPE_COLOR, "font_color", "Label", Color(1, 0, 0, 1))
	theme.set_theme_item(Theme.DATA_TYPE_CONSTANT, "outline_size", "Label", 4)
	theme.set_type_variation("CustomButton", "Button")
	return ResourceSaver.save(theme, "res://themes/scripted.tres")
`)

		payload := readThemePayload(t, session, "themes/scripted.tres")
		label := themeTypeByName(payload, "Label")
		if label == nil {
			t.Fatalf("types = %v, want a Label type", payload.Types)
		}
		colors, _ := label["colors"].(map[string]any)
		color, _ := colors["font_color"].(map[string]any)
		if color["$type"] != "Color" || color["r"] != float64(1) {
			t.Errorf("font_color = %v, want a tagged red Color", colors["font_color"])
		}
		constants, _ := label["constants"].(map[string]any)
		if constants["outline_size"] != float64(4) {
			t.Errorf("outline_size = %v, want 4", constants["outline_size"])
		}
		custom := themeTypeByName(payload, "CustomButton")
		if custom == nil || custom["variationBase"] != "Button" {
			t.Errorf("types = %v, want CustomButton based on Button", payload.Types)
		}
	})

	t.Run("theme create and modify defaults", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedThemeResources(t, session, "themes/smoke_")

		requireScriptSave(t, session, `extends RefCounted

func run():
	var theme := Theme.new()
	theme.default_font = load("res://themes/smoke_font.tres")
	theme.default_font_size = 16
	theme.default_base_scale = 1.5
	return ResourceSaver.save(theme, "res://themes/defaults.tres")
`)

		payload := readThemePayload(t, session, "themes/defaults.tres")
		if payload.DefaultFontSize == nil || *payload.DefaultFontSize != 16 {
			t.Fatalf("defaultFontSize = %v, want 16", payload.DefaultFontSize)
		}
		if payload.DefaultBaseScale == nil || *payload.DefaultBaseScale != 1.5 {
			t.Fatalf("defaultBaseScale = %v, want 1.5", payload.DefaultBaseScale)
		}
		if payload.DefaultFont["$type"] != "Resource" || payload.DefaultFont["path"] != "res://themes/smoke_font.tres" {
			t.Fatalf("defaultFont = %v", payload.DefaultFont)
		}

		requireScriptSave(t, session, `extends RefCounted

func run():
	var theme: Theme = load("res://themes/defaults.tres")
	theme.default_font_size = 20
	return ResourceSaver.save(theme, "res://themes/defaults.tres")
`)

		payload = readThemePayload(t, session, "themes/defaults.tres")
		if payload.DefaultFontSize == nil || *payload.DefaultFontSize != 20 {
			t.Fatalf("defaultFontSize = %v, want 20", payload.DefaultFontSize)
		}
	})

	t.Run("tilemap set, erase, and clear cells", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session, "tiles/set_scripted.tres")
		setupTilemapScene(t, session, "scenes/tilemap_scripted.tscn", "tiles/set_scripted.tres")

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/tilemap_scripted.tscn")
	var root: Node = scene.instantiate()
	var ground: TileMapLayer = root.get_node("Ground")
	ground.set_cell(Vector2i(1, 2), 0, Vector2i(0, 0))
	ground.set_cell(Vector2i(0, 0), 0, Vector2i(0, 0))
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/tilemap_scripted.tscn")
`)

		payload := readTilemapPayload(t, session, "scenes/tilemap_scripted.tscn")
		if payload.TileSet != "tiles/set_scripted.tres" || len(payload.UsedCells) != 2 {
			t.Fatalf("payload = %+v, want tileset and two cells", payload)
		}

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/tilemap_scripted.tscn")
	var root: Node = scene.instantiate()
	var ground: TileMapLayer = root.get_node("Ground")
	ground.erase_cell(Vector2i(1, 2))
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/tilemap_scripted.tscn")
`)

		payload = readTilemapPayload(t, session, "scenes/tilemap_scripted.tscn")
		if len(payload.UsedCells) != 1 {
			t.Fatalf("payload = %+v, want one cell after erase", payload)
		}

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/tilemap_scripted.tscn")
	var root: Node = scene.instantiate()
	var ground: TileMapLayer = root.get_node("Ground")
	ground.clear()
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/tilemap_scripted.tscn")
`)

		payload = readTilemapPayload(t, session, "scenes/tilemap_scripted.tscn")
		if len(payload.UsedCells) != 0 {
			t.Fatalf("payload = %+v, want no cells after clear", payload)
		}
	})

	t.Run("resource create and modify properties", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var material := StandardMaterial3D.new()
	material.metallic = 0.5
	material.albedo_color = Color(1, 0, 0, 1)
	DirAccess.make_dir_recursive_absolute("res://materials")
	return ResourceSaver.save(material, "res://materials/scripted.res")
`)

		text := requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath": "materials/scripted.res",
		})
		requireContains(t, text, `"type": "StandardMaterial3D"`, `"metallic": 0.5`, `"$type": "Color"`)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var material: StandardMaterial3D = load("res://materials/scripted.res")
	material.metallic = 0.75
	return ResourceSaver.save(material, "res://materials/scripted.res")
`)

		text = requireToolText(t, session, "read_binary_resource", map[string]any{
			"resourcePath": "materials/scripted.res",
		})
		requireContains(t, text, `"metallic": 0.75`)
	})
}

type testSceneConnection struct {
	Signal string `json:"signal"`
	From   string `json:"from"`
	To     string `json:"to"`
	Method string `json:"method"`
}

func readSceneConnections(t *testing.T, session *mcp.ClientSession, scenePath string) []testSceneConnection {
	t.Helper()
	text := requireToolText(t, session, "read_scene", map[string]any{"scenePath": scenePath})
	var payload struct {
		Connections []testSceneConnection `json:"connections"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode read_scene: %v\n%s", err, text)
	}
	return payload.Connections
}

// TestRunScriptSceneSignals smoke tests the signal connections the removed
// add_scene_signal/remove_scene_signal tools used to wrap. The scripts edit the
// scene through the live editor session and the result is read back from the
// file and with read_scene, so a bridge change that breaks editor-mode scene
// mutation fails here.
func TestRunScriptSceneSignals(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)
	createScene(t, session, "scenes/signals.tscn", "Button")
	sceneFile := filepath.Join(root, "scenes", "signals.tscn")
	connection := `[connection signal="pressed" from="." to="." method="on_pressed"]`

	requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/signals.tscn")
	var root: Node = scene.instantiate()
	root.pressed.connect(Callable(root, "on_pressed"), CONNECT_PERSIST)
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/signals.tscn")
`)
	requireContains(t, readFile(t, sceneFile), connection)
	if got := readSceneConnections(t, session, "scenes/signals.tscn"); len(got) != 1 || got[0].Signal != "pressed" || got[0].Method != "on_pressed" {
		t.Errorf("connections = %+v, want one pressed -> on_pressed", got)
	}

	requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/signals.tscn")
	var root: Node = scene.instantiate()
	root.disconnect("pressed", Callable(root, "on_pressed"))
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/signals.tscn")
`)
	requireNotContains(t, readFile(t, sceneFile), connection)
	if got := readSceneConnections(t, session, "scenes/signals.tscn"); len(got) != 0 {
		t.Errorf("connections = %+v, want none", got)
	}
}
