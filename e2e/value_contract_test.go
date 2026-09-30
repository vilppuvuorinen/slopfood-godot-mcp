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

// Shared conformance fixture consumed by this module and internal/godotconfig/json_test.go.
type valueContract struct {
	Values []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
		Godot string          `json:"godot"`
	} `json:"values"`
}

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

func TestValueContractScriptHalf(t *testing.T) {
	t.Parallel()
	contract := loadValueContract(t)

	root, session := seedScene(t, "scenes/contract.tscn", "Node2D")
	writeE2EFile(t, filepath.Join(root, "scripts", "contract.gd"),
		"extends Node2D\n\n@export var config: Dictionary = {}\n")
	requireToolText(t, session, "attach_script", map[string]any{
		"scenePath":  "scenes/contract.tscn",
		"nodePath":   "root",
		"scriptPath": "scripts/contract.gd",
	})

	// Raw bytes keep int and float spellings exactly as the fixture declares them.
	writeSceneConfig(t, session, contractJSON(t, contract))

	read := readSceneConfig(t, session)
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

	writeSceneConfig(t, session, read)
	again := readSceneConfig(t, session)
	if !reflect.DeepEqual(again, read) {
		t.Errorf("write-back changed the value:\n first = %#v\n second = %#v", read, again)
	}
}

// Embedding values as raw JSON preserves number spellings (int vs float).
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

// config may be a json.RawMessage (to control number spellings) or a JSON tree.
func writeSceneConfig(t *testing.T, session *mcp.ClientSession, config any) {
	t.Helper()
	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath":  "scenes/contract.tscn",
		"nodePath":   "root",
		"properties": map[string]any{"config": config},
	})
}

func readSceneConfig(t *testing.T, session *mcp.ClientSession) map[string]any {
	t.Helper()
	text := requireToolText(t, session, "read_scene", map[string]any{
		"scenePath": "scenes/contract.tscn",
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
