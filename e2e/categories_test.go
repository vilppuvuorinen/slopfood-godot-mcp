package e2e

import "testing"

// TestEnableCategories exercises the --enable category gating over the real
// stdio surface.
func TestEnableCategories(t *testing.T) {
	t.Run("default registers project only", func(t *testing.T) {
		session := connectSession(t, nil)

		findTool(t, session, "get_project_info")
		requireToolAbsent(t, session, "read_project_settings")
		requireToolAbsent(t, session, "create_scene")
	})

	t.Run("single category replaces the default", func(t *testing.T) {
		session := connectCategories(t, "scene")

		findTool(t, session, "create_scene")
		requireToolAbsent(t, session, "get_project_info")
	})

	t.Run("comma-separated categories", func(t *testing.T) {
		session := connectCategories(t, "project,settings")

		findTool(t, session, "get_project_info")
		findTool(t, session, "read_project_settings")
		requireToolAbsent(t, session, "create_scene")
	})

	t.Run("all registers every implemented category", func(t *testing.T) {
		session := connectCategories(t, "all")

		for _, name := range []string{"get_project_info", "read_project_settings", "create_scene", "add_node", "get_uid"} {
			findTool(t, session, name)
		}
	})
}
