package godotconfig

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestEscapeString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "plain", want: "plain"},
		{in: `a"b`, want: `a\"b`},
		{in: `a\b`, want: `a\\b`},
		{in: "a\nb", want: "a\nb"},
	}
	for _, tt := range tests {
		if got := EscapeString(tt.in); got != tt.want {
			t.Errorf("EscapeString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEscapeStringFull(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "plain", want: "plain"},
		{in: `a"b`, want: `a\"b`},
		{in: "a\nb", want: `a\nb`},
		{in: "a\tb", want: `a\tb`},
		{in: `a'b`, want: `a\'b`},
	}
	for _, tt := range tests {
		if got := EscapeStringFull(tt.in); got != tt.want {
			t.Errorf("EscapeStringFull(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnescapeString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "plain", want: "plain"},
		{in: `a\"b`, want: `a"b`},
		{in: `a\\b`, want: `a\b`},
		{in: `a\nb`, want: "a\nb"},
		{in: `\u0041`, want: "A"},
		{in: `\U01F600`, want: "\U0001F600"},
	}
	for _, tt := range tests {
		if got := UnescapeString(tt.in); got != tt.want {
			t.Errorf("UnescapeString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEncodePropertyName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "render", want: "render"},
		{in: "layer_names/2d_render/layer_1", want: "layer_names/2d_render/layer_1"},
		{in: `evil"`, want: `"evil\""`},
		{in: "line\nbreak", want: "\"line\nbreak\""},
		{in: "a=b", want: `"a=b"`},
		{in: "a[b]", want: `"a[b]"`},
		{in: "with space", want: `"with space"`},
		{in: "café", want: `"café"`},
	}
	for _, tt := range tests {
		if got := EncodePropertyName(tt.in); got != tt.want {
			t.Errorf("EncodePropertyName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{in: 0, want: "0.0"},
		{in: 1, want: "1.0"},
		{in: 0.5, want: "0.5"},
		{in: 100000, want: "100000.0"},
		{in: 1000000, want: "1e+06"},
		{in: 1.0 / 3.0, want: "0.333333"},
		{in: math.Inf(1), want: "inf"},
		{in: math.Inf(-1), want: "inf_neg"},
		{in: math.NaN(), want: "nan"},
	}
	for _, tt := range tests {
		if got := formatFloat(tt.in); got != tt.want {
			t.Errorf("formatFloat(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseValues(t *testing.T) {
	src := strings.Join([]string{
		"config_version=5",
		"flag=true",
		"ratio=0.25",
		"missing=null",
		`name="My \"Game\""`,
		`label=&"hero"`,
		`features=PackedStringArray("4.4", "GL Compatibility")`,
		`nums=[1, 2, 3]`,
		"nested={\n\"deadzone\": 0.5,\n\"events\": []\n}",
		"",
	}, "\n")

	cfg, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if v, ok := cfg.Get("", "config_version"); !ok || v != Int(5) {
		t.Errorf("config_version = %#v, %v", v, ok)
	}
	if v, ok := cfg.Get("", "flag"); !ok || v != Bool(true) {
		t.Errorf("flag = %#v, %v", v, ok)
	}
	if v, ok := cfg.Get("", "ratio"); !ok || v != Float(0.25) {
		t.Errorf("ratio = %#v, %v", v, ok)
	}
	if v, ok := cfg.Get("", "missing"); !ok || v != (Nil{}) {
		t.Errorf("missing = %#v, %v", v, ok)
	}
	if v, ok := cfg.Get("", "name"); !ok || v != String(`My "Game"`) {
		t.Errorf("name = %#v, %v", v, ok)
	}
	if v, ok := cfg.Get("", "label"); !ok || v != StringName("hero") {
		t.Errorf("label = %#v, %v", v, ok)
	}
	if v, ok := cfg.Get("", "features"); !ok {
		t.Fatalf("features missing")
	} else if psa, ok := v.(PackedStringArray); !ok || len(psa) != 2 || psa[0] != "4.4" {
		t.Errorf("features = %#v", v)
	}
	if v, ok := cfg.Get("", "nums"); !ok {
		t.Fatalf("nums missing")
	} else if arr, ok := v.(Array); !ok || len(arr) != 3 || arr[2] != Int(3) {
		t.Errorf("nums = %#v", v)
	}
	if v, ok := cfg.Get("", "nested"); !ok {
		t.Fatalf("nested missing")
	} else if dict, ok := v.(Dictionary); !ok || len(dict.Entries) != 2 || dict.Entries[0].Key != String("deadzone") {
		t.Errorf("nested = %#v", v)
	}
}

func TestParseSectionsAndRoundTrip(t *testing.T) {
	src := strings.Join([]string{
		"config_version=5",
		"",
		"[application]",
		"",
		`config/name="My Game"`,
		`config/features=PackedStringArray("4.4", "GL Compatibility")`,
		"",
		"[input]",
		"",
		"jump={",
		`"deadzone": 0.5,`,
		`"events": []`,
		"}",
		"",
		"[autoload]",
		"",
		`Game="*res://game.gd"`,
		"",
	}, "\n")

	cfg, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Encode(); got != src {
		t.Errorf("round trip mismatch:\n got %q\nwant %q", got, src)
	}
}

func TestParseSectionNameEscaping(t *testing.T) {
	cfg, err := Parse("[a\\]b]\n\nkey=1\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v, ok := cfg.Get("a]b", "key"); !ok || v != Int(1) {
		t.Errorf("a]b/key = %#v, %v", v, ok)
	}
	if got := cfg.Encode(); got != "[a\\]b]\n\nkey=1\n" {
		t.Errorf("Encode = %q", got)
	}
}

// TestKeyInjectionRoundTrip proves a name containing quotes and newlines is
// encoded into a single key and parsed back intact, rather than injecting new
// config entries.
func TestKeyInjectionRoundTrip(t *testing.T) {
	key := "Evil\"\n[application]\nconfig/name=\"pwned"
	cfg := &Config{}
	cfg.Set("autoload", key, String("*res://evil.gd"))

	// The literal "[application]" text stays inside the quoted key; it must not
	// be parsed as a real section.
	encoded := cfg.Encode()

	parsed, err := Parse(encoded)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	v, ok := parsed.Get("autoload", key)
	if !ok || v != String("*res://evil.gd") {
		t.Errorf("round-tripped value = %#v, %v", v, ok)
	}
	if _, ok := parsed.Get("application", "config/name"); ok {
		t.Errorf("injected key was parsed as real config")
	}
}

// TestParseValueRoundTrip checks the single-value helpers used by the tools
// that splice one value into an existing file.
func TestParseValueRoundTrip(t *testing.T) {
	tests := []struct {
		in   string
		want Value
	}{
		{in: "0.5", want: Float(0.5)},
		{in: `"Ground"`, want: String("Ground")},
		{in: `"say \"hi\""`, want: String(`say "hi"`)},
		{in: "true", want: Bool(true)},
		{in: `PackedStringArray("res://en.translation")`, want: PackedStringArray{"res://en.translation"}},
	}
	for _, tt := range tests {
		got, err := ParseValue(tt.in)
		if err != nil {
			t.Errorf("ParseValue(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseValue(%q) = %#v, want %#v", tt.in, got, tt.want)
		}
	}
	if _, err := ParseValue("1 2"); err == nil {
		t.Errorf("ParseValue(%q) = nil error, want trailing-content failure", "1 2")
	}
}

// TestOpaqueConstructorRoundTrip proves an unsupported constructor such as
// Object(InputEventKey, ...) is captured verbatim and re-emitted unchanged,
// including parentheses and quotes inside its arguments.
func TestOpaqueConstructorRoundTrip(t *testing.T) {
	event := `Object(InputEventKey,"resource_name":"a)b\"c","physical_keycode":87,"script":null)`
	v, err := ParseValue(event)
	if err != nil {
		t.Fatalf("ParseValue: %v", err)
	}
	if _, ok := v.(Opaque); !ok {
		t.Fatalf("value = %#v, want Opaque", v)
	}
	if got := FormatValue(v); got != event {
		t.Errorf("FormatValue = %q, want %q", got, event)
	}

	// The whole dictionary an input action uses must round-trip too.
	src := `{"deadzone": 0.5, "events": [` + event + `]}`
	dict, err := ParseValue(src)
	if err != nil {
		t.Fatalf("ParseValue(dict): %v", err)
	}
	got := FormatValue(dict)
	if !strings.Contains(got, event) {
		t.Errorf("re-serialized dictionary lost the event:\n%s", got)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "unterminated string", in: `name="oops`},
		{name: "unterminated array", in: "nums=[1, 2"},
		{name: "bare identifier", in: "value=Vector2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.in); err == nil {
				t.Errorf("Parse(%q) = nil error, want failure", tt.in)
			}
		})
	}
}
