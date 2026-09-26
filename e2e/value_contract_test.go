package e2e

import (
	"encoding/json"
	"fmt"
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
	contract := loadValueContract(t)

	root, session := seedScene(t, "scenes/contract.tscn", "Node2D")
	requireGodot(t, session)
	writeE2EFile(t, filepath.Join(root, "scripts", "contract.gd"),
		"extends Node2D\n\n@export var config: Dictionary = {}\n")
	requireToolText(t, session, "attach_script", map[string]any{
		"scenePath":  "scenes/contract.tscn",
		"nodePath":   "root",
		"scriptPath": "scripts/contract.gd",
	})

	// Write the fixture as raw bytes so int and float spellings reach the
	// engine exactly as the fixture declares them.
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

	// Read -> write -> read: hand the read value straight back and require it
	// to survive unchanged.
	writeSceneConfig(t, session, read)
	again := readSceneConfig(t, session)
	if !reflect.DeepEqual(again, read) {
		t.Errorf("write-back changed the value:\n first = %#v\n second = %#v", read, again)
	}
}

// TestValueContractGoEncoderHalf feeds the Go encoder's output through the real
// engine. modify_project_settings decodes each fixture value and formats its
// Godot literal, which lands in project.godot; read_project_settings then loads
// the file with Godot, so every literal the encoder produced must be one the
// engine parses. Unlike TestValueContractScriptHalf, the engine never sees the
// JSON contract here.
func TestValueContractGoEncoderHalf(t *testing.T) {
	contract := loadValueContract(t)
	root := newProject(t)
	session := connectProject(t, root)
	requireGodot(t, session)

	keys := make([]string, len(contract.Values))
	for i, entry := range contract.Values {
		keys[i] = fmt.Sprintf("k%02d", i)
		res := callTool(t, session, "modify_project_settings", map[string]any{
			"section": "contract",
			"key":     keys[i],
			"value":   entry.Value,
		})
		if res.IsError {
			t.Fatalf("%s: modify_project_settings error: %s", entry.Name, firstText(t, res))
		}
	}

	// The file holds exactly the literals the Go encoder formatted.
	content := readFile(t, filepath.Join(root, "project.godot"))
	for i, entry := range contract.Values {
		if want := keys[i] + "=" + entry.Godot; !strings.Contains(content, want) {
			t.Errorf("project.godot missing encoder output %q", want)
		}
	}

	// read_project_settings loads the whole file with Godot first, so a literal
	// the engine rejects fails here rather than silently passing.
	sections := requireProjectSettingsRoundTrip(t, session)
	stored, ok := sections["contract"]
	if !ok {
		t.Fatalf("contract section missing; sections = %#v", sections)
	}
	if len(stored) != len(contract.Values) {
		t.Errorf("stored %d settings, want %d", len(stored), len(contract.Values))
	}
	for i, entry := range contract.Values {
		got, ok := stored[keys[i]]
		if !ok {
			t.Errorf("%s: key %s missing from read-back", entry.Name, keys[i])
			continue
		}
		// Tags the Go codec models directly come back exactly. Engine-only tags
		// canonicalize to the GodotText escape hatch, whose round trip the Go
		// half already pins; here the engine acceptance above is what matters.
		if !valueContractTagsSurvive(entry.Value) {
			continue
		}
		var want any
		if err := json.Unmarshal(entry.Value, &want); err != nil {
			t.Fatalf("unmarshal fixture %s: %v", entry.Name, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: read-back = %#v, want %#v", entry.Name, got, want)
		}
	}
}

// readbackTypedTags are the fixture tags the Go codec models as their own Value
// type, so they survive a write-read round trip. Every other engine tag decodes
// to Opaque and re-encodes as GodotText.
var readbackTypedTags = map[string]bool{
	"StringName":        true,
	"PackedStringArray": true,
	"GodotText":         true,
}

// valueContractTagsSurvive reports whether every tagged object in a fixture
// value uses a tag the Go codec models directly, so the read-back equals the
// fixture value. A value with an engine-only tag anywhere (including nested)
// does not.
func valueContractTagsSurvive(raw json.RawMessage) bool {
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return false
	}
	var walk func(any) bool
	walk = func(node any) bool {
		switch n := node.(type) {
		case map[string]any:
			if tag, ok := n["$type"].(string); ok && !readbackTypedTags[tag] {
				return false
			}
			for _, child := range n {
				if !walk(child) {
					return false
				}
			}
		case []any:
			for _, child := range n {
				if !walk(child) {
					return false
				}
			}
		}
		return true
	}
	return walk(tree)
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
func writeSceneConfig(t *testing.T, session *mcp.ClientSession, config any) {
	t.Helper()
	requireToolText(t, session, "modify_scene_node", map[string]any{
		"scenePath":  "scenes/contract.tscn",
		"nodePath":   "root",
		"properties": map[string]any{"config": config},
	})
}

// readSceneConfig reads the scene's root node and returns the config Dictionary
// in the tool value contract.
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
