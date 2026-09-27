package gdcover

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
)

// requireGodot returns a usable engine path or skips the test. The unit-test
// job runs without Godot, so a missing engine is a skip here rather than a
// failure; the e2e job supplies one via GODOT_PATH.
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

// writeProject creates a minimal Godot project and returns its directory.
func writeProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "project.godot"), "config_version=5\n")
	return dir
}

// readHits parses the one-integer-per-line hit file into a set.
func readHits(t *testing.T, path string) map[int]bool {
	t.Helper()
	return readHitsFile(t, path)
}

// readHitsFile parses one hit file into a set.
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

// readHitsGlob merges every trace matched by pattern. It lets a test read hits
// whether the engine wrote the bare GDCOVER_FILE or, under the gdcover build
// tag, the per-PID path the engine environment hook derives from it.
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

// linesWith returns the 1-based lines of src containing marker.
func linesWith(src, marker string) []int {
	lines := make([]int, 0, strings.Count(src, marker))
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, marker) {
			lines = append(lines, i+1)
		}
	}
	return lines
}

// lineOf returns the 1-based line of the first occurrence of substr.
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

// runInstrumentedAt runs an instrumented script under a real engine, appending
// hits to hitsPath, and fails when the engine exits non-zero.
func runInstrumentedAt(t *testing.T, engine, project, script, hitsPath string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), engine, //nolint:gosec // engine is the test's own binary.
		"--headless", "--path", project, "--script", script)
	cmd.Env = append(os.Environ(), FileEnv+"="+hitsPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run instrumented script: %v\n%s", err, out)
	}
}

// runInstrumented writes src to a file, runs it under a real engine, and
// returns the reported hit lines.
func runInstrumented(t *testing.T, engine, src string) map[int]bool {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "instrumented.gd")
	writeFile(t, script, src)
	hits := filepath.Join(dir, "hits.txt")
	runInstrumentedAt(t, engine, writeProject(t), script, hits)
	return readHits(t, hits)
}

// TestInstrumentedScriptsAppendAcrossRuns guards the shared hit trace: every
// engine run against one GDCOVER_FILE appends to the same file, so a later run
// must add to the earlier run's hits instead of reopening the file in truncating
// write mode. A regression here silently shrinks the bridge report to whatever
// the last engine process happened to execute.
func TestInstrumentedScriptsAppendAcrossRuns(t *testing.T) {
	engine := requireGodot(t)
	dir := t.TempDir()
	project := writeProject(t)
	hits := filepath.Join(dir, "hits.txt")

	// The two scripts report different lines ("# first" on line 4, "# second"
	// on line 6) so a set of hits distinguishes append from truncate.
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

	firstResult, err := Instrument([]byte(first))
	if err != nil {
		t.Fatalf("Instrument first: %v", err)
	}
	secondResult, err := Instrument([]byte(second))
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

// TestInstrumentedScriptReportsHits is the end-to-end proof: instrument a
// fixture, run it, and check that exactly the lines annotated "# covered" were
// reported and the "# uncovered" ones were not.
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
	result, err := Instrument([]byte(src))
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

// instrumentedOperationsScript materializes the embedded bridge, instruments
// it, and returns the instrumented path plus the original bytes so reported
// line numbers can be mapped back.
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
	result, err := Instrument(original)
	if err != nil {
		t.Fatalf("Instrument: %v", err)
	}
	instrPath := filepath.Join(t.TempDir(), "godot_operations_instrumented.gd")
	writeFile(t, instrPath, string(result.Source))
	return instrPath, original
}

// TestInstrumentedOperationsScriptParses checks the instrumenter against the
// real bridge: the whole file must still compile.
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

// TestInstrumentedOperationsScriptReportsHits runs a real operation through the
// instrumented bridge and checks that the dispatch and handler paths report
// their original lines.
func TestInstrumentedOperationsScriptReportsHits(t *testing.T) {
	engine := requireGodot(t)
	script, original := instrumentedOperationsScript(t)
	hits := filepath.Join(t.TempDir(), "hits.txt")
	t.Setenv(FileEnv, hits)

	_, stderr, exit, err := godot.RunOperation(context.Background(), engine,
		writeProject(t), script, "validate_project", godot.OperationParams{"operation": "validate_project"})
	if err != nil {
		t.Fatalf("RunOperation: %v", err)
	}
	if exit != 0 {
		t.Fatalf("validate_project exited %d: %s", exit, stderr)
	}

	got := readHitsGlob(t, hits+"*")
	for _, marker := range []string{"run_one_shot(OS.get_cmdline_args())", `"validate_project":`} {
		line := lineOf(t, string(original), marker)
		if !got[line] {
			t.Errorf("line %d (%q) was not reported; hits=%v", line, marker, got)
		}
	}
}
