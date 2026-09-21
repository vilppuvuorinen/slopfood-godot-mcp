package tools

import "testing"

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
