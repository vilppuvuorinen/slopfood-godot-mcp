package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLCOVWritesReport(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "script.gd")
	if err := os.WriteFile(src, []byte("extends Node\n\nfunc _ready():\n    var x = 1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	hits := filepath.Join(dir, "hits.txt")
	if err := os.WriteFile(hits, []byte("4\n"), 0o600); err != nil {
		t.Fatalf("write hits: %v", err)
	}
	out := filepath.Join(dir, "out.info")

	var stdout, stderr strings.Builder
	if err := run([]string{"lcov", "-source", src, "-hits", hits, "-out", out}, &stdout, &stderr); err != nil {
		t.Fatalf("run lcov: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	for _, want := range []string{"SF:" + src, "DA:4,1", "LF:2", "LH:1"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("report missing %q:\n%s", want, data)
		}
	}
	if !strings.Contains(stderr.String(), "gdcover: 1/2 lines") {
		t.Errorf("summary = %q", stderr.String())
	}
}

func TestRunLCOVTreatsMissingHitsAsEmpty(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "script.gd")
	if err := os.WriteFile(src, []byte("extends Node\n\nfunc _ready():\n    var x = 1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	var stdout, stderr strings.Builder
	if err := run([]string{"lcov", "-source", src, "-hits", filepath.Join(dir, "absent.txt")}, &stdout, &stderr); err != nil {
		t.Fatalf("run lcov: %v", err)
	}
	if !strings.Contains(stdout.String(), "LH:0") {
		t.Errorf("report should show no hits:\n%s", stdout.String())
	}
}

func TestRunInstrument(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "script.gd")
	if err := os.WriteFile(src, []byte("extends Node\n\nfunc _ready():\n    var x = 1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out := filepath.Join(dir, "instrumented.gd")
	var stdout, stderr strings.Builder
	if err := run([]string{"instrument", "-in", src, "-out", out}, &stdout, &stderr); err != nil {
		t.Fatalf("run instrument: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read instrumented: %v", err)
	}
	if !strings.Contains(string(data), "__gdcover_hit(4)") {
		t.Errorf("instrumented output missing a hit:\n%s", data)
	}
}
