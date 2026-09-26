package e2e

import "testing"

// TestEnableCategories exercises the --enable category gating over the real
// stdio surface.
func TestEnableCategories(t *testing.T) {
	t.Run("default registers every category", func(t *testing.T) {
		session := connectSession(t, nil)

		for _, name := range []string{
			"get_project_info",      // project
			"validate_project",      // project
			"manage_export_presets", // export
			"create_scene",          // scene
			"add_node",              // node
			"add_scene_signal",      // signals
			"set_tilemap_cell",      // tilemap
			"attach_script",         // script
			"manage_resource",       // assets
			"read_animation",        // media
		} {
			findTool(t, session, name)
		}
	})

	t.Run("single category replaces the default", func(t *testing.T) {
		session := connectCategories(t, "scene")

		findTool(t, session, "create_scene")
		requireToolAbsent(t, session, "get_project_info")
		requireToolAbsent(t, session, "attach_script")
	})

	t.Run("comma-separated categories", func(t *testing.T) {
		session := connectCategories(t, "project,node")

		findTool(t, session, "get_project_info")
		findTool(t, session, "validate_project")
		findTool(t, session, "add_node")
		requireToolAbsent(t, session, "create_scene")
	})

	t.Run("all registers every category", func(t *testing.T) {
		session := connectCategories(t, "all")

		for _, name := range []string{
			"get_project_info",      // project
			"validate_project",      // project
			"manage_export_presets", // export
			"create_scene",          // scene
			"add_node",              // node
			"add_scene_signal",      // signals
			"set_tilemap_cell",      // tilemap
			"attach_script",         // script
			"manage_resource",       // assets
			"read_animation",        // media
		} {
			findTool(t, session, name)
		}
	})
}
