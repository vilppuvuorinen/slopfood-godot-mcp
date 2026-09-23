package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManageLayersSchema(t *testing.T) {
	tool := manageLayers()
	if tool.Name != "manage_layers" {
		t.Errorf("name = %q", tool.Name)
	}
	if tool.Description != "List/set named layer definitions in project" {
		t.Errorf("description = %q", tool.Description)
	}
	required, ok := tool.InputSchema["required"].([]string)
	if !ok || strings.Join(required, ",") != "projectPath,action" {
		t.Errorf("required = %v", tool.InputSchema["required"])
	}
	props, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T", tool.InputSchema["properties"])
	}
	if len(props) != 5 {
		t.Errorf("properties = %v", props)
	}
	for _, key := range []string{"projectPath", "action", "layerType", "layer", "name"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
	if layerProp, ok := props["layer"].(map[string]any); !ok || layerProp["type"] != "number" {
		t.Errorf("layer property = %v", props["layer"])
	}
}

func TestManageLayersList(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"),
		"config_version=5\n\n[layer_names]\n\nlayer_names/2d_render/layer_1=\"Ground\"\nlayer_names/3d_physics/layer_2=\"Water\"\n")

	result := callToolArgs(t, manageLayers(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	var out struct {
		Layers []struct {
			Type  string `json:"type"`
			Layer int    `json:"layer"`
			Name  string `json:"name"`
		} `json:"layers"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &out); err != nil {
		t.Fatalf("unmarshal %q: %v", result.Content[0].Text, err)
	}
	if len(out.Layers) != 2 {
		t.Fatalf("layers = %+v", out.Layers)
	}
	if out.Layers[0].Type != "2d_render" || out.Layers[0].Layer != 1 || out.Layers[0].Name != "Ground" {
		t.Errorf("layers[0] = %+v", out.Layers[0])
	}
	if out.Layers[1].Type != "3d_physics" || out.Layers[1].Layer != 2 || out.Layers[1].Name != "Water" {
		t.Errorf("layers[1] = %+v", out.Layers[1])
	}
}

func TestManageLayersSetReplaces(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"),
		"config_version=5\n\n[layer_names]\n\nlayer_names/2d_render/layer_1=\"Old\"\n")

	result := callToolArgs(t, manageLayers(),
		`{"projectPath":`+jsonString(root)+`,"action":"set","layerType":"2d_render","layer":1,"name":"Ground"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	if got := result.Content[0].Text; got != `Layer set: layer_names/2d_render/layer_1="Ground"` {
		t.Errorf("message = %q", got)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `layer_names/2d_render/layer_1="Ground"`) {
		t.Errorf("content = %q", content)
	}
	if strings.Contains(content, `"Old"`) {
		t.Errorf("old value still present: %q", content)
	}
}

func TestManageLayersSetInsertsIntoSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"),
		"config_version=5\n\n[layer_names]\n\nlayer_names/2d_render/layer_1=\"Ground\"\n")

	result := callToolArgs(t, manageLayers(),
		`{"projectPath":`+jsonString(root)+`,"action":"set","layerType":"3d_physics","layer":2,"name":"Water"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[layer_names]\n"+`layer_names/3d_physics/layer_2="Water"`) {
		t.Errorf("content = %q", content)
	}
}

func TestManageLayersSetAppendsSection(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	seedFile(t, filepath.Join(root, "project.godot"), "config_version=5\n")

	result := callToolArgs(t, manageLayers(),
		`{"projectPath":`+jsonString(root)+`,"action":"set","layerType":"2d_render","layer":1,"name":"Ground"}`)
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result.Content)
	}
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	want := "config_version=5\n\n[layer_names]\n" + `layer_names/2d_render/layer_1="Ground"` + "\n"
	if got := string(data); got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// TestManageLayersEscaping covers a quoted layer name and a key that needs
// quoting, and checks the existing escaped value round-trips through list.
func TestManageLayersEscaping(t *testing.T) {
	t.Run("quote in name round trips", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)

		result := callToolArgs(t, manageLayers(),
			`{"projectPath":`+jsonString(root)+`,"action":"set","layerType":"2d_render","layer":1,"name":`+jsonString(`Ground "A"`)+`}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		if !strings.Contains(string(data), `layer_names/2d_render/layer_1="Ground \"A\""`) {
			t.Errorf("project.godot = %q, want the escaped name", data)
		}

		list := callToolArgs(t, manageLayers(), `{"projectPath":`+jsonString(root)+`,"action":"list"}`)
		var out struct {
			Layers []layerDefinition `json:"layers"`
		}
		if err := json.Unmarshal([]byte(list.Content[0].Text), &out); err != nil {
			t.Fatalf("unmarshal %q: %v", list.Content[0].Text, err)
		}
		if len(out.Layers) != 1 || out.Layers[0].Name != `Ground "A"` {
			t.Errorf("layers = %+v", out.Layers)
		}
	})

	t.Run("newline in key", func(t *testing.T) {
		root := t.TempDir()
		makeProject(t, root)

		result := callToolArgs(t, manageLayers(),
			`{"projectPath":`+jsonString(root)+`,"action":"set","layerType":`+jsonString("2d\nrender")+`,"layer":1,"name":"Ground"}`)
		if result.IsError {
			t.Fatalf("unexpected error: %+v", result.Content)
		}
		data, err := os.ReadFile(filepath.Join(root, "project.godot"))
		if err != nil {
			t.Fatalf("read project.godot: %v", err)
		}
		if !strings.Contains(string(data), "\"layer_names/2d\nrender/layer_1\"=\"Ground\"") {
			t.Errorf("project.godot = %q, want the quoted multi-line key", data)
		}
	})
}

func TestManageLayersErrors(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)

	tests := []struct {
		name string
		args string
		want string
	}{
		{name: "missing params", args: `{}`, want: "projectPath and action are required."},
		{name: "missing action", args: `{"projectPath":` + jsonString(root) + `}`, want: "projectPath and action are required."},
		{name: "invalid path", args: `{"projectPath":"../escape","action":"list"}`, want: "Invalid path."},
		{name: "not a project", args: `{"projectPath":` + jsonString(t.TempDir()) + `,"action":"list"}`, want: "Not a valid Godot project: "},
		{name: "set missing fields", args: `{"projectPath":` + jsonString(root) + `,"action":"set"}`, want: "layerType, layer, and name are required for set."},
		{name: "set zero layer", args: `{"projectPath":` + jsonString(root) + `,"action":"set","layerType":"2d_render","layer":0,"name":"Ground"}`, want: "layerType, layer, and name are required for set."},
		{name: "unknown action", args: `{"projectPath":` + jsonString(root) + `,"action":"bogus"}`, want: "Unknown action: bogus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolArgs(t, manageLayers(), tt.args)
			if !result.IsError {
				t.Fatalf("expected an error result: %+v", result.Content)
			}
			if !strings.HasPrefix(result.Content[0].Text, tt.want) {
				t.Errorf("message = %q, want prefix %q", result.Content[0].Text, tt.want)
			}
		})
	}
}
