package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

func seedDotnetProject(t *testing.T, dir string) {
	t.Helper()
	seedProject(t, dir)
	writeE2EFile(t, filepath.Join(dir, "Game.csproj"), `<Project Sdk="Godot.NET.Sdk/4.4.0">
</Project>
`)
}

func TestCreateCsharpScriptSchema(t *testing.T) {
	schema := requireSchema(t, "create_csharp_script", "Create a C# script file in a Godot .NET project")
	schema.requireRequired(t, "projectPath", "scriptPath")
	schema.requirePropCount(t, 7)
	schema.requireStringProps(t, "projectPath", "scriptPath", "className", "baseClass", "namespaceName", "source")
	schema.requireType(t, "methods", "array")
}

func TestCreateCsharpScriptTool(t *testing.T) {
	session := connect(t)

	t.Run("default source", func(t *testing.T) {
		root := t.TempDir()
		seedDotnetProject(t, root)

		text := requireToolText(t, session, "create_csharp_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "scripts/Player.cs",
		})
		if want := "C# script created at scripts/Player.cs"; text != want {
			t.Errorf("text = %q, want %q", text, want)
		}
		content := readFile(t, filepath.Join(root, "scripts", "Player.cs"))
		want := "using Godot;\n\npublic partial class Player : Node\n{\n\t\n}\n"
		if content != want {
			t.Errorf("content = %q, want %q", content, want)
		}
	})

	t.Run("methods and namespace", func(t *testing.T) {
		root := t.TempDir()
		seedDotnetProject(t, root)

		requireToolText(t, session, "create_csharp_script", map[string]any{
			"projectPath":   root,
			"scriptPath":    "Player.cs",
			"baseClass":     "Node2D",
			"namespaceName": "MyGame",
			"methods":       []string{"_Ready", "_Process"},
		})
		content := readFile(t, filepath.Join(root, "Player.cs"))
		requireContains(t, content,
			"using Godot;",
			"namespace MyGame;",
			"public partial class Player : Node2D",
			"public override void _Ready()",
			"public override void _Process(double delta)",
		)
	})

	t.Run("custom source", func(t *testing.T) {
		root := t.TempDir()
		seedDotnetProject(t, root)

		requireToolText(t, session, "create_csharp_script", map[string]any{
			"projectPath": root,
			"scriptPath":  "Player.cs",
			"source":      "// custom\n",
		})
		if got := readFile(t, filepath.Join(root, "Player.cs")); got != "// custom\n" {
			t.Errorf("content = %q", got)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		root := t.TempDir()
		seedDotnetProject(t, root)

		requireToolText(t, session, "create_csharp_script", map[string]any{
			"project_path":   root,
			"script_path":    "Player.cs",
			"class_name":     "Player",
			"base_class":     "Node2D",
			"namespace_name": "Game",
		})
		content := readFile(t, filepath.Join(root, "Player.cs"))
		requireContains(t, content, "namespace Game;", "public partial class Player : Node2D")
	})
}

func TestCreateCsharpScriptErrors(t *testing.T) {
	session := connect(t)

	plain := t.TempDir()
	seedProject(t, plain)
	dotnet := t.TempDir()
	seedDotnetProject(t, dotnet)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "missing scriptPath",
			args: map[string]any{"projectPath": plain},
			want: "projectPath and scriptPath are required.",
		},
		{
			name: "invalid path",
			args: map[string]any{"projectPath": "../escape", "scriptPath": "Player.cs"},
			want: "Invalid path.",
		},
		{
			name: "not a project",
			args: map[string]any{"projectPath": t.TempDir(), "scriptPath": "Player.cs"},
			want: "Not a valid Godot project",
		},
		{
			name: "not a dotnet project",
			args: map[string]any{"projectPath": plain, "scriptPath": "Player.cs"},
			want: "Not a Godot .NET project (no .csproj found). Use create_project with dotnet: true first.",
		},
		{
			name: "bad extension",
			args: map[string]any{"projectPath": dotnet, "scriptPath": "Player.gd"},
			want: "scriptPath must end with .cs",
		},
		{
			name: "invalid class name",
			args: map[string]any{"projectPath": dotnet, "scriptPath": "1Player.cs"},
			want: `Invalid C# script file name "1Player.cs": the name must be a valid class name (letters, digits, underscore; not starting with a digit), because Godot requires the class name to match the file name.`,
		},
		{
			name: "className mismatch",
			args: map[string]any{"projectPath": dotnet, "scriptPath": "Player.cs", "className": "Other"},
			want: `className "Other" must match the script file name "Player" for Godot to attach the script.`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := callTool(t, session, "create_csharp_script", tt.args)
			if !res.IsError {
				t.Fatalf("expected an error result: %s", firstText(t, res))
			}
			if got := firstText(t, res); !strings.HasPrefix(got, tt.want) {
				t.Errorf("message = %q, want prefix %q", got, tt.want)
			}
		})
	}
}
