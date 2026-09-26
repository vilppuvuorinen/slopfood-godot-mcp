package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const textureImport = `[remap]

importer="texture"
type="CompressedTexture2D"
uid="uid://c8q3v7x1abcde"
path="res://.godot/imported/icon.svg-1a2b3c4d.ctex"
metadata={
"vram_texture": false
}

[deps]

source_file="res://icon.svg"
dest_files=["res://.godot/imported/icon.svg-1a2b3c4d.ctex"]

[params]

compress/mode=0
compress/lossy_quality=0.7
`

// readImportPayload calls read_import and decodes its payload.
func readImportPayload(t *testing.T, root, resourcePath string) importMetadata {
	t.Helper()
	result := callToolArgs(t, readImport(), root, `{"resourcePath":`+jsonString(resourcePath)+`}`)
	if result.IsError {
		t.Fatalf("read_import error: %+v", result.Content)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var payload importMetadata
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload %s: %v", data, err)
	}
	return payload
}

func TestReadImportParsesMetadata(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "icon.svg"), "<svg/>")
	seedFile(t, filepath.Join(root, "icon.svg.import"), textureImport)

	payload := readImportPayload(t, root, "icon.svg")
	if payload.SourcePath != "icon.svg" || payload.ImportPath != "icon.svg.import" {
		t.Errorf("paths = %q, %q", payload.SourcePath, payload.ImportPath)
	}
	if payload.Importer != "texture" || payload.Type != "CompressedTexture2D" {
		t.Errorf("importer/type = %q/%q", payload.Importer, payload.Type)
	}
	if payload.UID != "uid://c8q3v7x1abcde" {
		t.Errorf("uid = %q", payload.UID)
	}
	if payload.SourceFile != "res://icon.svg" || len(payload.DestFiles) != 1 {
		t.Errorf("deps = %q, %v", payload.SourceFile, payload.DestFiles)
	}
	metadata, _ := payload.Metadata.(map[string]any)
	if metadata["vram_texture"] != false {
		t.Errorf("metadata = %v", payload.Metadata)
	}
	if payload.Params["compress/mode"] != float64(0) {
		t.Errorf("params = %v", payload.Params)
	}

	// The ".import" path itself is accepted.
	direct := readImportPayload(t, root, "icon.svg.import")
	if direct.SourcePath != "icon.svg" || direct.ImportPath != "icon.svg.import" {
		t.Errorf("direct paths = %q, %q", direct.SourcePath, direct.ImportPath)
	}
}

func TestReadImportAcceptsPackedDestFiles(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "a.png.import"), `[deps]

source_file="res://a.png"
dest_files=PackedStringArray("res://.godot/imported/a.png.ctex")
`)

	payload := readImportPayload(t, root, "a.png")
	if len(payload.DestFiles) != 1 || payload.DestFiles[0] != "res://.godot/imported/a.png.ctex" {
		t.Errorf("destFiles = %v", payload.DestFiles)
	}
	if payload.Params == nil || len(payload.Params) != 0 {
		t.Errorf("params = %v, want empty non-nil", payload.Params)
	}
}

func TestReadImportErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	result := callToolArgs(t, readImport(), root, `{"resourcePath":"missing.svg"}`)
	if !result.IsError {
		t.Fatal("expected an error result")
	}
	if got := result.Content[0].Text; !strings.Contains(got, "Import metadata not found: missing.svg.import") {
		t.Errorf("message = %q", got)
	}

	result = callToolArgs(t, readImport(), root, `{"resourcePath":"../escape.svg"}`)
	if !result.IsError || !strings.Contains(result.Content[0].Text, "Invalid resource path") {
		t.Errorf("escaping path result = %+v", result.Content)
	}
}

func TestManageImportValidation(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "icon.svg.import"), textureImport)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing args", args: `{}`, want: "resourcePath and action are required."},
		{name: "unknown action", args: `{"resourcePath":"icon.svg","action":"bogus"}`, want: "Unknown action: bogus."},
		{name: "setParam without name", args: `{"resourcePath":"icon.svg","action":"setParam","value":1}`, want: "name is required for the setParam action."},
		{name: "setParam without value", args: `{"resourcePath":"icon.svg","action":"setParam","name":"compress/mode"}`, want: "value is required for the setParam action."},
		{name: "removeParam without name", args: `{"resourcePath":"icon.svg","action":"removeParam"}`, want: "name is required for the removeParam action."},
		{name: "missing import", args: `{"resourcePath":"missing.svg","action":"removeParam","name":"x"}`, want: "Import metadata not found: missing.svg.import"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageImport(), root, tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result")
			}
			if got := result.Content[0].Text; !strings.Contains(got, tt.want) {
				t.Errorf("message = %q, want containing %q", got, tt.want)
			}
		})
	}
}
