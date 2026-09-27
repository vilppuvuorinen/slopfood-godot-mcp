package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateProjectSchema ports the reference's tool-definitions test: the
// schema must require projectName and expose the dotnet flag.
func TestCreateProjectSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "create_project", "Create a new Godot project from scratch")
	schema.requireRequired(t, "projectName")
	schema.requireProps(t, "projectName", "dotnet")
	schema.requireType(t, "dotnet", "boolean")
}

// TestCreateProjectTool ports the reference's handleCreateProject behaviors:
// success, existing-project rejection, and validation. The server is bound to
// the target directory because create_project creates the bound root itself.
func TestCreateProjectTool(t *testing.T) {
	t.Parallel()
	t.Run("creates a project", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "game")
		session := connectProject(t, dir)
		text := requireToolText(t, session, "create_project", map[string]any{
			"projectName": "My Game",
		})
		if want := `Project "My Game" created at ` + dir; text != want {
			t.Errorf("message = %q, want %q", text, want)
		}

		content := readFile(t, filepath.Join(dir, "project.godot"))
		requireContains(t, content,
			"config_version=5",
			`config/name="My Game"`,
			`config/features=PackedStringArray("4.4")`,
		)
		requireNotContains(t, content, "[dotnet]")
		if _, err := os.Stat(filepath.Join(dir, "My_Game.csproj")); !os.IsNotExist(err) {
			t.Errorf("plain project unexpectedly wrote a .csproj (err=%v)", err)
		}
	})

	t.Run("existing project rejected", func(t *testing.T) {
		dir := t.TempDir()
		seedProject(t, dir)
		session := connectProject(t, dir)
		requireToolError(t, session, "create_project", map[string]any{
			"projectName": "Existing",
		}, "A project.godot already exists at this path.")
	})

	t.Run("missing projectName", func(t *testing.T) {
		session := connectProject(t, filepath.Join(t.TempDir(), "nope"))
		requireToolError(t, session, "create_project", map[string]any{},
			"projectName is required.")
	})
}

// TestCreateProjectDotnetTool covers the .NET branch and, by extension, the
// reference's dotnet.test.ts helpers (identifier sanitizing, features, csproj).
func TestCreateProjectDotnetTool(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "dotnet")
	session := connectProject(t, dir)

	text := requireToolText(t, session, "create_project", map[string]any{
		"projectName": "My Rpg-Game!",
		"dotnet":      true,
	})
	if want := "(Godot .NET / C#)"; !strings.HasSuffix(text, want) {
		t.Errorf("message = %q, want suffix %q", text, want)
	}

	content := readFile(t, filepath.Join(dir, "project.godot"))
	requireContains(t, content,
		`config/features=PackedStringArray("4.4", "C#")`,
		"[dotnet]",
		`project/assembly_name="My_Rpg_Game_"`,
	)

	csproj := readFile(t, filepath.Join(dir, "My_Rpg_Game_.csproj"))
	requireContains(t, csproj,
		`<Project Sdk="Godot.NET.Sdk/`,
		"<TargetFramework>net8.0</TargetFramework>",
		"<EnableDynamicLoading>true</EnableDynamicLoading>",
		"<Nullable>enable</Nullable>",
		"<RootNamespace>My_Rpg_Game_</RootNamespace>",
	)
}

// TestCreateProjectDotnetIdentifiers ports the toDotnetIdentifier table from
// the reference's dotnet.test.ts through the observable MCP surface.
func TestCreateProjectDotnetIdentifiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		project string
		want    string
	}{
		{name: "invalid chars", project: "My Rpg-Game!", want: "My_Rpg_Game_"},
		{name: "leading digit", project: "2Fast", want: "_2Fast"},
		{name: "all invalid", project: "***", want: "___"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "id")
			session := connectProject(t, dir)
			requireToolText(t, session, "create_project", map[string]any{
				"projectName": tt.project,
				"dotnet":      true,
			})
			content := readFile(t, filepath.Join(dir, "project.godot"))
			requireContains(t, content, `project/assembly_name="`+tt.want+`"`)
			if _, err := os.Stat(filepath.Join(dir, tt.want+".csproj")); err != nil {
				t.Errorf("csproj %s.csproj: %v", tt.want, err)
			}
		})
	}
}
