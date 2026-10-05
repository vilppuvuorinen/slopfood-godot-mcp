package tools

import (
	"path/filepath"
	"testing"
)

func TestNormalizeReadPath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "proj")
	abs := filepath.Join(string(filepath.Separator), "etc", "passwd")
	tests := []struct {
		name    string
		project string
		path    string
		want    string
		ok      bool
	}{
		{name: "relative", project: root, path: "scenes/main.tscn", want: filepath.Join(root, "scenes", "main.tscn"), ok: true},
		{name: "res scheme", project: root, path: "res://scenes/main.tscn", want: filepath.Join(root, "scenes", "main.tscn"), ok: true},
		{name: "user scheme", project: root, path: "user://save.dat", ok: false},
		{name: "absolute", project: root, path: abs, ok: false},
		{name: "leading parent", project: root, path: "../escape.tscn", ok: false},
		{name: "trailing parent", project: root, path: "scenes/..", ok: false},
		{name: "normalized parent", project: root, path: "foo/../bar.tscn", ok: false},
		{name: "innocuous dots", project: root, path: "foo..bar.tscn", ok: false},
		{name: "bound root parent", project: "../escape", path: "scenes/main.tscn", ok: false},
		{name: "empty path", project: root, path: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeReadPath(tt.project, tt.path)
			if (err == nil) != tt.ok {
				t.Fatalf("normalizeReadPath(%q, %q) err = %v, want ok %v", tt.project, tt.path, err, tt.ok)
			}
			if tt.ok && got != tt.want {
				t.Errorf("normalizeReadPath(%q, %q) = %q, want %q", tt.project, tt.path, got, tt.want)
			}
		})
	}
}
