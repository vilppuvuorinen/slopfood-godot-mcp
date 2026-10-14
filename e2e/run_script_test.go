package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

type runScriptDoc struct {
	Compiled bool             `json:"compiled"`
	Result   any              `json:"result"`
	Logs     []string         `json:"logs"`
	Errors   []map[string]any `json:"errors"`
}

func decodeRunScript(t *testing.T, text string) runScriptDoc {
	t.Helper()
	var doc runScriptDoc
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return doc
}

func TestRunScriptTool(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("returns the body's return value and printed logs", func(t *testing.T) {
		body := "print(\"sum is \", 1 + 1)\nreturn {\"answer\": 42}\n"
		doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": body}))
		if !doc.Compiled {
			t.Fatalf("compiled = false, errors = %+v", doc.Errors)
		}
		if len(doc.Errors) != 0 {
			t.Errorf("errors = %+v, want none", doc.Errors)
		}
		if len(doc.Logs) != 1 || doc.Logs[0] != "sum is 2" {
			t.Errorf("logs = %v, want [sum is 2]", doc.Logs)
		}
		result, ok := doc.Result.(map[string]any)
		if !ok {
			t.Fatalf("result = %#v, want an object", doc.Result)
		}
		if result["answer"] != float64(42) {
			t.Errorf("result.answer = %v, want 42", result["answer"])
		}
	})

	t.Run("returns null when the body has no return", func(t *testing.T) {
		body := "print(\"side effect\")\n"
		doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": body}))
		if !doc.Compiled {
			t.Fatalf("compiled = false, errors = %+v", doc.Errors)
		}
		if doc.Result != nil {
			t.Errorf("result = %#v, want null", doc.Result)
		}
		if len(doc.Logs) != 1 || doc.Logs[0] != "side effect" {
			t.Errorf("logs = %v, want [side effect]", doc.Logs)
		}
	})

	t.Run("normalises a space-indented body", func(t *testing.T) {
		body := "var total := 0\nfor value in range(3):\n    total += value\nreturn total\n"
		doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": body}))
		if !doc.Compiled {
			t.Fatalf("compiled = false, errors = %+v", doc.Errors)
		}
		if doc.Result != float64(3) {
			t.Errorf("result = %v, want 3", doc.Result)
		}
	})

	t.Run("reports parse errors on the body line", func(t *testing.T) {
		body := "var a := 1\nvar b := 2\nvar c := (\n"
		doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": body}))
		if doc.Compiled {
			t.Fatal("compiled = true, want false")
		}
		if len(doc.Errors) == 0 {
			t.Fatal("errors = none, want a parse diagnostic")
		}
		// The generated @tool/extends/func prelude must not shift the reported line.
		if line, _ := doc.Errors[0]["line"].(float64); line != 3 {
			t.Errorf("parse error line = %v, want 3", doc.Errors[0]["line"])
		}
	})

	t.Run("reports runtime errors", func(t *testing.T) {
		body := "var data := {}\nprint(data.missing)\nreturn 1\n"
		doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": body}))
		if !doc.Compiled {
			t.Fatalf("compiled = false, errors = %+v", doc.Errors)
		}
		if len(doc.Errors) == 0 {
			t.Fatal("errors = none, want the invalid access diagnostic")
		}
	})
}

// TestRunScriptRealInstances covers the non-editor execution path: a node
// loaded from a scene gets a real script instance, so plain member state and
// methods work (an editor-session placeholder exposes only exported properties
// and cannot run script code).
func TestRunScriptRealInstances(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "scripts", "target.gd"), `extends StaticBody3D

@export var label: String = ""
var last_region: String = ""

func ping() -> String:
	return "pong"
`)
	writeE2EFile(t, filepath.Join(root, "scenes", "target.tscn"), `[gd_scene load_steps=2 format=3]

[ext_resource type="Script" path="res://scripts/target.gd" id="1"]

[node name="RangeTarget" type="StaticBody3D"]
script = ExtResource("1")
`)
	session := connectProject(t, root)

	body := `var inst = load("res://scenes/target.tscn").instantiate()
inst.set("last_region", "x")
inst.set("label", "abc")
return {"last_region": inst.get("last_region"), "label": inst.get("label"), "ping": inst.call("ping")}
`
	doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": body}))
	if !doc.Compiled {
		t.Fatalf("compiled = false, errors = %+v", doc.Errors)
	}
	if len(doc.Errors) != 0 {
		t.Errorf("errors = %+v, want none", doc.Errors)
	}
	result, ok := doc.Result.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want an object", doc.Result)
	}
	if result["last_region"] != "x" {
		t.Errorf("last_region = %v, want x", result["last_region"])
	}
	if result["label"] != "abc" {
		t.Errorf("label = %v, want abc", result["label"])
	}
	if result["ping"] != "pong" {
		t.Errorf("ping = %v, want pong", result["ping"])
	}
}

// A run_script save goes through a throwaway non-editor engine, which cannot
// query the editor UID database. The bridge must still preserve the uid= on a
// scene header and its ext_resources so the editor does not regenerate them and
// churn references on its next save.
func TestRunScriptPreservesResourceUIDs(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeE2EFile(t, filepath.Join(root, "player.gd"), "extends Node\n")
	writeE2EFile(t, filepath.Join(root, "player.gd.uid"), "uid://bplayeruid0001\n")
	scene := "[gd_scene load_steps=2 format=3 uid=\"uid://csceneuid0001\"]\n\n" +
		"[ext_resource type=\"Script\" uid=\"uid://bplayeruid0001\" path=\"res://player.gd\" id=\"1_player\"]\n\n" +
		"[node name=\"root\" type=\"Node2D\"]\nscript = ExtResource(\"1_player\")\n"
	writeE2EFile(t, filepath.Join(root, "main.tscn"), scene)
	session := connectProject(t, root)

	doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": `var packed_scene: PackedScene = load("res://main.tscn")
var root_node: Node = packed_scene.instantiate()
var added := Node.new()
added.name = "Added"
root_node.add_child(added)
added.owner = root_node
var packed := PackedScene.new()
packed.pack(root_node)
return ResourceSaver.save(packed, "res://main.tscn")
`}))
	if code, ok := doc.Result.(float64); !ok || code != 0 {
		t.Fatalf("save result = %v, want 0 (errors = %+v)", doc.Result, doc.Errors)
	}

	after := readFile(t, filepath.Join(root, "main.tscn"))
	requireContains(t, after,
		`uid="uid://csceneuid0001"`,
		`uid="uid://bplayeruid0001"`,
		`name="Added"`,
	)

	// A text resource the body creates gets a fresh UID, matching an editor save.
	doc = decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": `var node := Node2D.new()
node.name = "root"
var packed := PackedScene.new()
packed.pack(node)
return ResourceSaver.save(packed, "res://fresh.tscn")
`}))
	if code, ok := doc.Result.(float64); !ok || code != 0 {
		t.Fatalf("fresh save result = %v, want 0 (errors = %+v)", doc.Result, doc.Errors)
	}
	requireContains(t, readFile(t, filepath.Join(root, "fresh.tscn")), `uid="uid://`)
}

func TestRunScriptErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing body", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireToolError(t, session, "run_script", map[string]any{}, "body is required.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		session := connectProject(t, notProject)
		requireToolError(t, session, "run_script", map[string]any{
			"body": "return 0\n",
		}, "Not a valid Godot project: "+notProject)
	})
}
