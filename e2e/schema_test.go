package e2e

import "testing"

type toolSchemaCase struct {
	name        string
	description string
	check       func(t *testing.T, schema toolSchema)
}

func TestToolSchemas(t *testing.T) {
	t.Parallel()
	tools := listToolsOnce(t, connect(t))

	cases := []toolSchemaCase{
		{"add_node", "Add a built-in node or a scene instance to an existing scene (rewrites the .tscn file)", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodeName")
			schema.requireProps(t, "scenePath", "parentNodePath", "nodeType", "instancePath", "nodeName", "properties")
			schema.requireType(t, "properties", "object")
		}},
		{"attach_script", "Attach a GDScript to a scene node (headless)", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath", "scriptPath")
			schema.requireStringProps(t, "scenePath", "nodePath", "scriptPath")
			schema.requirePropCount(t, 3)
		}},
		{"clear_tilemap", "Clear every cell on a TileMapLayer node in a scene", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requirePropCount(t, 2)
		}},
		{"create_animation", "Create an Animation resource", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath", "loopMode")
			schema.requireType(t, "length", "number")
			schema.requireType(t, "step", "number")
			schema.requirePropCount(t, 4)
		}},
		{"create_project", "Create a new Godot project from scratch", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "projectName")
			schema.requireProps(t, "projectName", "dotnet")
			schema.requireType(t, "dotnet", "boolean")
		}},
		{"create_resource", "Create a .tres/.res resource file", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath", "resourceType")
			schema.requireStringProps(t, "resourcePath", "resourceType")
			schema.requireType(t, "properties", "object")
			schema.requirePropCount(t, 3)
		}},
		{"create_scene", "Create a new Godot scene file", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath")
			schema.requireStringProps(t, "scenePath", "rootNodeType")
		}},
		{"create_theme", "Create a Theme resource", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath")
			schema.requireType(t, "defaultFontSize", "integer")
			schema.requireType(t, "defaultBaseScale", "number")
			schema.requirePropCount(t, 4)
		}},
		{"duplicate_scene_node", "Duplicate a node within a .tscn scene", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requirePropCount(t, 2)
		}},
		{"erase_tilemap_cell", "Erase a tile from a TileMapLayer node in a scene", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath", "coords")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requirePropCount(t, 3)
		}},
		{"export_mesh_library", "Export a scene as a MeshLibrary resource", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "outputPath")
			schema.requireType(t, "meshItemNames", "array")

			items, ok := schema.Properties["meshItemNames"]["items"].(map[string]any)
			if !ok || items["type"] != "string" {
				t.Errorf("meshItemNames items = %v", schema.Properties["meshItemNames"]["items"])
			}
		}},
		{"export_project", "Export a Godot project using a preset", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "presetName", "outputPath")
			schema.requirePropCount(t, 3)
			schema.requireStringProps(t, "presetName", "outputPath")
			schema.requireType(t, "debug", "boolean")
		}},
		{"modify_animation", "Set an Animation resource's length, loop mode, or step", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath", "loopMode")
			schema.requireType(t, "length", "number")
			schema.requireType(t, "step", "number")
			schema.requirePropCount(t, 4)
		}},
		{"modify_resource", "Set properties on a .tres/.res resource file", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath", "properties")
			schema.requireStringProps(t, "resourcePath")
			schema.requireType(t, "properties", "object")
			schema.requirePropCount(t, 2)
		}},
		{"modify_scene_node", "Modify node properties in a scene file (headless)", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath", "properties")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requireType(t, "properties", "object")
			schema.requirePropCount(t, 3)
		}},
		{"modify_theme", "Set a Theme's default font, font size, or base scale", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath")
			schema.requireType(t, "defaultFontSize", "integer")
			schema.requireType(t, "defaultBaseScale", "number")
			schema.requirePropCount(t, 4)
		}},
		{"move_scene_node", "Move a node to a new parent in a .tscn scene", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath", "newParentPath")
			schema.requireStringProps(t, "scenePath", "nodePath", "newParentPath")
			schema.requirePropCount(t, 3)
		}},
		{"read_animation", "Read Animation resources and their tracks", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath")
			schema.requirePropCount(t, 1)
		}},
		{"read_binary_resource", "Read engine-decoded binary .res resource files (text .tres files are read directly)", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath")
			schema.requireType(t, "propertyFilter", "array")
			schema.requirePropCount(t, 2)
		}},
		{"read_scene", "Read scene file as JSON node tree, including signal connections (headless)", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requireType(t, "depth", "integer")
			schema.requirePropCount(t, 3)
		}},
		{"read_theme", "Read Theme resources, their types, variations, and items", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath")
			schema.requirePropCount(t, 1)
		}},
		{"read_tilemap", "Read the cells of a TileMapLayer node in a scene", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requirePropCount(t, 2)
		}},
		{"read_tileset", "Read TileSet resources and their sources and tiles", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "resourcePath")
			schema.requireStringProps(t, "resourcePath")
			schema.requirePropCount(t, 1)
		}},
		{"reimport_project", "Reimport a Godot project by scanning it in the editor, generating .import sidecars and missing resource UIDs", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t)
			schema.requirePropCount(t, 0)
		}},
		{"remove_scene_node", "Remove a node from a scene file (headless)", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requirePropCount(t, 2)
		}},
		{"rename_scene_node", "Rename a node in a .tscn scene", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath", "newName")
			schema.requireStringProps(t, "scenePath", "nodePath", "newName")
			schema.requirePropCount(t, 3)
		}},
		{"run_script", "Run a GDScript in the live editor session and return its output", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "script")
			schema.requireStringProps(t, "script")
			schema.requirePropCount(t, 1)
		}},
		{"set_tilemap_cell", "Place a tile on a TileMapLayer node in a scene", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath", "coords", "sourceId", "atlasCoords")
			schema.requireStringProps(t, "scenePath", "nodePath")
			schema.requireType(t, "sourceId", "integer")
			schema.requireType(t, "alternativeTile", "integer")
			schema.requirePropCount(t, 6)
		}},
		{"validate_project", "Load project.godot with the Godot engine and report whether it parsed", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t)
			schema.requirePropCount(t, 0)
		}},
		{"validate_scripts", "Validate every GDScript file in the project", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t)
			schema.requirePropCount(t, 0)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema := requireSchema(t, tools, tc.name, tc.description)
			tc.check(t, schema)
		})
	}
}
