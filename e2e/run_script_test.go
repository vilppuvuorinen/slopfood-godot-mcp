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
