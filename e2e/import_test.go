package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testImportMetadata is the subset of read_import's payload the tests assert
// against.
type testImportMetadata struct {
	SourcePath string         `json:"sourcePath"`
	ImportPath string         `json:"importPath"`
	Importer   string         `json:"importer"`
	Type       string         `json:"type"`
	UID        string         `json:"uid"`
	SourceFile string         `json:"sourceFile"`
	DestFiles  []string       `json:"destFiles"`
	Params     map[string]any `json:"params"`
}

// TestReadImportSchema pins the read-only import tool's input surface.
func TestReadImportSchema(t *testing.T) {
	schema := requireSchema(t, "read_import", "Read the .import metadata sidecar of an imported resource")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requirePropCount(t, 1)
}

// TestManageImportSchema pins the action tool's input surface.
func TestManageImportSchema(t *testing.T) {
	schema := requireSchema(t, "manage_import", "Set or remove a resource's import parameter and reimport it")
	schema.requireRequired(t, "resourcePath", "action")
	schema.requireStringProps(t, "resourcePath", "action", "name")
	schema.requirePropCount(t, 4)
}

// readImportPayloadE2E calls read_import and decodes its payload.
func readImportPayloadE2E(t *testing.T, session *mcp.ClientSession, resourcePath string) testImportMetadata {
	t.Helper()
	text := requireToolText(t, session, "read_import", map[string]any{"resourcePath": resourcePath})
	var payload testImportMetadata
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal import payload: %v\n%s", err, text)
	}
	return payload
}

// TestManageImportTool drives the real import tools against an asset the engine
// imported.
func TestManageImportTool(t *testing.T) {
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "icon.svg"), `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)
	session := connectProject(t, root)
	requireGodot(t, session)

	// --import is the headless path that generates the .import sidecar.
	requireToolText(t, session, "update_project_uids", nil)

	payload := readImportPayloadE2E(t, session, "icon.svg")
	if payload.SourcePath != "icon.svg" || payload.ImportPath != "icon.svg.import" {
		t.Fatalf("paths = %q, %q", payload.SourcePath, payload.ImportPath)
	}
	if payload.Importer != "texture" || payload.Type != "CompressedTexture2D" {
		t.Errorf("importer/type = %q/%q", payload.Importer, payload.Type)
	}
	if payload.UID == "" || payload.SourceFile != "res://icon.svg" || len(payload.DestFiles) != 1 {
		t.Errorf("metadata = %+v", payload)
	}

	requireToolText(t, session, "manage_import", map[string]any{
		"resourcePath": "icon.svg",
		"action":       "setParam",
		"name":         "compress/mode",
		"value":        1,
	})
	payload = readImportPayloadE2E(t, session, "icon.svg")
	if payload.Params["compress/mode"] != float64(1) {
		t.Errorf("compress/mode = %v, want 1", payload.Params["compress/mode"])
	}

	requireToolText(t, session, "manage_import", map[string]any{
		"resourcePath": "icon.svg",
		"action":       "removeParam",
		"name":         "compress/mode",
	})
	// Removing the override and reimporting lets the importer write its default
	// back, so the value must no longer be the override.
	payload = readImportPayloadE2E(t, session, "icon.svg")
	if value, ok := payload.Params["compress/mode"]; ok && value != float64(0) {
		t.Errorf("compress/mode = %v, want the default 0", value)
	}
}

// TestManageImportErrors covers the validation paths.
func TestManageImportErrors(t *testing.T) {
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "icon.svg"), "<svg/>")
	session := connectProject(t, root)
	requireGodot(t, session)

	res := callTool(t, session, "read_import", map[string]any{"resourcePath": "missing.svg"})
	if !res.IsError {
		t.Fatal("expected an error result")
	}
	requireContains(t, firstText(t, res), "Import metadata not found: missing.svg.import")
}
