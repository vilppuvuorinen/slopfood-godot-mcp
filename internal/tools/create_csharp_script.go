package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func createCSharpScript() mcp.Tool {
	return mcp.Tool{
		Name:        "create_csharp_script",
		Title:       "Create C# Script",
		Description: "Create a C# script file in a Godot .NET project",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Godot .NET project path (must contain a .csproj)",
				},
				"scriptPath": map[string]any{
					"type":        "string",
					"description": `Script file path relative to project (e.g. "scripts/Player.cs")`,
				},
				"className": map[string]any{
					"type":        "string",
					"description": "C# class name. Default: derived from the file name",
				},
				"baseClass": map[string]any{
					"type":        "string",
					"description": "Godot base class to extend. Default: Node",
				},
				"namespaceName": map[string]any{
					"type":        "string",
					"description": "Optional C# namespace",
				},
				"methods": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Method stubs (e.g. _Ready, _Process) to include",
				},
				"source": map[string]any{
					"type":        "string",
					"description": "Full source code (overrides template)",
				},
			},
			"required": []string{"projectPath", "scriptPath"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"scriptPath": map[string]any{"type": "string"},
			"className":  map[string]any{"type": "string"},
		}),
		Handler: handleCreateCSharpScript,
	}
}

type createCSharpScriptArgs struct {
	ProjectPath   string   `json:"projectPath"`
	ScriptPath    string   `json:"scriptPath"`
	ClassName     string   `json:"className"`
	BaseClass     string   `json:"baseClass"`
	NamespaceName string   `json:"namespaceName"`
	Methods       []string `json:"methods"`
	Source        string   `json:"source"`
}

func handleCreateCSharpScript(ctx context.Context, raw json.RawMessage) (any, error) {
	var args createCSharpScriptArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" || args.ScriptPath == "" {
		return mcp.ErrorResult("projectPath and scriptPath are required."), nil
	}
	fullPath, pathOK := confinePath(args.ProjectPath, args.ScriptPath)
	if !validPath(args.ProjectPath) || !pathOK {
		return mcp.ErrorResult("Invalid path."), nil
	}
	if !pathExists(filepath.Join(args.ProjectPath, "project.godot")) {
		return notAProjectResult(args.ProjectPath), nil
	}
	if !isDotnetProject(args.ProjectPath) {
		return mcp.ErrorResult("Not a Godot .NET project (no .csproj found). Use create_project with dotnet: true first."), nil
	}
	if !strings.HasSuffix(strings.ToLower(args.ScriptPath), ".cs") {
		return mcp.ErrorResult("scriptPath must end with .cs"), nil
	}

	base := filepath.Base(args.ScriptPath)
	fileBase := base[:len(base)-3]
	if !validClassName(fileBase) {
		return mcp.ErrorResult(fmt.Sprintf(`Invalid C# script file name "%s.cs": the name must be a valid class name (letters, digits, underscore; not starting with a digit), because Godot requires the class name to match the file name.`, fileBase)), nil
	}
	if args.ClassName != "" && args.ClassName != fileBase {
		return mcp.ErrorResult(fmt.Sprintf(`className "%s" must match the script file name "%s" for Godot to attach the script.`, args.ClassName, fileBase)), nil
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil { //nolint:gosec // scriptPath is caller-controlled.
		return nil, fmt.Errorf("create script directory: %w", err)
	}
	source := args.Source
	if source == "" {
		source = generateCsharpScriptSource(csharpScriptOptions{
			ClassName:     fileBase,
			BaseClass:     args.BaseClass,
			NamespaceName: args.NamespaceName,
			Methods:       args.Methods,
		})
	}
	if err := os.WriteFile(fullPath, []byte(source), 0o644); err != nil { //nolint:gosec // user-facing script file.
		return nil, fmt.Errorf("write script file: %w", err)
	}
	return statusResult("C# script created at "+args.ScriptPath, map[string]any{
		"scriptPath": args.ScriptPath,
		"className":  fileBase,
	}), nil
}

// csharpScriptOptions are the inputs to the C# template generator, mirroring
// the reference server's CsharpScriptOptions.
type csharpScriptOptions struct {
	ClassName     string
	BaseClass     string
	NamespaceName string
	Methods       []string
}

// csharpGodotOverrides maps well-known Godot virtual method names to their C#
// override signatures.
var csharpGodotOverrides = map[string]string{
	"_Ready":          "public override void _Ready()",
	"_Process":        "public override void _Process(double delta)",
	"_PhysicsProcess": "public override void _PhysicsProcess(double delta)",
	"_Input":          "public override void _Input(InputEvent @event)",
	"_UnhandledInput": "public override void _UnhandledInput(InputEvent @event)",
	"_EnterTree":      "public override void _EnterTree()",
	"_ExitTree":       "public override void _ExitTree()",
}

// toDotnetNamespace turns a dotted name into a valid C# namespace, sanitizing
// each segment like toDotnetIdentifier.
func toDotnetNamespace(name string) string {
	segments := strings.Split(name, ".")
	for i, segment := range segments {
		segments[i] = toDotnetIdentifier(segment)
	}
	return strings.Join(segments, ".")
}

// generateCsharpScriptSource builds a Godot C# script template with the
// requested class, base class, optional namespace, and method stubs.
func generateCsharpScriptSource(opts csharpScriptOptions) string {
	className := toDotnetIdentifier(opts.ClassName)
	baseClass := strings.TrimSpace(opts.BaseClass)
	if baseClass == "" {
		baseClass = "Node"
	}
	indent := "\t"
	if opts.NamespaceName != "" {
		indent = "\t\t"
	}
	bodyIndent := indent + "\t"

	methodBlocks := []string{}
	seen := map[string]bool{}
	for _, raw := range opts.Methods {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		signature, ok := csharpGodotOverrides[name]
		if !ok {
			signature = "public void " + toDotnetIdentifier(name) + "()"
		}
		methodBlocks = append(methodBlocks, indent+signature+"\n"+indent+"{\n"+bodyIndent+"\n"+indent+"}")
	}
	body := indent
	if len(methodBlocks) > 0 {
		body = strings.Join(methodBlocks, "\n\n")
	}

	classIndent := ""
	if opts.NamespaceName != "" {
		classIndent = "\t"
	}
	classBlock := classIndent + "public partial class " + className + " : " + baseClass + "\n" +
		classIndent + "{\n" +
		body + "\n" +
		classIndent + "}"

	lines := []string{"using Godot;", ""}
	if opts.NamespaceName != "" {
		lines = append(lines, "namespace "+toDotnetNamespace(opts.NamespaceName)+";", "")
	}
	lines = append(lines, classBlock, "")
	return strings.Join(lines, "\n")
}

// isDotnetProject reports whether projectPath contains a .csproj file, which
// Godot requires for C# scripting support. It is shared by the C# script
// helpers.
func isDotnetProject(projectPath string) bool {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".csproj") {
			return true
		}
	}
	return false
}
