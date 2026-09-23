package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	result := callToolArgs(t, createProject(), `{"projectPath":`+jsonString(dir)+`,"projectName":"Evil\" config/name=\"pwned"}`)
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
	result := callToolArgs(t, createProject(), `{"projectPath":`+jsonString(dir)+`,"projectName":"Evil\nconfig/name=\"pwned"}`)
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
