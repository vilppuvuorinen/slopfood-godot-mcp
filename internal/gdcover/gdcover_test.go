package gdcover

import (
	"reflect"
	"strings"
	"testing"
)

func TestInstrumentInjectsStatementAndBranchHits(t *testing.T) {
	src := `extends Node

func _init():
    var x = 1
    if x:
        print(x)
    else:
        pass
    return
`
	result, err := Instrument([]byte(src))
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	got := string(result.Source)
	want := []string{
		"func _init():\n    __gdcover_hit(3)\n    var x = 1\n    __gdcover_hit(4)\n",
		"if x:\n        __gdcover_hit(5)\n        print(x)\n        __gdcover_hit(6)\n",
		"else:\n        __gdcover_hit(7)\n        __gdcover_hit(8)\n        pass\n",
		"    __gdcover_hit(9)\n    return\n",
	}
	for _, fragment := range want {
		if !strings.Contains(got, fragment) {
			t.Errorf("instrumented output missing:\n%s\ngot:\n%s", fragment, got)
		}
	}
	if n := strings.Count(got, "func "+HitFunc+"("); n != 1 {
		t.Errorf("expected one injected helper, found %d", n)
	}
	if want := []int{3, 4, 5, 6, 7, 8, 9}; !reflect.DeepEqual(result.Lines, want) {
		t.Errorf("instrumentable lines = %v, want %v", result.Lines, want)
	}
}

func TestInstrumentSkipsClassLevelAndAnnotations(t *testing.T) {
	src := `class_name Foo
extends Node

signal ready_signal
const LIMIT = 1
@export var exported = 1
var member = 2

func _ready():
    @warning_ignore("unused_variable")
    var local = 3
    print(local)
`
	result, err := Instrument([]byte(src))
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	text := string(result.Source)
	for _, fragment := range []string{
		"    __gdcover_hit(9)\n    @warning_ignore",
		"var local = 3\n    __gdcover_hit(11)",
		"print(local)\n    __gdcover_hit(12)",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("instrumented output missing:\n%s\ngot:\n%s", fragment, text)
		}
	}
	for _, forbidden := range []string{
		"__gdcover_hit(4)", "__gdcover_hit(5)",
		"__gdcover_hit(6)", "__gdcover_hit(7)",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("class-level line was instrumented: %s", forbidden)
		}
	}
}

func TestInstrumentGroupsContinuedLines(t *testing.T) {
	src := `extends Node

func _init():
    var values = [
        1,
        2,
    ]
    var total = values[0] + \
        values[1]
    print(total)
`
	result, err := Instrument([]byte(src))
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	text := string(result.Source)
	for _, fragment := range []string{
		"    __gdcover_hit(3)\n    var values = [",
		"    ]\n    __gdcover_hit(4)",
		"values[1]\n    __gdcover_hit(8)",
		"print(total)\n    __gdcover_hit(10)",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("instrumented output missing:\n%s\ngot:\n%s", fragment, text)
		}
	}
	for _, forbidden := range []string{"__gdcover_hit(5)", "__gdcover_hit(6)", "__gdcover_hit(9)"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("continuation line was instrumented: %s", forbidden)
		}
	}
}

func TestInstrumentUsesTabsWhenScriptDoes(t *testing.T) {
	src := "extends Node\n\nfunc _init():\n\tvar x = 1\n"
	result, err := Instrument([]byte(src))
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	text := string(result.Source)
	if !strings.Contains(text, "\tif __gdcover_path.is_empty():") {
		t.Errorf("helper should be tab-indented:\n%s", text)
	}
	if !strings.Contains(text, "\t__gdcover_hit(4)") {
		t.Errorf("hit should be tab-indented:\n%s", text)
	}
}

func TestInstrumentEnvNamesItsOutputVariable(t *testing.T) {
	src := []byte("extends Node\n\nfunc _init():\n    pass\n")

	def, err := Instrument(src)
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	if !strings.Contains(string(def.Source), `OS.get_environment("`+FileEnv+`")`) {
		t.Errorf("default prologue does not read %s:\n%s", FileEnv, def.Source)
	}

	custom, err := InstrumentEnv(src, "GDCOVER_OTHER_FILE")
	if err != nil {
		t.Fatalf("InstrumentEnv: %v", err)
	}
	if !strings.Contains(string(custom.Source), `OS.get_environment("GDCOVER_OTHER_FILE")`) {
		t.Errorf("custom prologue does not read its variable:\n%s", custom.Source)
	}
	if strings.Contains(string(custom.Source), FileEnv) {
		t.Errorf("custom prologue still references %s:\n%s", FileEnv, custom.Source)
	}
}

func TestInstrumentRejectsDoubleInstrumentation(t *testing.T) {
	src := []byte("extends Node\n\nfunc _init():\n    pass\n")
	once, err := Instrument(src)
	if err != nil {
		t.Fatalf("first Instrument: %v", err)
	}
	if _, err := Instrument(once.Source); err == nil {
		t.Fatal("second Instrument: want error, got nil")
	}
}
