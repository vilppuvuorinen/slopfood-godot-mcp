package e2e

import "testing"

// TestEnableCategories exercises the --enable category gating over the real
// stdio surface.
func TestEnableCategories(t *testing.T) {
	t.Run("default registers project only", func(t *testing.T) {
		session := connectSession(t, nil)

		findTool(t, session, "get_project_info")
		for _, name := range []string{
			"read_project_settings", "manage_export_presets", "create_scene",
			"add_node", "create_script", "create_resource",
		} {
			requireToolAbsent(t, session, name)
		}
	})

	t.Run("single category replaces the default", func(t *testing.T) {
		session := connectCategories(t, "scene")

		findTool(t, session, "create_scene")
		requireToolAbsent(t, session, "get_project_info")
		requireToolAbsent(t, session, "create_script")
	})

	t.Run("comma-separated categories", func(t *testing.T) {
		session := connectCategories(t, "project,settings")

		findTool(t, session, "get_project_info")
		findTool(t, session, "read_project_settings")
		requireToolAbsent(t, session, "create_scene")
	})

	t.Run("all registers every category", func(t *testing.T) {
		session := connectCategories(t, "all")

		for _, name := range []string{
			"get_project_info",      // project
			"read_project_settings", // settings
			"manage_export_presets", // export
			"create_scene",          // scene
			"add_node",              // node
			"create_script",         // script
			"create_resource",       // resources
		} {
			findTool(t, session, name)
		}
	})
}
