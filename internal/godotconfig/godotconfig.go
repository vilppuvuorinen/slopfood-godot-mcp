// Copyright (c) 2014-present Godot Engine contributors (see AUTHORS.md).
// Copyright (c) 2007-2014 Juan Linietsky, Ariel Manzur.
//
// This file is a Go port of Godot Engine source. Godot is distributed under
// the MIT License; see THIRD_PARTY_NOTICES in the repository root for the
// full license text.

// Package godotconfig parses and serializes Godot's ConfigFile format, the
// text used by project.godot and export_presets.cfg.
//
// It is a Go port of the subset of Godot (4.4) needed to round-trip those
// files. The upstream sources to diff against when syncing are:
//
//	core/io/config_file.cpp         ConfigFile::_parse, _internal_save
//	core/variant/variant_parser.cpp VariantParser::get_token, parse_value,
//	                                _parse_array, _parse_dictionary,
//	                                _parse_tag, parse_tag_assign_eof
//	                                VariantWriter::write
//	core/string/ustring.cpp         String::c_escape, c_escape_multiline,
//	                                property_name_encode, num_scientific
//
// Supported values: null, bool, int, float, String, StringName, Array,
// Dictionary and PackedStringArray. Every other Variant constructor
// (Object, Resource, Vector*, Color, NodePath, RID, Signal, ...) is captured
// verbatim as an Opaque value so files that contain them (notably the
// Object(InputEventKey, ...) events in project.godot's [input] section) still
// round-trip. Opaque is opaque on purpose: its text is never interpreted, only
// re-emitted.
package godotconfig

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// EscapeString escapes a value for a double-quoted Godot string, mirroring
// String::c_escape_multiline (core/string/ustring.cpp). Only backslashes and
// double quotes are escaped; literal newlines are legal inside a quoted string.
func EscapeString(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// EscapeStringFull escapes a value the way String::c_escape does, including
// control characters. PackedStringArray and StringName use this form.
func EscapeStringFull(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\v':
			b.WriteString(`\v`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// UnescapeString decodes the escape sequences VariantParser::get_token accepts
// in a quoted string. It is the inverse of EscapeString/EscapeStringFull.
func UnescapeString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			b.WriteByte('\\')
			break
		}
		switch s[i] {
		case 'b':
			b.WriteByte(8)
		case 't':
			b.WriteByte(9)
		case 'n':
			b.WriteByte(10)
		case 'f':
			b.WriteByte(12)
		case 'r':
			b.WriteByte(13)
		case 'u', 'U':
			hexLen := 4
			if s[i] == 'U' {
				hexLen = 6
			}
			if i+hexLen >= len(s) {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				continue
			}
			v, err := strconv.ParseUint(s[i+1:i+1+hexLen], 16, 32)
			if err != nil || v > unicode.MaxRune {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				continue
			}
			b.WriteRune(rune(v))
			i += hexLen
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// EncodePropertyName encodes a config key, mirroring String::property_name_encode
// (core/string/ustring.cpp): keys containing '=', '"', ';', '[', ']', a control
// character or non-ASCII are quoted and escaped.
func EncodePropertyName(s string) string {
	for _, r := range s {
		if r == '=' || r == '"' || r == ';' || r == '[' || r == ']' || r < 33 || r > 126 {
			return `"` + EscapeString(s) + `"`
		}
	}
	return s
}

// EscapeSectionName escapes a section header, mirroring ConfigFile::_internal_save
// (core/io/config_file.cpp), which escapes only ']'. A backslash is written
// literally, and the parser treats the character after any backslash as escaped,
// so a name ending in a backslash produces a tag whose closing bracket never
// terminates; Godot itself cannot round-trip such a name. A caller that accepts
// arbitrary section names must reject that case rather than try to escape it.
func EscapeSectionName(s string) string {
	return strings.ReplaceAll(s, "]", `\]`)
}

// UnescapeSectionName reverses EscapeSectionName, mirroring ConfigFile::_parse.
func UnescapeSectionName(s string) string {
	return strings.ReplaceAll(s, `\]`, "]")
}

// Value is one Variant. The set of implementations is closed; use the types
// below.
type Value interface {
	write(b *strings.Builder)
}

// Nil is the null/nil Variant.
type Nil struct{}

func (Nil) write(b *strings.Builder) { b.WriteString("null") }

// Bool is the bool Variant.
type Bool bool

func (v Bool) write(b *strings.Builder) {
	if v {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
}

// Int is the int Variant.
type Int int64

func (v Int) write(b *strings.Builder) {
	b.WriteString(strconv.FormatInt(int64(v), 10))
}

// Float is the float Variant.
type Float float64

func (v Float) write(b *strings.Builder) {
	b.WriteString(formatFloat(float64(v)))
}

// String is the String Variant.
type String string

func (v String) write(b *strings.Builder) {
	b.WriteByte('"')
	b.WriteString(EscapeString(string(v)))
	b.WriteByte('"')
}

// StringName is the StringName Variant, written as &"...".
type StringName string

func (v StringName) write(b *strings.Builder) {
	b.WriteString(`&"`)
	b.WriteString(EscapeStringFull(string(v)))
	b.WriteByte('"')
}

// Array is the Array Variant.
type Array []Value

func (v Array) write(b *strings.Builder) {
	b.WriteByte('[')
	for i, e := range v {
		if i > 0 {
			b.WriteString(", ")
		}
		writeValue(b, e)
	}
	b.WriteByte(']')
}

// DictEntry is one key/value pair of a Dictionary.
type DictEntry struct {
	Key   Value
	Value Value
}

// Dictionary is the Dictionary Variant.
type Dictionary struct {
	Entries []DictEntry
}

func (v Dictionary) write(b *strings.Builder) {
	if len(v.Entries) == 0 {
		b.WriteString("{}")
		return
	}
	// NOTE: VariantWriter sorts keys with StringLikeVariantOrder
	// (core/variant/variant.h) before writing. We preserve insertion order;
	// for string keys this differs only when they are inserted out of lexical
	// order.
	b.WriteString("{\n")
	for i, e := range v.Entries {
		writeValue(b, e.Key)
		b.WriteString(": ")
		writeValue(b, e.Value)
		if i < len(v.Entries)-1 {
			b.WriteString(",\n")
		} else {
			b.WriteString("\n")
		}
	}
	b.WriteString("}")
}

// PackedStringArray is the PackedStringArray Variant.
type PackedStringArray []string

func (v PackedStringArray) write(b *strings.Builder) {
	b.WriteString("PackedStringArray(")
	for i, s := range v {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('"')
		b.WriteString(EscapeStringFull(s))
		b.WriteByte('"')
	}
	b.WriteByte(')')
}

// Opaque is a Variant constructor the parser does not model, kept as its
// verbatim source text (for example `Object(InputEventKey,...)`). It is
// written back unchanged, so parsing and re-serializing a value that contains
// one never rewrites it.
type Opaque string

func (v Opaque) write(b *strings.Builder) { b.WriteString(string(v)) }

func writeValue(b *strings.Builder, v Value) {
	if v == nil {
		b.WriteString("null")
		return
	}
	v.write(b)
}

// formatFloat mirrors VariantWriter's float case: rtos_fix (core/variant/
// variant_parser.cpp) delegates to String::num_scientific, i.e. C printf
// "%lg" (6 significant digits), then a trailing ".0" is added when the result
// has no '.', 'e' or 'E'.
func formatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "inf_neg"
	}
	s := strconv.FormatFloat(f, 'g', 6, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// Config is a parsed ConfigFile, preserving section and key order. Keys read
// before any section header live in the section with an empty name.
type Config struct {
	Sections []Section
}

// Section is one "[name]" block; an empty Name is the section-less prefix.
type Section struct {
	Name string
	Keys []KeyValue
}

// KeyValue is one key/value pair within a Section.
type KeyValue struct {
	Key   string
	Value Value
}

// Parse reads a ConfigFile document.
func Parse(data string) (*Config, error) {
	s := newScanner(data)
	cfg := &Config{}
	section := ""
	for {
		name, assign, value, err := s.parseTagAssignEOF()
		if errors.Is(err, io.EOF) {
			return cfg, nil
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", s.line, err)
		}
		switch {
		case assign != "":
			cfg.Set(section, assign, value)
		case name != "":
			section = UnescapeSectionName(name)
		}
	}
}

// Encode serializes the Config back to ConfigFile text.
func (c *Config) Encode() string {
	var b strings.Builder
	for i, sec := range c.Sections {
		if i > 0 {
			b.WriteByte('\n')
		}
		if sec.Name != "" {
			b.WriteByte('[')
			b.WriteString(EscapeSectionName(sec.Name))
			b.WriteString("]\n\n")
		}
		for _, kv := range sec.Keys {
			b.WriteString(EncodePropertyName(kv.Key))
			b.WriteByte('=')
			writeValue(&b, kv.Value)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// ParseValue parses a single Variant literal, for example `0.5`, `"hi"` or
// `{"deadzone": 0.5}`. Leading and trailing whitespace and comments are
// ignored; any other trailing text is an error.
func ParseValue(text string) (Value, error) {
	s := newScanner(text)
	tok, err := s.getToken()
	if err != nil {
		return nil, err
	}
	v, err := s.parseValue(tok)
	if err != nil {
		return nil, err
	}
	tok, err = s.getToken()
	if err != nil {
		return nil, err
	}
	if tok.kind != tkEOF {
		return nil, fmt.Errorf("unexpected trailing content %s", tok.kind)
	}
	return v, nil
}

// FormatValue serializes a single Variant the way VariantWriter would, without
// a trailing newline.
func FormatValue(v Value) string {
	var b strings.Builder
	writeValue(&b, v)
	return b.String()
}

func (c *Config) section(name string) *Section {
	for i := range c.Sections {
		if c.Sections[i].Name == name {
			return &c.Sections[i]
		}
	}
	return nil
}

// Get returns the value for a section/key pair.
func (c *Config) Get(section, key string) (Value, bool) {
	sec := c.section(section)
	if sec == nil {
		return nil, false
	}
	for _, kv := range sec.Keys {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return nil, false
}

// Set stores a value, creating the section and overwriting an existing key.
//
// Deviation: ConfigFile::set_value treats a Nil value as "erase this key". We
// store it instead so that `key=null` survives a parse/encode round trip.
func (c *Config) Set(section, key string, value Value) {
	sec := c.section(section)
	if sec == nil {
		c.Sections = append(c.Sections, Section{Name: section})
		sec = &c.Sections[len(c.Sections)-1]
	}
	for i := range sec.Keys {
		if sec.Keys[i].Key == key {
			sec.Keys[i].Value = value
			return
		}
	}
	sec.Keys = append(sec.Keys, KeyValue{Key: key, Value: value})
}

// Delete removes a key, reporting whether it existed.
func (c *Config) Delete(section, key string) bool {
	sec := c.section(section)
	if sec == nil {
		return false
	}
	for i := range sec.Keys {
		if sec.Keys[i].Key == key {
			sec.Keys = append(sec.Keys[:i], sec.Keys[i+1:]...)
			return true
		}
	}
	return false
}

// DeleteSection removes a whole section, reporting whether it existed.
func (c *Config) DeleteSection(section string) bool {
	for i := range c.Sections {
		if c.Sections[i].Name == section {
			c.Sections = append(c.Sections[:i], c.Sections[i+1:]...)
			return true
		}
	}
	return false
}

// SectionKeys returns a copy of a section's keys.
func (c *Config) SectionKeys(section string) []KeyValue {
	sec := c.section(section)
	if sec == nil {
		return nil
	}
	out := make([]KeyValue, len(sec.Keys))
	copy(out, sec.Keys)
	return out
}

// tokenKind mirrors VariantParser::TokenType (core/variant/variant_parser.h).
type tokenKind int

const (
	tkEOF tokenKind = iota
	tkCurlyOpen
	tkCurlyClose
	tkBracketOpen
	tkBracketClose
	tkParenOpen
	tkParenClose
	tkColon
	tkComma
	tkPeriod
	tkEqual
	tkIdentifier
	tkString
	tkStringName
	tkNumber
	tkColor
	tkError
)

var tokenKindNames = [...]string{
	"EOF", "'{'", "'}'", "'['", "']'", "'('", "')'", "':'", "','", "'.'", "'='",
	"identifier", "string", "string_name", "number", "color", "ERROR",
}

func (k tokenKind) String() string {
	if int(k) < len(tokenKindNames) {
		return tokenKindNames[k]
	}
	return "ERROR"
}

type token struct {
	kind    tokenKind
	text    string
	i       int64
	f       float64
	isFloat bool
}

// scanner is a rune stream with one character of pushback, mirroring
// VariantParser::Stream plus the Stream::saved slot.
type scanner struct {
	src      []rune
	pos      int
	saved    rune
	hasSaved bool
	line     int
}

func newScanner(data string) *scanner {
	return &scanner{src: []rune(data), line: 1}
}

func (s *scanner) getChar() (rune, bool) {
	if s.hasSaved {
		s.hasSaved = false
		return s.saved, true
	}
	if s.pos >= len(s.src) {
		return 0, false
	}
	c := s.src[s.pos]
	s.pos++
	return c, true
}

func (s *scanner) unget(c rune) {
	s.saved = c
	s.hasSaved = true
}

// getToken mirrors VariantParser::get_token for the supported subset.
func (s *scanner) getToken() (token, error) {
	stringName := false
	for {
		c, ok := s.getChar()
		if !ok {
			return token{kind: tkEOF}, nil
		}
		switch c {
		case '\n':
			s.line++
		case 0:
			return token{kind: tkEOF}, nil
		case '{':
			return token{kind: tkCurlyOpen}, nil
		case '}':
			return token{kind: tkCurlyClose}, nil
		case '[':
			return token{kind: tkBracketOpen}, nil
		case ']':
			return token{kind: tkBracketClose}, nil
		case '(':
			return token{kind: tkParenOpen}, nil
		case ')':
			return token{kind: tkParenClose}, nil
		case ':':
			return token{kind: tkColon}, nil
		case ',':
			return token{kind: tkComma}, nil
		case '.':
			return token{kind: tkPeriod}, nil
		case '=':
			return token{kind: tkEqual}, nil
		case ';':
			for {
				ch, ok := s.getChar()
				if !ok {
					return token{kind: tkEOF}, nil
				}
				if ch == '\n' {
					s.line++
					break
				}
			}
		case '#':
			var b strings.Builder
			b.WriteRune('#')
			for {
				ch, ok := s.getChar()
				if !ok {
					return token{kind: tkEOF}, nil
				}
				if !isHexDigit(ch) {
					s.unget(ch)
					break
				}
				b.WriteRune(ch)
			}
			return token{kind: tkColor, text: b.String()}, nil
		case '&':
			next, ok := s.getChar()
			if !ok || next != '"' {
				return token{}, errors.New(`expected '"' after '&'`)
			}
			stringName = true
			fallthrough
		case '"':
			str, err := s.parseQuotedString()
			if err != nil {
				return token{}, err
			}
			if stringName {
				return token{kind: tkStringName, text: str}, nil
			}
			return token{kind: tkString, text: str}, nil
		default:
			if c <= 32 {
				continue
			}
			if c == '-' || isDigit(c) {
				return s.readNumber(c)
			}
			if isASCIIAlpha(c) || c == '_' {
				return s.readIdentifier(c), nil
			}
			return token{}, fmt.Errorf("unexpected character %q", c)
		}
	}
}

// readNumber mirrors the number branch of VariantParser::get_token.
func (s *scanner) readNumber(c rune) (token, error) {
	const (
		readingInt = iota
		readingDec
		readingExp
		readingDone
	)

	var num strings.Builder
	if c == '-' {
		num.WriteRune('-')
		next, ok := s.getChar()
		if !ok {
			return token{}, errors.New("unexpected EOF in number")
		}
		c = next
	}

	reading := readingInt
	isFloat := false
	expSign := false
	expBeg := false
	atEOF := false
	for !atEOF {
		switch reading {
		case readingInt:
			switch {
			case isDigit(c):
			case c == '.':
				reading = readingDec
				isFloat = true
			case c == 'e' || c == 'E':
				reading = readingExp
				isFloat = true
			default:
				reading = readingDone
			}
		case readingDec:
			switch {
			case isDigit(c):
			case c == 'e' || c == 'E':
				reading = readingExp
			default:
				reading = readingDone
			}
		case readingExp:
			switch {
			case isDigit(c):
				expBeg = true
			case (c == '-' || c == '+') && !expSign && !expBeg:
				expSign = true
			default:
				reading = readingDone
			}
		case readingDone:
		}
		if reading == readingDone {
			break
		}
		num.WriteRune(c)
		var ok bool
		c, ok = s.getChar()
		atEOF = !ok
	}
	if !atEOF {
		s.unget(c)
	}

	text := num.String()
	if isFloat {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return token{}, fmt.Errorf("parse number %q: %w", text, err)
		}
		return token{kind: tkNumber, f: f, isFloat: true}, nil
	}
	i, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return token{}, fmt.Errorf("parse number %q: %w", text, err)
	}
	return token{kind: tkNumber, i: i}, nil
}

// readIdentifier mirrors the identifier branch of VariantParser::get_token.
func (s *scanner) readIdentifier(c rune) token {
	var b strings.Builder
	b.WriteRune(c)
	for {
		next, ok := s.getChar()
		if !ok {
			break
		}
		if isASCIIAlpha(next) || isDigit(next) || next == '_' {
			b.WriteRune(next)
			continue
		}
		s.unget(next)
		break
	}
	return token{kind: tkIdentifier, text: b.String()}
}

// parseQuotedString mirrors the string branch of VariantParser::get_token,
// including \u/\U and UTF-16 surrogate pairs.
func (s *scanner) parseQuotedString() (string, error) {
	var b strings.Builder
	var prev rune
	for {
		c, ok := s.getChar()
		if !ok || c == 0 {
			return "", errors.New("unterminated string")
		}
		if c == '"' {
			break
		}
		if c != '\\' {
			if prev != 0 {
				return "", errors.New("invalid UTF-16 sequence in string, unpaired lead surrogate")
			}
			if c == '\n' {
				s.line++
			}
			b.WriteRune(c)
			continue
		}

		next, ok := s.getChar()
		if !ok || next == 0 {
			return "", errors.New("unterminated string")
		}
		var res rune
		switch next {
		case 'b':
			res = 8
		case 't':
			res = 9
		case 'n':
			res = 10
		case 'f':
			res = 12
		case 'r':
			res = 13
		case 'U', 'u':
			hexLen := 4
			if next == 'U' {
				hexLen = 6
			}
			var v rune
			for j := 0; j < hexLen; j++ {
				hc, ok := s.getChar()
				if !ok || hc == 0 {
					return "", errors.New("unterminated string")
				}
				if !isHexDigit(hc) {
					return "", errors.New("malformed hex constant in string")
				}
				v = v<<4 | hexVal(hc)
			}
			res = v
		default:
			res = next
		}

		//nolint:gosec // res is always a non-negative code point here.
		code := uint32(res)
		switch code & 0xfffffc00 {
		case 0xd800:
			if prev == 0 {
				prev = res
				continue
			}
			return "", errors.New("invalid UTF-16 sequence in string, unpaired lead surrogate")
		case 0xdc00:
			if prev == 0 {
				return "", errors.New("invalid UTF-16 sequence in string, unpaired trail surrogate")
			}
			//nolint:gosec // the surrogate-pair math yields a valid code point.
			res = rune((uint32(prev) << 10) + code - ((0xd800 << 10) + 0xdc00 - 0x10000))
			prev = 0
		}
		if prev != 0 {
			return "", errors.New("invalid UTF-16 sequence in string, unpaired lead surrogate")
		}
		b.WriteRune(res)
	}
	if prev != 0 {
		return "", errors.New("invalid UTF-16 sequence in string, unpaired lead surrogate")
	}
	return b.String(), nil
}

// parseValue mirrors VariantParser::parse_value for the supported subset.
func (s *scanner) parseValue(tok token) (Value, error) {
	switch tok.kind {
	case tkCurlyOpen:
		return s.parseDictionary()
	case tkBracketOpen:
		return s.parseArray()
	case tkIdentifier:
		switch tok.text {
		case "true":
			return Bool(true), nil
		case "false":
			return Bool(false), nil
		case "null", "nil":
			return Nil{}, nil
		case "inf":
			return Float(math.Inf(1)), nil
		case "inf_neg":
			return Float(math.Inf(-1)), nil
		case "nan":
			return Float(math.NaN()), nil
		case "PackedStringArray", "PoolStringArray", "StringArray":
			return s.parsePackedStringArray()
		default:
			return s.parseConstructor(tok.text)
		}
	case tkNumber:
		if tok.isFloat {
			return Float(tok.f), nil
		}
		return Int(tok.i), nil
	case tkString:
		return String(tok.text), nil
	case tkStringName:
		return StringName(tok.text), nil
	case tkEOF, tkCurlyClose, tkBracketClose, tkParenOpen, tkParenClose,
		tkColon, tkComma, tkPeriod, tkEqual, tkColor, tkError:
		return nil, fmt.Errorf("expected value, got %s", tok.kind)
	}
	return nil, fmt.Errorf("expected value, got %s", tok.kind)
}

// parseArray mirrors VariantParser::_parse_array.
func (s *scanner) parseArray() (Array, error) {
	arr := Array{}
	needComma := false
	for {
		tok, err := s.getToken()
		if err != nil {
			return nil, err
		}
		if tok.kind == tkEOF {
			return nil, errors.New("unexpected EOF while parsing array")
		}
		if tok.kind == tkBracketClose {
			return arr, nil
		}
		if needComma {
			if tok.kind != tkComma {
				return nil, errors.New("expected ','")
			}
			needComma = false
			continue
		}
		v, err := s.parseValue(tok)
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		needComma = true
	}
}

// parseDictionary mirrors VariantParser::_parse_dictionary.
func (s *scanner) parseDictionary() (Dictionary, error) {
	dict := Dictionary{}
	atKey := true
	needComma := false
	var key Value
	for {
		tok, err := s.getToken()
		if err != nil {
			return Dictionary{}, err
		}
		if tok.kind == tkEOF {
			return Dictionary{}, errors.New("unexpected EOF while parsing dictionary")
		}
		if atKey {
			if tok.kind == tkCurlyClose {
				return dict, nil
			}
			if needComma {
				if tok.kind != tkComma {
					return Dictionary{}, errors.New("expected '}' or ','")
				}
				needComma = false
				continue
			}
			key, err = s.parseValue(tok)
			if err != nil {
				return Dictionary{}, err
			}
			colon, colonErr := s.getToken()
			if colonErr != nil {
				return Dictionary{}, colonErr
			}
			if colon.kind != tkColon {
				return Dictionary{}, errors.New("expected ':'")
			}
			atKey = false
			continue
		}
		v, err := s.parseValue(tok)
		if err != nil {
			return Dictionary{}, err
		}
		dict.Entries = append(dict.Entries, DictEntry{Key: key, Value: v})
		needComma = true
		atKey = true
	}
}

// parsePackedStringArray mirrors the PackedStringArray branch of parse_value.
func (s *scanner) parsePackedStringArray() (PackedStringArray, error) {
	open, err := s.getToken()
	if err != nil {
		return nil, err
	}
	if open.kind != tkParenOpen {
		return nil, errors.New("expected '('")
	}
	var out PackedStringArray
	first := true
	for {
		if !first {
			tok, err := s.getToken()
			if err != nil {
				return nil, err
			}
			switch tok.kind {
			case tkComma:
			case tkParenClose:
				return out, nil
			case tkEOF, tkCurlyOpen, tkCurlyClose, tkBracketOpen, tkBracketClose,
				tkParenOpen, tkColon, tkPeriod, tkEqual, tkIdentifier, tkString,
				tkStringName, tkNumber, tkColor, tkError:
				return nil, errors.New("expected ',' or ')'")
			}
		}
		tok, err := s.getToken()
		if err != nil {
			return nil, err
		}
		if tok.kind == tkParenClose {
			return out, nil
		}
		if tok.kind != tkString {
			return nil, errors.New("expected string")
		}
		out = append(out, tok.text)
		first = false
	}
}

// parseConstructor captures an unsupported `Name(...)` Variant constructor as
// its verbatim source text. It balances nested parentheses and copies quoted
// strings whole, so `Object(InputEventKey, "resource_name": "a)b", ...)` is
// captured intact.
func (s *scanner) parseConstructor(name string) (Value, error) {
	open, ok := s.getChar()
	if !ok || open != '(' {
		return nil, fmt.Errorf("unexpected identifier %q", name)
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('(')
	depth := 1
	for {
		c, ok := s.getChar()
		if !ok || c == 0 {
			return nil, fmt.Errorf("unexpected EOF in %s constructor", name)
		}
		switch c {
		case '"':
			b.WriteRune(c)
			if err := s.copyQuoted(&b); err != nil {
				return nil, err
			}
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				b.WriteByte(')')
				return Opaque(b.String()), nil
			}
		case '\n':
			s.line++
		}
		b.WriteRune(c)
	}
}

// copyQuoted copies the remainder of a quoted string, starting after its
// opening quote, into b, including the closing quote.
func (s *scanner) copyQuoted(b *strings.Builder) error {
	for {
		c, ok := s.getChar()
		if !ok || c == 0 {
			return errors.New("unterminated string")
		}
		b.WriteRune(c)
		if c == '\\' {
			next, ok := s.getChar()
			if !ok || next == 0 {
				return errors.New("unterminated string")
			}
			b.WriteRune(next)
			if next == '\n' {
				s.line++
			}
			continue
		}
		if c == '"' {
			return nil
		}
		if c == '\n' {
			s.line++
		}
	}
}

// parseTagAssignEOF mirrors VariantParser::parse_tag_assign_eof with
// simple_tag=true. io.EOF reports the end of the document.
func (s *scanner) parseTagAssignEOF() (string, string, Value, error) {
	var what strings.Builder
	for {
		c, ok := s.getChar()
		if !ok {
			return "", "", nil, io.EOF
		}
		switch {
		case c == ';':
			for {
				ch, ok := s.getChar()
				if !ok {
					return "", "", nil, io.EOF
				}
				if ch == '\n' {
					s.line++
					break
				}
			}
		case c == '[' && what.Len() == 0:
			s.unget('[')
			name, err := s.parseTagSimple()
			if err != nil {
				return "", "", nil, err
			}
			return name, "", nil, nil
		case c > 32:
			switch {
			case c == '"':
				s.unget('"')
				tok, err := s.getToken()
				if err != nil {
					return "", "", nil, err
				}
				if tok.kind != tkString {
					return "", "", nil, errors.New("error reading quoted string")
				}
				what.Reset()
				what.WriteString(tok.text)
			case c != '=':
				what.WriteRune(c)
			default:
				tok, err := s.getToken()
				if err != nil {
					return "", "", nil, err
				}
				value, err := s.parseValue(tok)
				if err != nil {
					return "", "", nil, err
				}
				return "", what.String(), value, nil
			}
		case c == '\n':
			s.line++
		}
	}
}

// parseTagSimple mirrors the simple-tag path of VariantParser::_parse_tag,
// used for ConfigFile section headers.
func (s *scanner) parseTagSimple() (string, error) {
	c, ok := s.getChar()
	if !ok || c != '[' {
		return "", errors.New("expected '['")
	}
	var b strings.Builder
	escaping := false
	for {
		c, ok := s.getChar()
		if !ok {
			return "", errors.New("unexpected EOF while parsing section tag")
		}
		if c == ']' && !escaping {
			break
		}
		if c == '\\' {
			escaping = true
		} else {
			escaping = false
		}
		b.WriteRune(c)
	}
	return strings.TrimSpace(b.String()), nil
}

func isDigit(c rune) bool      { return c >= '0' && c <= '9' }
func isASCIIAlpha(c rune) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isHexDigit(c rune) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexVal(c rune) rune {
	switch {
	case isDigit(c):
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}
