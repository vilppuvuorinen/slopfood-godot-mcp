package e2e

import "testing"

// TestEnableCategories exercises the --enable category gating over the real
// stdio surface.
func TestEnableCategories(t *testing.T) {
	t.Run("default registers every category", func(t *testing.T) {
		session := connectSession(t, nil)

		for _, name := range []string{
			"get_godot_version", // project
			"validate_project",  // project
			"export_project",    // export
			"create_scene",      // scene
			"add_node",          // node
			"add_scene_signal",  // signals
			"set_tilemap_cell",  // tilemap
			"attach_script",     // script
			"create_resource",   // assets
			"create_animation",  // animation
			"create_tileset",    // tileset
			"create_theme",      // theme
		} {
			findTool(t, session, name)
		}
	})

	t.Run("single category replaces the default", func(t *testing.T) {
		session := connectCategories(t, "scene")

		findTool(t, session, "create_scene")
		requireToolAbsent(t, session, "get_godot_version")
		requireToolAbsent(t, session, "attach_script")
	})

	t.Run("comma-separated categories", func(t *testing.T) {
		session := connectCategories(t, "project,node")

		findTool(t, session, "get_godot_version")
		findTool(t, session, "validate_project")
		findTool(t, session, "add_node")
		requireToolAbsent(t, session, "create_scene")
	})

	t.Run("animation, tileset, and theme are independent", func(t *testing.T) {
		animation := connectCategories(t, "animation")
		findTool(t, animation, "create_animation")
		requireToolAbsent(t, animation, "create_tileset")
		requireToolAbsent(t, animation, "create_theme")

		tileset := connectCategories(t, "tileset")
		findTool(t, tileset, "create_tileset")
		requireToolAbsent(t, tileset, "create_animation")

		theme := connectCategories(t, "theme")
		findTool(t, theme, "create_theme")
		requireToolAbsent(t, theme, "create_animation")
	})

	t.Run("all registers every category", func(t *testing.T) {
		session := connectCategories(t, "all")

		for _, name := range []string{
			"get_godot_version", // project
			"validate_project",  // project
			"export_project",    // export
			"create_scene",      // scene
			"add_node",          // node
			"add_scene_signal",  // signals
			"set_tilemap_cell",  // tilemap
			"attach_script",     // script
			"create_resource",   // assets
			"create_animation",  // animation
			"create_tileset",    // tileset
			"create_theme",      // theme
		} {
			findTool(t, session, name)
		}
	})
}
