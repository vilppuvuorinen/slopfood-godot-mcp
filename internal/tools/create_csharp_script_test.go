package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dotnetProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "Game.csproj"), `<Project Sdk="Godot.NET.Sdk/4.4.0">
</Project>
`)
	return root
}

func readScript(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestCreateCsharpScriptSchema(t *testing.T) {
	tool := createCSharpScript()
	if tool.Name != "create_csharp_script" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "Create a C# script file in a Godot .NET project" {
		t.Errorf("description = %q", tool.Description)
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("schema type = %v", tool.InputSchema["type"])
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "projectPath" || required[1] != "scriptPath" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T", tool.InputSchema["properties"])
	}
	for _, key := range []string{"projectPath", "scriptPath", "className", "baseClass", "namespaceName", "methods", "source"} {
		if _, present := props[key]; !present {
			t.Errorf("missing property %q", key)
		}
	}
	methods, ok := props["methods"].(map[string]any)
	if !ok || methods["type"] != "array" {
		t.Errorf("methods property = %v", props["methods"])
	}
}

func TestCreateCsharpScriptDefaultSource(t *testing.T) {
	root := dotnetProject(t)
	result := callToolArgs(t, createCSharpScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"scripts/Player.cs"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got, want := result.Content[0].Text, "C# script created at scripts/Player.cs"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	content := readScript(t, filepath.Join(root, "scripts", "Player.cs"))
	want := "using Godot;\n\npublic partial class Player : Node\n{\n\t\n}\n"
	if content != want {
		t.Errorf("content = %q, want %q", content, want)
	}
}

func TestCreateCsharpScriptNamespaceAndBaseClass(t *testing.T) {
	root := dotnetProject(t)
	result := callToolArgs(t, createCSharpScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"Player.cs","baseClass":"Node2D","namespaceName":"My Game.Core"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content := readScript(t, filepath.Join(root, "Player.cs"))
	want := "using Godot;\n\nnamespace My_Game.Core;\n\n\tpublic partial class Player : Node2D\n\t{\n\t\t\n\t}\n"
	if content != want {
		t.Errorf("content = %q, want %q", content, want)
	}
}

func TestCreateCsharpScriptMethods(t *testing.T) {
	root := dotnetProject(t)
	result := callToolArgs(t, createCSharpScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"Player.cs","methods":["_Ready","_Process","_PhysicsProcess","_Input","_UnhandledInput","_EnterTree","_ExitTree","_Ready","Custom Thing",""]}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content := readScript(t, filepath.Join(root, "Player.cs"))
	for _, want := range []string{
		"public override void _Ready()",
		"public override void _Process(double delta)",
		"public override void _PhysicsProcess(double delta)",
		"public override void _Input(InputEvent @event)",
		"public override void _UnhandledInput(InputEvent @event)",
		"public override void _EnterTree()",
		"public override void _ExitTree()",
		"public void Custom_Thing()",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
	if got := strings.Count(content, "public override void _Ready()"); got != 1 {
		t.Errorf("_Ready appears %d times, want 1", got)
	}
}

func TestCreateCsharpScriptCustomSource(t *testing.T) {
	root := dotnetProject(t)
	result := callToolArgs(t, createCSharpScript(), `{"projectPath":`+jsonString(root)+`,"scriptPath":"Player.cs","source":"// custom\n"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := readScript(t, filepath.Join(root, "Player.cs")); got != "// custom\n" {
		t.Errorf("content = %q", got)
	}
}

func TestCreateCsharpScriptAliases(t *testing.T) {
	root := dotnetProject(t)
	result := callToolArgs(t, createCSharpScript(), `{"project_path":`+jsonString(root)+`,"script_path":"Player.cs","class_name":"Player","base_class":"Node2D","namespace_name":"Game"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	content := readScript(t, filepath.Join(root, "Player.cs"))
	if !strings.Contains(content, "namespace Game;") {
		t.Errorf("missing namespace in:\n%s", content)
	}
	if !strings.Contains(content, "public partial class Player : Node2D") {
		t.Errorf("missing class in:\n%s", content)
	}
}

func TestCreateCsharpScriptErrors(t *testing.T) {
	project := t.TempDir()
	makeProject(t, project)
	empty := t.TempDir()
	dotnet := dotnetProject(t)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing args", args: `{}`, want: "projectPath and scriptPath are required."},
		{name: "missing scriptPath", args: `{"projectPath":` + jsonString(project) + `}`, want: "projectPath and scriptPath are required."},
		{name: "invalid path", args: `{"projectPath":"../escape","scriptPath":"Player.cs"}`, want: "Invalid path."},
		{name: "invalid script path", args: `{"projectPath":` + jsonString(dotnet) + `,"scriptPath":"../Player.cs"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(empty) + `,"scriptPath":"Player.cs"}`, want: "Not a valid Godot project: " + empty},
		{name: "not a dotnet project", args: `{"projectPath":` + jsonString(project) + `,"scriptPath":"Player.cs"}`, want: "Not a Godot .NET project (no .csproj found). Use create_project with dotnet: true first."},
		{name: "bad extension", args: `{"projectPath":` + jsonString(dotnet) + `,"scriptPath":"Player.gd"}`, want: "scriptPath must end with .cs"},
		{name: "invalid class name", args: `{"projectPath":` + jsonString(dotnet) + `,"scriptPath":"1Player.cs"}`, want: `Invalid C# script file name "1Player.cs": the name must be a valid class name (letters, digits, underscore; not starting with a digit), because Godot requires the class name to match the file name.`},
		{name: "className mismatch", args: `{"projectPath":` + jsonString(dotnet) + `,"scriptPath":"Player.cs","className":"Other"}`, want: `className "Other" must match the script file name "Player" for Godot to attach the script.`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, createCSharpScript(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if got := result.Content[0].Text; got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}
