package tools

import (
	"path/filepath"
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
		{name: "empty", sub: "", ok: false},
		{name: "parent", sub: "../escape", ok: false},
		{name: "nested parent", sub: "a/../../escape", ok: false},
		{name: "absolute", sub: filepath.Join(string(filepath.Separator), "etc", "passwd"), ok: false},
		{name: "res scheme", sub: "res://scenes/main.tscn", ok: false},
		{name: "user scheme", sub: "user://save.dat", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := confinePath(root, tt.sub)
			if ok != tt.ok {
				t.Fatalf("confinePath(%q, %q) ok = %v, want %v", root, tt.sub, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("confinePath(%q, %q) = %q, want %q", root, tt.sub, got, tt.want)
			}
		})
	}
	if _, ok := confinePath("", "scenes/main.tscn"); ok {
		t.Error("confinePath with empty root should not be ok")
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
