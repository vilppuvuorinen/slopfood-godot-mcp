package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// valueContract is the shared conformance fixture: the canonical tagged values
// both halves of the codec must accept. It is read from the repository root so
// this suite (a separate module) and internal/godotconfig/json_test.go consume
// the same bytes.
type valueContract struct {
	Values []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
		Godot string          `json:"godot"`
	} `json:"values"`
}

// loadValueContract reads the shared fixture, failing when it is missing or
// empty so a renamed fixture cannot silently drop the conformance check.
func loadValueContract(t *testing.T) valueContract {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "testdata", "value_contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read value contract: %v", err)
	}
	var contract valueContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("unmarshal value contract: %v", err)
	}
	if len(contract.Values) == 0 {
		t.Fatal("value contract has no entries")
	}
	return contract
}

// TestValueContractScriptHalf runs the shared conformance fixture through the
// real engine, which is the only place the GDScript decoder runs. It proves a
// read-write-read identity for every fixture entry: writing the whole table,
// reading it back, writing that read value unchanged, and reading it again must
// be stable. The fixture is also decoded by the Go half in
// internal/godotconfig/json_test.go, so a tag one decoder gains or loses
// without the other following fails a test on one side.
func TestValueContractScriptHalf(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	contract := loadValueContract(t)

	root := seedScene(t, session, "scenes/contract.tscn", "Node2D")
	writeE2EFile(t, filepath.Join(root, "scripts", "contract.gd"),
		"extends Node2D\n\n@export var config: Dictionary = {}\n")
	requireToolText(t, session, "attach_script", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/contract.tscn",
		"nodePath":    "root",
		"scriptPath":  "scripts/contract.gd",
	})

	// Write the fixture as raw bytes so int and float spellings reach the
	// engine exactly as the fixture declares them.
	writeSceneConfig(t, session, root, contractJSON(t, contract))

	read := readSceneConfig(t, session, root)
	for _, entry := range contract.Values {
		var parsed any
		if err := json.Unmarshal(entry.Value, &parsed); err != nil {
			t.Fatalf("unmarshal fixture value %q: %v", entry.Name, err)
		}
		if got, ok := read[entry.Name]; !ok {
			t.Errorf("%s: missing from read-back", entry.Name)
		} else if !reflect.DeepEqual(got, parsed) {
			t.Errorf("%s: read-back = %#v, want %#v", entry.Name, got, parsed)
		}
	}

	// Read -> write -> read: hand the read value straight back and require it
	// to survive unchanged.
	writeSceneConfig(t, session, root, read)
	again := readSceneConfig(t, session, root)
	if !reflect.DeepEqual(again, read) {
		t.Errorf("write-back changed the value:\n first = %#v\n second = %#v", read, again)
	}
}

// contractJSON builds a single Dictionary value keyed by fixture name, with each
// value embedded as raw JSON so number spellings (int vs float) are preserved.
func contractJSON(t *testing.T, contract valueContract) json.RawMessage {
	t.Helper()
	var b strings.Builder
	b.WriteByte('{')
	for i, entry := range contract.Values {
		if i > 0 {
			b.WriteByte(',')
		}
		name, err := json.Marshal(entry.Name)
		if err != nil {
			t.Fatalf("marshal fixture name %q: %v", entry.Name, err)
		}
		b.Write(name)
		b.WriteByte(':')
		b.Write(entry.Value)
	}
	b.WriteByte('}')
	return json.RawMessage(b.String())
}

// writeSceneConfig writes the config Dictionary onto the scene's root node.
// config may be a json.RawMessage (to control number spellings) or any
// JSON-marshalable tree.
func writeSceneConfig(t *testing.T, session *mcp.ClientSession, root string, config any) {
	t.Helper()
	requireToolText(t, session, "modify_scene_node", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/contract.tscn",
		"nodePath":    "root",
		"properties":  map[string]any{"config": config},
	})
}

// readSceneConfig reads the scene's root node and returns the config Dictionary
// in the tool value contract.
func readSceneConfig(t *testing.T, session *mcp.ClientSession, root string) map[string]any {
	t.Helper()
	text := requireToolText(t, session, "read_scene", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/contract.tscn",
	})
	var tree sceneNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal scene %q: %v", text, err)
	}
	config, ok := tree.Properties["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want an object; tree = %#v", tree.Properties["config"], tree)
	}
	return config
}
