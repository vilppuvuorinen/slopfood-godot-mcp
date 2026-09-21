package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func findTool(t *testing.T, session *mcp.ClientSession, name string) *mcp.Tool {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not listed", name)
	return nil
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func firstText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T", res.Content[0])
	}
	return text.Text
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestCreateProjectSchema ports the reference's tool-definitions test: the
// schema must require projectPath and projectName and expose the dotnet flag.
func TestCreateProjectSchema(t *testing.T) {
	tool := findTool(t, connect(t), "create_project")
	if tool.Description != "Create a new Godot project from scratch" {
		t.Errorf("description = %q", tool.Description)
	}

	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema struct {
		Type       string                    `json:"type"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal schema %s: %v", data, err)
	}
	if schema.Type != "object" {
		t.Errorf("schema type = %q", schema.Type)
	}
	required := strings.Join(schema.Required, ",")
	if required != "projectPath,projectName" {
		t.Errorf("required = %v", schema.Required)
	}
	for _, key := range []string{"projectPath", "projectName", "dotnet"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
	if schema.Properties["dotnet"]["type"] != "boolean" {
		t.Errorf("dotnet type = %v", schema.Properties["dotnet"]["type"])
	}
}

// TestCreateProjectTool ports the reference's handleCreateProject behaviors:
// success, snake_case aliasing, existing-project rejection, and validation.
func TestCreateProjectTool(t *testing.T) {
	session := connect(t)

	t.Run("camelCase", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "game")
		res := callTool(t, session, "create_project", map[string]any{
			"projectPath": dir,
			"projectName": "My Game",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		want := `Project "My Game" created at ` + dir
		if got := firstText(t, res); got != want {
			t.Errorf("message = %q, want %q", got, want)
		}

		content := readFile(t, filepath.Join(dir, "project.godot"))
		for _, want := range []string{
			"config_version=5",
			`config/name="My Game"`,
			`config/features=PackedStringArray("4.4")`,
		} {
			if !strings.Contains(content, want) {
				t.Errorf("project.godot missing %q:\n%s", want, content)
			}
		}
		if strings.Contains(content, "[dotnet]") {
			t.Errorf("plain project unexpectedly has [dotnet]:\n%s", content)
		}
		if _, err := os.Stat(filepath.Join(dir, "My_Game.csproj")); !os.IsNotExist(err) {
			t.Errorf("plain project unexpectedly wrote a .csproj (err=%v)", err)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "aliased")
		res := callTool(t, session, "create_project", map[string]any{
			"project_path": dir,
			"project_name": "Aliased",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		content := readFile(t, filepath.Join(dir, "project.godot"))
		if !strings.Contains(content, `config/name="Aliased"`) {
			t.Errorf("project.godot not created from aliases:\n%s", content)
		}
	})

	t.Run("existing project rejected", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte("config_version=5\n"), 0o600); err != nil {
			t.Fatalf("seed project: %v", err)
		}
		res := callTool(t, session, "create_project", map[string]any{
			"projectPath": dir,
			"projectName": "Existing",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "A project.godot already exists at this path." {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("missing projectName", func(t *testing.T) {
		res := callTool(t, session, "create_project", map[string]any{
			"projectPath": filepath.Join(t.TempDir(), "nope"),
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "projectPath and projectName are required." {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "create_project", map[string]any{
			"projectPath": "../escape",
			"projectName": "Escape",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid path." {
			t.Errorf("message = %q", got)
		}
	})
}

// TestCreateProjectDotnetTool covers the .NET branch and, by extension, the
// reference's dotnet.test.ts helpers (identifier sanitizing, features, csproj).
func TestCreateProjectDotnetTool(t *testing.T) {
	session := connect(t)
	dir := filepath.Join(t.TempDir(), "dotnet")

	res := callTool(t, session, "create_project", map[string]any{
		"projectPath": dir,
		"projectName": "My Rpg-Game!",
		"dotnet":      true,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", firstText(t, res))
	}
	if got := firstText(t, res); !strings.HasSuffix(got, "(Godot .NET / C#)") {
		t.Errorf("message = %q, want a .NET suffix", got)
	}

	content := readFile(t, filepath.Join(dir, "project.godot"))
	for _, want := range []string{
		`config/features=PackedStringArray("4.4", "C#")`,
		"[dotnet]",
		`project/assembly_name="My_Rpg_Game_"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("project.godot missing %q:\n%s", want, content)
		}
	}

	csproj := readFile(t, filepath.Join(dir, "My_Rpg_Game_.csproj"))
	for _, want := range []string{
		`<Project Sdk="Godot.NET.Sdk/`,
		"<TargetFramework>net8.0</TargetFramework>",
		"<EnableDynamicLoading>true</EnableDynamicLoading>",
		"<Nullable>enable</Nullable>",
		"<RootNamespace>My_Rpg_Game_</RootNamespace>",
	} {
		if !strings.Contains(csproj, want) {
			t.Errorf("csproj missing %q:\n%s", want, csproj)
		}
	}
}

// TestCreateProjectDotnetIdentifiers ports the toDotnetIdentifier table from
// the reference's dotnet.test.ts through the observable MCP surface.
func TestCreateProjectDotnetIdentifiers(t *testing.T) {
	session := connect(t)
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
			res := callTool(t, session, "create_project", map[string]any{
				"projectPath": dir,
				"projectName": tt.project,
				"dotnet":      true,
			})
			if res.IsError {
				t.Fatalf("unexpected error: %s", firstText(t, res))
			}
			content := readFile(t, filepath.Join(dir, "project.godot"))
			if !strings.Contains(content, `project/assembly_name="`+tt.want+`"`) {
				t.Errorf("assembly_name = %q, want %q:\n%s", content, tt.want, content)
			}
			if _, err := os.Stat(filepath.Join(dir, tt.want+".csproj")); err != nil {
				t.Errorf("csproj %s.csproj: %v", tt.want, err)
			}
		})
	}
}
