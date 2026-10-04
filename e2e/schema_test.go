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
		{"attach_script", "Attach a GDScript to a scene node (headless)", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "scenePath", "nodePath", "scriptPath")
			schema.requireStringProps(t, "scenePath", "nodePath", "scriptPath")
			schema.requirePropCount(t, 3)
		}},
		{"create_project", "Create a new Godot project from scratch", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "projectName")
			schema.requireProps(t, "projectName", "dotnet")
			schema.requireType(t, "dotnet", "boolean")
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
		{"read", "Read a scene, animation, tileset, theme, or binary resource file; use read_node to read a node subtree", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "type", "path")
			schema.requireStringProps(t, "type", "path")
			schema.requireType(t, "depth", "integer")
			schema.requireType(t, "propertyFilter", "array")
			schema.requirePropCount(t, 4)

			enum, ok := schema.Properties["type"]["enum"].([]any)
			if !ok {
				t.Fatalf("type enum = %v", schema.Properties["type"]["enum"])
			}
			want := []string{"scene", "animation", "tileset", "theme", "resource"}
			if len(enum) != len(want) {
				t.Fatalf("type enum = %v, want %v", enum, want)
			}
			for i, value := range want {
				if enum[i] != value {
					t.Errorf("type enum[%d] = %v, want %q", i, enum[i], value)
				}
			}
		}},
		{"read_node", "Read a node subtree from a scene, or the cells of a TileMapLayer node; use read to read whole files", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "type", "path", "nodePath")
			schema.requireStringProps(t, "type", "path", "nodePath")
			schema.requireType(t, "depth", "integer")
			schema.requirePropCount(t, 4)

			enum, ok := schema.Properties["type"]["enum"].([]any)
			if !ok {
				t.Fatalf("type enum = %v", schema.Properties["type"]["enum"])
			}
			want := []string{"scene", "tilemap"}
			if len(enum) != len(want) {
				t.Fatalf("type enum = %v, want %v", enum, want)
			}
			for i, value := range want {
				if enum[i] != value {
					t.Errorf("type enum[%d] = %v, want %q", i, enum[i], value)
				}
			}
		}},
		{"rescan_project", "Rescan a Godot project in the editor, generating .import sidecars and missing resource UIDs", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t)
			schema.requirePropCount(t, 0)
		}},
		{"run_script", "Run a GDScript function body in the live editor session and return its output", func(t *testing.T, schema toolSchema) {
			schema.requireRequired(t, "body")
			schema.requireStringProps(t, "body")
			schema.requirePropCount(t, 1)
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
