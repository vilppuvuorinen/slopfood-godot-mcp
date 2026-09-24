package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func callToolArgs(t *testing.T, tool mcp.Tool, args string) mcp.ToolResult {
	t.Helper()
	var raw json.RawMessage
	if args != "" {
		raw = json.RawMessage(args)
	}
	out, err := tool.Handler(context.Background(), raw)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return result
}

func decodeProjects(t *testing.T, text string) []godotProject {
	t.Helper()
	var projects []godotProject
	if err := json.Unmarshal([]byte(text), &projects); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return projects
}

func TestListProjectsSchema(t *testing.T) {
	tool := listProjects()
	if tool.Name != "list_projects" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "List Godot projects in a directory" {
		t.Errorf("description = %q", tool.Description)
	}
	if tool.InputSchema["type"] != "object" {
		t.Errorf("schema type = %v", tool.InputSchema["type"])
	}
	if tool.OutputSchema["type"] != "object" {
		t.Errorf("output schema type = %v", tool.OutputSchema["type"])
	}
	outProps, ok := tool.OutputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("output properties = %T", tool.OutputSchema["properties"])
	}
	projects, ok := outProps["projects"].(map[string]any)
	if !ok || projects["type"] != "array" {
		t.Errorf("projects property = %v", outProps["projects"])
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "directory" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	recursive, ok := props["recursive"].(map[string]any)
	if !ok || recursive["type"] != "boolean" {
		t.Errorf("recursive property = %v", props["recursive"])
	}
	if _, ok := props["directory"].(map[string]any); !ok {
		t.Errorf("directory property = %v", props["directory"])
	}
}

func TestListProjectsNonRecursive(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	makeProject(t, filepath.Join(root, "alpha"))
	makeProject(t, filepath.Join(root, "nested", "gamma"))

	result := callToolArgs(t, listProjects(), `{"directory":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}

	projects := decodeProjects(t, result.Content[0].Text)
	want := []string{root, filepath.Join(root, "alpha")}
	if paths := projectPaths(projects); !equalStrings(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestListProjectsRecursive(t *testing.T) {
	root := t.TempDir()
	makeProject(t, filepath.Join(root, "nested", "gamma"))
	makeProject(t, filepath.Join(root, "nested", "deeper", "delta"))
	makeProject(t, filepath.Join(root, ".hidden", "secret"))

	result := callToolArgs(t, listProjects(), `{"directory":`+jsonString(root)+`,"recursive":true}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	projects := decodeProjects(t, result.Content[0].Text)
	want := []string{
		filepath.Join(root, "nested", "deeper", "delta"),
		filepath.Join(root, "nested", "gamma"),
	}
	if paths := projectPaths(projects); !equalStrings(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestListProjectsEmpty(t *testing.T) {
	result := callToolArgs(t, listProjects(), `{"directory":`+jsonString(t.TempDir())+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != "[]" {
		t.Fatalf("text = %q, want []", got)
	}
}

func TestListProjectsMissingDirectory(t *testing.T) {
	result := callToolArgs(t, listProjects(), `{}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %+v", result.Content)
	}
	if result.Content[0].Text != "Directory is required" {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if !strings.HasPrefix(result.Content[1].Text, "Possible solutions:") {
		t.Errorf("solutions = %q", result.Content[1].Text)
	}
}

func TestListProjectsInvalidPath(t *testing.T) {
	result := callToolArgs(t, listProjects(), `{"directory":"../escape"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if result.Content[0].Text != "Invalid directory path" {
		t.Errorf("message = %q", result.Content[0].Text)
	}
}

func TestListProjectsMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	result := callToolArgs(t, listProjects(), `{"directory":`+jsonString(missing)+`}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	want := "Directory does not exist: " + missing
	if result.Content[0].Text != want {
		t.Errorf("message = %q, want %q", result.Content[0].Text, want)
	}
}

func TestListProjectsStructuredContent(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, listProjects(), `{"directory":`+jsonString(root)+`}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	output, ok := result.StructuredContent.(listProjectsOutput)
	if !ok {
		t.Fatalf("structuredContent type %T, want listProjectsOutput", result.StructuredContent)
	}
	if paths := projectPaths(output.Projects); !equalStrings(paths, []string{root}) {
		t.Fatalf("paths = %v, want [%s]", paths, root)
	}
}

// jsonString quotes a string for embedding in a JSON test argument.
func jsonString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(data)
}
