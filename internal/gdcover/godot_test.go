package gdcover_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/gdcover"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
)

// requireGodot returns an engine path or skips the test: the unit-test job runs
// without Godot, while the e2e job supplies one via GODOT_PATH.
func requireGodot(t *testing.T) string {
	t.Helper()
	path := godot.Path()
	if _, err := godot.Version(context.Background(), path); err != nil {
		t.Skipf("Godot unavailable at %q: %v", path, err)
	}
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "project.godot"), "config_version=5\n")
	return dir
}

func readHits(t *testing.T, path string) map[int]bool {
	t.Helper()
	return readHitsFile(t, path)
}

func readHitsFile(t *testing.T, path string) map[int]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hits: %v", err)
	}
	got := map[int]bool{}
	for _, field := range strings.Fields(string(data)) {
		line, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("parse hit %q: %v", field, err)
		}
		got[line] = true
	}
	return got
}

// readHitsGlob merges every trace matched by pattern, tolerating the per-PID
// path the engine derives from GDCOVER_FILE under the gdcover build tag.
func readHitsGlob(t *testing.T, pattern string) map[int]bool {
	t.Helper()
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob hits: %v", err)
	}
	got := map[int]bool{}
	for _, path := range paths {
		for line := range readHitsFile(t, path) {
			got[line] = true
		}
	}
	return got
}

func linesWith(src, marker string) []int {
	lines := make([]int, 0, strings.Count(src, marker))
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, marker) {
			lines = append(lines, i+1)
		}
	}
	return lines
}

func lineOf(t *testing.T, src, substr string) int {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, substr) {
			return i + 1
		}
	}
	t.Fatalf("substring %q not found", substr)
	return 0
}

func runInstrumentedAt(t *testing.T, engine, project, script, hitsPath string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), engine, //nolint:gosec // engine is the test's own binary.
		"--headless", "--path", project, "--script", script)
	cmd.Env = append(os.Environ(), gdcover.FileEnv+"="+hitsPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run instrumented script: %v\n%s", err, out)
	}
}

func runInstrumented(t *testing.T, engine, src string) map[int]bool {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "instrumented.gd")
	writeFile(t, script, src)
	hits := filepath.Join(dir, "hits.txt")
	runInstrumentedAt(t, engine, writeProject(t), script, hits)
	return readHits(t, hits)
}

func TestInstrumentedScriptsAppendAcrossRuns(t *testing.T) {
	engine := requireGodot(t)
	dir := t.TempDir()
	project := writeProject(t)
	hits := filepath.Join(dir, "hits.txt")

	first := `extends SceneTree

func _init():
    print("first")  # first
    quit(0)
`
	second := `extends SceneTree

# A leading comment shifts the body so the two runs report different lines.

func _init():
    print("second")  # second
    quit(0)
`

	firstResult, err := gdcover.Instrument([]byte(first))
	if err != nil {
		t.Fatalf("Instrument first: %v", err)
	}
	secondResult, err := gdcover.Instrument([]byte(second))
	if err != nil {
		t.Fatalf("Instrument second: %v", err)
	}

	firstScript := filepath.Join(dir, "first.gd")
	writeFile(t, firstScript, string(firstResult.Source))
	runInstrumentedAt(t, engine, project, firstScript, hits)

	secondScript := filepath.Join(dir, "second.gd")
	writeFile(t, secondScript, string(secondResult.Source))
	runInstrumentedAt(t, engine, project, secondScript, hits)

	got := readHits(t, hits)
	for _, line := range linesWith(first, "# first") {
		if !got[line] {
			t.Errorf("line %d from the first run was truncated; hits=%v", line, got)
		}
	}
	for _, line := range linesWith(second, "# second") {
		if !got[line] {
			t.Errorf("line %d from the second run is missing; hits=%v", line, got)
		}
	}
}

func TestInstrumentedScriptReportsHits(t *testing.T) {
	engine := requireGodot(t)
	src := `extends SceneTree

func _init():
    var total = 0  # covered
    for i in range(3):  # covered
        total += i  # covered
    if total == 3:  # covered
        print("three")  # covered
    else:  # uncovered
        print("other")  # uncovered
    match total:  # covered
        3:  # covered
            print("matched")  # covered
        _:  # uncovered
            print("fallback")  # uncovered
    print("done")  # covered
    quit(0)  # covered
`
	result, err := gdcover.Instrument([]byte(src))
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	got := runInstrumented(t, engine, string(result.Source))
	for _, line := range linesWith(src, "# covered") {
		if !got[line] {
			t.Errorf("line %d should be covered; hits=%v", line, got)
		}
	}
	for _, line := range linesWith(src, "# uncovered") {
		if got[line] {
			t.Errorf("line %d should not be covered; hits=%v", line, got)
		}
	}
}

func instrumentedOperationsScript(t *testing.T) (path string, original []byte) {
	t.Helper()
	originalPath, err := godotops.OperationsScript()
	if err != nil {
		t.Fatalf("OperationsScript: %v", err)
	}
	original, err = os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read operations script: %v", err)
	}
	result, err := gdcover.Instrument(original)
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	instrPath := filepath.Join(t.TempDir(), "godot_operations_instrumented.gd")
	writeFile(t, instrPath, string(result.Source))
	return instrPath, original
}

func TestInstrumentedOperationsScriptParses(t *testing.T) {
	engine := requireGodot(t)
	script, _ := instrumentedOperationsScript(t)
	cmd := exec.CommandContext(context.Background(), engine, //nolint:gosec // engine is the test's own binary.
		"--headless", "--path", writeProject(t), "--check-only", "--script", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("instrumented bridge failed to parse: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "Parse Error") {
		t.Fatalf("instrumented bridge reported a parse error:\n%s", out)
	}
}

func TestInstrumentedOperationsScriptReportsHits(t *testing.T) {
	engine := requireGodot(t)
	script, original := instrumentedOperationsScript(t)
	hits := filepath.Join(t.TempDir(), "hits.txt")
	t.Setenv(gdcover.FileEnv, hits)

	sess := godot.NewSession(engine, script)
	defer func() { _ = sess.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, stderr, exit, err := sess.Run(ctx, writeProject(t), "validate_project",
		map[string]any{"operation": "validate_project"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exit != 0 {
		t.Fatalf("validate_project exited %d: %s", exit, stderr)
	}

	got := readHitsGlob(t, hits+"*")
	for _, marker := range []string{"func _init():", `"validate_project":`} {
		line := lineOf(t, string(original), marker)
		if !got[line] {
			t.Errorf("line %d (%q) was not reported; hits=%v", line, marker, got)
		}
	}
}
