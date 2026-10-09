package tools

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/tmp/project", want: true},
		{path: "scenes/main.tscn", want: true},
		{path: "", want: false},
		{path: "../escape", want: false},
		{path: "res://../escape", want: false},
		{path: "foo..bar.tscn", want: false},
		{path: "scenes/..", want: false},
		{path: "foo/../bar.tscn", want: false},
		{path: "../proj", want: false},
	}
	for _, tt := range tests {
		if got := validPath(tt.path); got != tt.want {
			t.Errorf("validPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestConfinePath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "proj")
	tests := []struct {
		name string
		sub  string
		want string
		ok   bool
	}{
		{name: "relative", sub: "scenes/main.tscn", want: filepath.Join(root, "scenes", "main.tscn"), ok: true},
		{name: "normalized", sub: "scenes/../player.gd", want: filepath.Join(root, "player.gd"), ok: true},
		{name: "res scheme", sub: "res://scenes/main.tscn", want: filepath.Join(root, "scenes", "main.tscn"), ok: true},
		{name: "empty", sub: "", ok: false},
		{name: "parent", sub: "../escape", ok: false},
		{name: "nested parent", sub: "a/../../escape", ok: false},
		{name: "absolute", sub: filepath.Join(string(filepath.Separator), "etc", "passwd"), ok: false},
		{name: "user scheme", sub: "user://save.dat", ok: false},
		{name: "res escape", sub: "res://../escape", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := confinePath(root, tt.sub)
			if (err == nil) != tt.ok {
				t.Fatalf("confinePath(%q, %q) err = %v, want ok %v", root, tt.sub, err, tt.ok)
			}
			if tt.ok && got != tt.want {
				t.Errorf("confinePath(%q, %q) = %q, want %q", root, tt.sub, got, tt.want)
			}
		})
	}
	if _, err := confinePath("", "scenes/main.tscn"); err == nil {
		t.Error("confinePath with empty root should error")
	}
}

func TestConfinePathUserSchemeHint(t *testing.T) {
	_, err := confinePath("/proj", "user://save.dat")
	var pe *pathError
	if !errors.As(err, &pe) {
		t.Fatalf("confinePath err = %v, want *pathError", err)
	}
	if !strings.Contains(pe.reason, "user://") {
		t.Errorf("reason = %q, want it to mention user://", pe.reason)
	}
	if len(pe.hints) == 0 {
		t.Error("user:// rejection should carry a recovery hint")
	}
}

func TestConfineAbsPath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "proj")
	abs := filepath.Join(string(filepath.Separator), "etc", "passwd")
	tests := []struct {
		name string
		sub  string
		want string
		ok   bool
	}{
		{name: "relative", sub: "build/game.x86_64", want: filepath.Join(root, "build", "game.x86_64"), ok: true},
		{name: "normalized", sub: "build/../game.x86_64", want: filepath.Join(root, "game.x86_64"), ok: true},
		{name: "res scheme", sub: "res://build/game.x86_64", want: filepath.Join(root, "build", "game.x86_64"), ok: true},
		{name: "absolute", sub: abs, want: abs, ok: true},
		{name: "absolute parent", sub: filepath.Join(root, "..", "escape"), want: filepath.Join(string(filepath.Separator), "escape"), ok: true},
		{name: "empty", sub: "", ok: false},
		{name: "parent", sub: "../escape", ok: false},
		{name: "nested parent", sub: "a/../../escape", ok: false},
		{name: "user scheme", sub: "user://build/game.x86_64", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := confineAbsPath(root, tt.sub)
			if (err == nil) != tt.ok {
				t.Fatalf("confineAbsPath(%q, %q) err = %v, want ok %v", root, tt.sub, err, tt.ok)
			}
			if tt.ok && got != tt.want {
				t.Errorf("confineAbsPath(%q, %q) = %q, want %q", root, tt.sub, got, tt.want)
			}
		})
	}
	if _, err := confineAbsPath("", "build/game.x86_64"); err == nil {
		t.Error("confineAbsPath with empty root should error")
	}
}

func TestValidClassName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "Node2D", want: true},
		{name: "CharacterBody3D", want: true},
		{name: "_Private", want: true},
		{name: "Node2D_", want: true},
		{name: "", want: false},
		{name: "2Node", want: false},
		{name: "res://evil.gd", want: false},
		{name: "Node2D.tscn", want: false},
		{name: "Node2D/Child", want: false},
		{name: "Node 2D", want: false},
	}
	for _, tt := range tests {
		if got := validClassName(tt.name); got != tt.want {
			t.Errorf("validClassName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
