package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// versionSession is an Engine fake that reports a fixed version, so feature-tag
// derivation can be exercised without a real engine.
type versionSession struct {
	fakeSession
	version string
	err     error
}

func (s *versionSession) Version(context.Context) (string, error) {
	return s.version, s.err
}

func callHandler(t *testing.T, tool mcp.Tool, args string) mcp.ToolResult {
	t.Helper()
	out, err := tool.Handler(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	result, ok := out.(mcp.ToolResult)
	if !ok {
		t.Fatalf("result type %T, want mcp.ToolResult", out)
	}
	return result
}

func readProjectGodot(t *testing.T, dir string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	return string(content)
}

func TestEscapeGodotString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "My Game", want: "My Game"},
		{in: `say "hi"`, want: `say \"hi\"`},
		{in: `back\slash`, want: `back\\slash`},
		{in: `a\b"c`, want: `a\\b\"c`},
	}
	for _, tt := range tests {
		if got := escapeGodotString(tt.in); got != tt.want {
			t.Errorf("escapeGodotString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidProjectName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "My Game", want: true},
		{name: `Evil" quote`, want: true},
		{name: `back\slash`, want: true},
		{name: "", want: false},
		{name: "   ", want: false},
		{name: "line\nbreak", want: false},
		{name: "tab\tname", want: false},
		{name: "nul\x00name", want: false},
	}
	for _, tt := range tests {
		if got := validProjectName(tt.name); got != tt.want {
			t.Errorf("validProjectName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCreateProjectEscapesName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "game")
	result := callToolArgs(t, (*toolset).createProject, dir, `{"projectName":"Evil\" config/name=\"pwned"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}

	content, err := os.ReadFile(filepath.Join(dir, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	got := string(content)
	want := `config/name="Evil\" config/name=\"pwned"` + "\n"
	if !strings.Contains(got, want) {
		t.Errorf("project.godot = %q, want it to contain %q", got, want)
	}
}

func TestCreateProjectRejectsControlChars(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "game")
	result := callToolArgs(t, (*toolset).createProject, dir, `{"projectName":"Evil\nconfig/name=\"pwned"}`)
	if !result.IsError {
		t.Fatalf("expected an error result: %+v", result.Content)
	}
	if result.Content[0].Text != "Invalid project name." {
		t.Errorf("message = %q", result.Content[0].Text)
	}
	if _, err := os.Stat(filepath.Join(dir, "project.godot")); !os.IsNotExist(err) {
		t.Errorf("project.godot written despite invalid name (err=%v)", err)
	}
}

func TestCreateProjectDerivesFeatureTag(t *testing.T) {
	tests := []struct {
		name    string
		version string
		err     error
		want    string
	}{
		{name: "stable", version: "4.7.2.stable.official.abc", want: `config/features=PackedStringArray("4.7")`},
		{name: "minor only", version: "4.8.stable", want: `config/features=PackedStringArray("4.8")`},
		{name: "unparseable falls back", version: "unknown", want: `config/features=PackedStringArray("` + defaultGodotVersion + `")`},
		{name: "error falls back", err: errors.New("no engine"), want: `config/features=PackedStringArray("` + defaultGodotVersion + `")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "game")
			ts := newToolset(dir, &versionSession{version: tt.version, err: tt.err})
			result := callHandler(t, ts.createProject(), `{"projectName":"Demo"}`)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			if got := readProjectGodot(t, dir); !strings.Contains(got, tt.want) {
				t.Errorf("project.godot = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestCreateProjectWritesSettings(t *testing.T) {
	tests := []struct {
		name    string
		main    string
		want    []string
		csproj  string
		dotnet  bool
		version string
	}{
		{
			name:    "description and relative scene",
			main:    "scenes/main.tscn",
			dotnet:  false,
			version: "4.7.2.stable",
			want: []string{
				`config/description="A demo game"`,
				`run/main_scene="res://scenes/main.tscn"`,
			},
		},
		{
			name:    "res path preserved",
			main:    "res://scenes/main.tscn",
			version: "4.7.2.stable",
			want:    []string{`run/main_scene="res://scenes/main.tscn"`},
		},
		{
			name:    "leading slash normalized",
			main:    "/scenes/main.tscn",
			version: "4.7.2.stable",
			want:    []string{`run/main_scene="res://scenes/main.tscn"`},
		},
		{
			name:    "dotnet uses detected sdk version",
			version: "4.7.2.stable",
			dotnet:  true,
			want: []string{
				`config/features=PackedStringArray("4.7", "C#")`,
				`project/assembly_name="Demo"`,
			},
			csproj: "4.7.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "game")
			ts := newToolset(dir, &versionSession{version: tt.version})
			args := `{"projectName":"Demo","description":"A demo game","mainScene":` + jsonString(tt.main)
			if tt.dotnet {
				args += `,"dotnet":true`
			}
			args += `}`
			result := callHandler(t, ts.createProject(), args)
			if result.IsError {
				t.Fatalf("unexpected error: %+v", result.Content)
			}
			if got := readProjectGodot(t, dir); !containsAll(got, tt.want...) {
				t.Errorf("project.godot = %q, want it to contain %v", got, tt.want)
			}
			if tt.csproj != "" {
				csproj, err := os.ReadFile(filepath.Join(dir, "Demo.csproj"))
				if err != nil {
					t.Fatalf("read csproj: %v", err)
				}
				if !strings.Contains(string(csproj), "Godot.NET.Sdk/"+tt.csproj) {
					t.Errorf("csproj = %q, want SDK %s", csproj, tt.csproj)
				}
			}
		})
	}
}

func TestCreateProjectRejectsInvalidSettings(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "description control char", args: `{"projectName":"Demo","description":"a\nb"}`, want: "Invalid project description."},
		{name: "main scene control char", args: `{"projectName":"Demo","mainScene":"a\nb"}`, want: "Invalid main scene path."},
		{name: "main scene traversal", args: `{"projectName":"Demo","mainScene":"../main.tscn"}`, want: "Invalid path."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "game")
			ts := newToolset(dir, &versionSession{version: "4.7.2.stable"})
			result := callHandler(t, ts.createProject(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if result.Content[0].Text != tt.want {
				t.Errorf("message = %q, want %q", result.Content[0].Text, tt.want)
			}
			if _, err := os.Stat(filepath.Join(dir, "project.godot")); !os.IsNotExist(err) {
				t.Errorf("project.godot written despite invalid settings (err=%v)", err)
			}
		})
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
