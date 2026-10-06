package godotops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

func requireGodot(t *testing.T) string {
	t.Helper()
	path := godot.Path()
	if _, err := godot.Version(context.Background(), path); err != nil {
		t.Skipf("Godot unavailable at %q: %v", path, err)
	}
	return path
}

func TestDispatchSurvivesHandlerAbort(t *testing.T) {
	engine := requireGodot(t)
	withCache(t)
	script, err := materialize("godot_operations", operationsScript)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	project := t.TempDir()
	if writeErr := os.WriteFile(filepath.Join(project, "project.godot"), []byte("config_version=5\n"), 0o644); writeErr != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", writeErr)
	}

	sess := godot.NewSession(engine, script)
	defer func() { _ = sess.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, stderr, exitCode, err := sess.Run(ctx, project, "read_scene",
		map[string]any{"scenePath": 123})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1; stderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "unhandled engine error") {
		t.Errorf("stderr = %q, want the dispatcher's recovery message", stderr)
	}
}

// newBridge starts a session against a fresh project and returns its context.
func newBridge(t *testing.T) (string, *godot.Session, context.Context) {
	t.Helper()
	engine := requireGodot(t)
	withCache(t)
	script, err := materialize("godot_operations", operationsScript)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "project.godot"), []byte("config_version=5\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", err)
	}
	sess := godot.NewSession(engine, script)
	t.Cleanup(func() { _ = sess.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return project, sess, ctx
}

// A call that writes a file returns only after the closing scan has settled,
// so the file's .uid sidecar is present when the call returns.
func TestBridgeGateScansAfterHandlerWrite(t *testing.T) {
	project, sess, ctx := newBridge(t)
	body := "var f = FileAccess.open(\"res://generated.gd\", FileAccess.WRITE)\nf.store_string(\"extends Node\\n\")\nreturn 0\n"
	if _, stderr, code, err := sess.Run(ctx, project, "run_script", map[string]any{"body": body}); err != nil || code != 0 {
		t.Fatalf("run_script: err=%v code=%d stderr=%s", err, code, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, "generated.gd.uid")); err != nil {
		t.Fatalf("generated.gd.uid missing after the call: %v", err)
	}
}

// An out-of-band write in the same second as the previous scan is still
// reconciled by the entry scan of the next call.
func TestBridgeGateSeesOutOfBandWrite(t *testing.T) {
	project, sess, ctx := newBridge(t)
	if _, _, _, err := sess.Run(ctx, project, "rescan_project", nil); err != nil {
		t.Fatalf("warmup rescan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "external.gd"), []byte("extends Node\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write external.gd: %v", err)
	}
	if _, _, _, err := sess.Run(ctx, project, "ping", nil); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "external.gd.uid")); err != nil {
		t.Fatalf("external.gd.uid missing after the call: %v", err)
	}
}

// A dropped .uid is restored from the guard while its source still exists, and
// a second pass leaves the restored uid untouched.
func TestBridgeUIDGuardPreservesDroppedUid(t *testing.T) {
	project, sess, ctx := newBridge(t)
	if err := os.WriteFile(filepath.Join(project, "used.gd"), []byte("extends Node\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write used.gd: %v", err)
	}
	uidPath := filepath.Join(project, "used.gd.uid")
	const original = "uid://original123"
	if err := os.WriteFile(uidPath, []byte(original+"\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write used.gd.uid: %v", err)
	}
	if _, _, _, err := sess.Run(ctx, project, "rescan_project", nil); err != nil {
		t.Fatalf("snapshot rescan: %v", err)
	}
	if err := os.Remove(uidPath); err != nil {
		t.Fatalf("remove used.gd.uid: %v", err)
	}
	for pass := 0; pass < 2; pass++ {
		if _, _, _, err := sess.Run(ctx, project, "rescan_project", nil); err != nil {
			t.Fatalf("restore rescan %d: %v", pass, err)
		}
		data, err := os.ReadFile(uidPath)
		if err != nil {
			t.Fatalf("pass %d: used.gd.uid missing: %v", pass, err)
		}
		if got := strings.TrimSpace(string(data)); got != original {
			t.Errorf("pass %d: uid = %q, want %q", pass, got, original)
		}
	}
}
