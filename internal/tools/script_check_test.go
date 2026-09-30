package tools

import "testing"

func TestParseGodotScriptDiagnosticGroups(t *testing.T) {
	stderr := "<<<GODOT_MCP_VALIDATE:res://a.gd>>>\n" +
		"<<<GODOT_MCP_VALIDATE:res://nested/b.gd>>>\n" +
		"SCRIPT ERROR: Parse Error: Unexpected token\n" +
		"    at: (res://nested/b.gd:7)\n" +
		"<<<GODOT_MCP_VALIDATE:res://c.gd>>>\n" +
		"SCRIPT ERROR: Identifier \"foo\" not declared\n"
	groups := parseGodotScriptDiagnosticGroups(stderr)
	if _, ok := groups["a.gd"]; ok {
		t.Fatalf("a.gd should have no diagnostics: %+v", groups["a.gd"])
	}
	b := groups["nested/b.gd"]
	if len(b) != 1 || b[0].Message != "Unexpected token" || b[0].File != "nested/b.gd" || b[0].Line != 7 {
		t.Fatalf("nested/b.gd = %+v", b)
	}
	c := groups["c.gd"]
	if len(c) != 1 || c[0].File != "c.gd" || c[0].Message != `Identifier "foo" not declared` {
		t.Fatalf("c.gd = %+v", c)
	}
}

func TestParseGodotScriptDiagnosticsDropsUnattributed(t *testing.T) {
	groups := parseGodotScriptDiagnosticGroups("SCRIPT ERROR: stray before any marker\n")
	if len(groups) != 0 {
		t.Fatalf("groups = %+v, want none", groups)
	}
}
