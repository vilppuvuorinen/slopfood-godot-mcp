package e2e

import (
	"fmt"
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
		if payload.Length != 3 || payload.LoopMode != "ping_pong" {
			t.Fatalf("payload = %+v, want length 3 ping_pong", payload)
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
		if payload.TileSet != "tiles/set_scripted.tres" || len(payload.Cells) != 2 {
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
		if len(payload.Cells) != 1 {
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
		if len(payload.Cells) != 0 {
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

		text := requireToolText(t, session, "read", map[string]any{
			"type": "resource",
			"path": "materials/scripted.res",
		})
		requireContains(t, text, `"class": "StandardMaterial3D"`, `"metallic": 0.5`, `"$type": "Color"`)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var material: StandardMaterial3D = load("res://materials/scripted.res")
	material.metallic = 0.75
	return ResourceSaver.save(material, "res://materials/scripted.res")
`)

		text = requireToolText(t, session, "read", map[string]any{
			"type": "resource",
			"path": "materials/scripted.res",
		})
		requireContains(t, text, `"metallic": 0.75`)
	})
}

func readSceneTree(t *testing.T, session *mcp.ClientSession, scenePath string) sceneNode {
	t.Helper()
	text := requireToolText(t, session, "read", map[string]any{"type": "scene", "path": scenePath})
	return unmarshalSceneEnvelope(t, text).Data.Root
}

func childByName(node sceneNode, name string) *sceneNode {
	for i := range node.Children {
		if node.Children[i].Name == name {
			return &node.Children[i]
		}
	}
	return nil
}

// TestRunScriptSceneOperations smoke tests the load-mutate-pack-save GDScript
// that the removed create_scene, add_node, modify_scene_node,
// remove_scene_node, rename_scene_node, duplicate_scene_node and move_scene_node
// tools used to wrap. The scripts run in the live editor session and the result
// is read back with read(type=scene), so a bridge change that breaks editor-mode
// scene mutation fails here.
func TestRunScriptSceneOperations(t *testing.T) {
	t.Parallel()

	t.Run("creates scenes with custom roots", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)

		for _, tc := range []struct{ path, rootType string }{
			{"levels/arena.tscn", "Node3D"},
			{"world.tscn", "Node2D"},
		} {
			requireScriptSave(t, session, fmt.Sprintf(`extends RefCounted

func run():
	var root = ClassDB.instantiate("%s")
	root.name = "root"
	var dir := "res://%s".get_base_dir()
	if not dir.is_empty():
		DirAccess.make_dir_recursive_absolute(dir)
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://%s")
`, tc.rootType, tc.path, tc.path))

			requireContains(t, readFile(t, filepath.Join(root, tc.path)), `type="`+tc.rootType+`"`)
			tree := readSceneTree(t, session, tc.path)
			if tree.Name != "root" || tree.Type != tc.rootType {
				t.Errorf("%s root = %+v, want root of type %s", tc.path, tree, tc.rootType)
			}
		}
	})

	t.Run("adds typed nodes, nested children, and scene instances", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		createScene(t, session, "scenes/level.tscn", "Node2D")
		createScene(t, session, "scenes/enemy.tscn", "Sprite2D")

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/level.tscn")
	var root: Node = scene.instantiate()
	var player = ClassDB.instantiate("Sprite2D")
	player.name = "Player"
	root.add_child(player)
	player.owner = root
	var weapon = ClassDB.instantiate("Node2D")
	weapon.name = "Weapon"
	player.add_child(weapon)
	weapon.owner = root
	var foe = load("res://scenes/enemy.tscn").instantiate()
	foe.name = "Foe"
	root.add_child(foe)
	foe.owner = root
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/level.tscn")
`)

		tree := readSceneTree(t, session, "scenes/level.tscn")
		player := childByName(tree, "Player")
		if player == nil || player.Type != "Sprite2D" {
			t.Fatalf("children = %+v, want a Sprite2D Player", tree.Children)
		}
		if weapon := childByName(*player, "Weapon"); weapon == nil || weapon.Type != "Node2D" {
			t.Errorf("Player children = %+v, want a Node2D Weapon", player.Children)
		}
		if foe := childByName(tree, "Foe"); foe == nil || foe.Type != "Sprite2D" {
			t.Errorf("children = %+v, want an instanced Sprite2D Foe", tree.Children)
		}
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "level.tscn")),
			`[ext_resource type="PackedScene"`, `instance=ExtResource(`)
	})

	t.Run("modifies typed node properties", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		createScene(t, session, "scenes/props.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/props.tscn", "", "Player", "Sprite2D", nil)
		writeE2EFile(t, filepath.Join(root, "art", "icon.tres"),
			"[gd_resource type=\"GradientTexture2D\" format=3]\n\n[resource]\nwidth = 16\nheight = 16\n")

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/props.tscn")
	var root: Node = scene.instantiate()
	var player: Sprite2D = root.get_node("Player")
	player.position = Vector2(10, 20)
	player.modulate = Color(0, 1, 0, 1)
	player.visible = false
	player.z_index = 3
	player.editor_description = "hero"
	player.texture = load("res://art/icon.tres")
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/props.tscn")
`)

		requireContains(t, readFile(t, filepath.Join(root, "scenes", "props.tscn")),
			"position = Vector2(10, 20)", "visible = false", "z_index = 3", `editor_description = "hero"`)

		player := childByName(readSceneTree(t, session, "scenes/props.tscn"), "Player")
		if player == nil {
			t.Fatal("Player missing from the scene tree")
		}
		if position, _ := player.Properties["position"].(map[string]any); position["$type"] != "Vector2" || position["x"] != float64(10) || position["y"] != float64(20) {
			t.Errorf("position = %v, want Vector2(10, 20)", player.Properties["position"])
		}
		if modulate, _ := player.Properties["modulate"].(map[string]any); modulate["$type"] != "Color" || modulate["g"] != float64(1) {
			t.Errorf("modulate = %v, want a green Color", player.Properties["modulate"])
		}
		if player.Properties["visible"] != false || player.Properties["z_index"] != float64(3) {
			t.Errorf("visible/z_index = %v/%v, want false/3", player.Properties["visible"], player.Properties["z_index"])
		}
		if player.Properties["editor_description"] != "hero" {
			t.Errorf("editor_description = %v, want hero", player.Properties["editor_description"])
		}
		if texture, _ := player.Properties["texture"].(map[string]any); texture["$type"] != "Resource" || texture["path"] != "res://art/icon.tres" {
			t.Errorf("texture = %v, want the icon resource", player.Properties["texture"])
		}
	})

	t.Run("removes a node", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		createScene(t, session, "scenes/remove.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/remove.tscn", "", "Keep", "Node2D", nil)
		addNodeViaScript(t, session, "scenes/remove.tscn", "", "Drop", "Sprite2D", nil)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/remove.tscn")
	var root: Node = scene.instantiate()
	var drop: Node = root.get_node("Drop")
	root.remove_child(drop)
	drop.free()
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/remove.tscn")
`)

		tree := readSceneTree(t, session, "scenes/remove.tscn")
		if childByName(tree, "Drop") != nil || childByName(tree, "Keep") == nil {
			t.Errorf("children = %+v, want Keep retained and Drop removed", tree.Children)
		}
		requireNotContains(t, readFile(t, filepath.Join(root, "scenes", "remove.tscn")), `name="Drop"`)
	})

	t.Run("renames a node", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		createScene(t, session, "scenes/rename.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/rename.tscn", "", "Player", "Sprite2D", nil)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/rename.tscn")
	var root: Node = scene.instantiate()
	var player: Node = root.get_node("Player")
	player.name = "Hero"
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/rename.tscn")
`)

		tree := readSceneTree(t, session, "scenes/rename.tscn")
		if hero := childByName(tree, "Hero"); hero == nil || hero.Type != "Sprite2D" {
			t.Errorf("children = %+v, want a renamed Sprite2D Hero", tree.Children)
		}
		if childByName(tree, "Player") != nil {
			t.Errorf("children = %+v, want no Player after rename", tree.Children)
		}
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "rename.tscn")), `[node name="Hero"`)
	})

	t.Run("duplicates a subtree with ownership", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		createScene(t, session, "scenes/dup.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/dup.tscn", "", "Parent", "Node2D", nil)
		addNodeViaScript(t, session, "scenes/dup.tscn", "Parent", "Child", "Sprite2D", nil)

		// The duplicate's children must be re-owned to the scene root or pack() drops them.
		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/dup.tscn")
	var root: Node = scene.instantiate()
	var parent: Node = root.get_node("Parent")
	var copy: Node = parent.duplicate()
	copy.name = "Parent2"
	root.add_child(copy)
	_own(copy, root)
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/dup.tscn")

func _own(node: Node, owner: Node) -> void:
	node.owner = owner
	for child in node.get_children():
		_own(child, owner)
`)

		copy := childByName(readSceneTree(t, session, "scenes/dup.tscn"), "Parent2")
		if copy == nil || copy.Type != "Node2D" {
			t.Fatalf("children = %+v, want a duplicated Node2D Parent2", copy)
		}
		if childByName(*copy, "Child") == nil {
			t.Errorf("Parent2 children = %+v, want the duplicated Child", copy.Children)
		}
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "dup.tscn")),
			`[node name="Parent2"`, `parent="Parent2"`)
	})

	t.Run("moves a node under a new parent", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		createScene(t, session, "scenes/move.tscn", "Node2D")
		addNodeViaScript(t, session, "scenes/move.tscn", "", "Player", "Node2D", nil)
		addNodeViaScript(t, session, "scenes/move.tscn", "", "World", "Node2D", nil)

		requireScriptSave(t, session, `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/move.tscn")
	var root: Node = scene.instantiate()
	var player: Node = root.get_node("Player")
	var world: Node = root.get_node("World")
	player.get_parent().remove_child(player)
	player.owner = null
	world.add_child(player)
	player.owner = root
	var packed := PackedScene.new()
	var err := packed.pack(root)
	if err != OK:
		return err
	return ResourceSaver.save(packed, "res://scenes/move.tscn")
`)

		world := childByName(readSceneTree(t, session, "scenes/move.tscn"), "World")
		if world == nil || childByName(*world, "Player") == nil {
			t.Fatalf("World = %+v, want Player reparented under it", world)
		}
		requireContains(t, readFile(t, filepath.Join(root, "scenes", "move.tscn")), `parent="World"`)
	})

	t.Run("reports a structure error from the editor session", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		createScene(t, session, "scenes/fail.tscn", "Node2D")

		doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"script": `extends RefCounted

func run():
	var scene: PackedScene = load("res://scenes/fail.tscn")
	var root: Node = scene.instantiate()
	var missing: Node = root.get_node("Missing")
	missing.name = "Nope"
	return 0
`}))
		if !doc.Compiled {
			t.Fatalf("compiled = false, errors = %+v", doc.Errors)
		}
		if len(doc.Errors) == 0 {
			t.Fatal("errors = none, want a runtime error for the missing node")
		}
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
	text := requireToolText(t, session, "read", map[string]any{"type": "scene", "path": scenePath})
	return unmarshalSceneEnvelope(t, text).Data.Connections
}

// TestRunScriptSceneSignals smoke tests the signal connections the removed
// add_scene_signal/remove_scene_signal tools used to wrap. The scripts edit the
// scene through the live editor session and the result is read back from the
// file and with read(type=scene), so a bridge change that breaks editor-mode scene
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
