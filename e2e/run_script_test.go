package e2e

import (
	"encoding/json"
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

	t.Run("runs in the live editor session", func(t *testing.T) {
		body := "return EditorInterface.get_resource_filesystem() != null\n"
		doc := decodeRunScript(t, requireToolText(t, session, "run_script", map[string]any{"body": body}))
		if !doc.Compiled {
			t.Fatalf("compiled = false, errors = %+v", doc.Errors)
		}
		if doc.Result != true {
			t.Errorf("result = %v, want true (EditorInterface available)", doc.Result)
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
