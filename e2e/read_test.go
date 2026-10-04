package e2e

import "testing"

// TestReadAcceptsResPaths checks that read forwards res:// paths for the
// resource types: the bridge re-prepends res:// only when absent, so the
// project-relative and res:// forms must resolve to the same resource.
func TestReadAcceptsResPaths(t *testing.T) {
	t.Parallel()

	t.Run("animation", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireScriptSave(t, session, `DirAccess.make_dir_recursive_absolute("res://anims")
var animation := Animation.new()
animation.length = 1.5
return ResourceSaver.save(animation, "res://anims/foo.tres")
`)

		relative := readAnimationPayload(t, session, "anims/foo.tres")
		res := readAnimationPayload(t, session, "res://anims/foo.tres")
		if res.Path != relative.Path || res.Length != relative.Length {
			t.Errorf("res:// payload = %+v, want path %q length %v", res, relative.Path, relative.Length)
		}
	})

	t.Run("tileset", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedAtlasTexture(t, root)
		setupTestTileSet(t, session, "tiles/foo.tres")

		relative := readTilesetPayload(t, session, "tiles/foo.tres")
		res := readTilesetPayload(t, session, "res://tiles/foo.tres")
		if res.Path != relative.Path {
			t.Errorf("res:// path = %q, want %q", res.Path, relative.Path)
		}
	})

	t.Run("theme", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireScriptSave(t, session, `DirAccess.make_dir_recursive_absolute("res://themes")
var theme := Theme.new()
theme.default_font_size = 12
return ResourceSaver.save(theme, "res://themes/foo.tres")
`)

		relative := readThemePayload(t, session, "themes/foo.tres")
		res := readThemePayload(t, session, "res://themes/foo.tres")
		if res.Path != relative.Path {
			t.Errorf("res:// path = %q, want %q", res.Path, relative.Path)
		}
	})

	t.Run("resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		seedMaterialRes(t, session, "resources/foo.res")

		relative := requireToolText(t, session, "read", map[string]any{
			"type": "resource",
			"path": "resources/foo.res",
		})
		res := requireToolText(t, session, "read", map[string]any{
			"type": "resource",
			"path": "res://resources/foo.res",
		})
		if res != relative {
			t.Errorf("res:// payload = %q, want %q", res, relative)
		}
		requireContains(t, res, `"kind": "resource"`, `"class": "StandardMaterial3D"`, `"path": "resources/foo.res"`)
	})
}
