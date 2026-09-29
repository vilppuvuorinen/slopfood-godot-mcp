package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// socketOnlyEngine writes a fake engine that speaks only the bridge's
// newline-delimited Unix-socket protocol. If it is ever invoked without
// --gd-socket - the removed one-shot `<op> <json>` argv form, or `--import` -
// it records the fact and exits non-zero, so a tool call that still spawned a
// one-shot engine fails loudly instead of passing. It returns the executable
// path plus the argv, operation, and one-shot marker files it writes.
func socketOnlyEngine(t *testing.T) (engine, args, ops, oneShot string) {
	t.Helper()
	dir := t.TempDir()
	engine = filepath.Join(dir, "godot")
	args = filepath.Join(dir, "args")
	ops = filepath.Join(dir, "ops")
	oneShot = filepath.Join(dir, "one-shot")
	body := `#!/bin/sh
printf '%s\n' "$*" >> '` + args + `'
case " $* " in
  *" --gd-socket "*) ;;
  *) echo one-shot > '` + oneShot + `'; echo "fake engine: non-socket invocation: $*" >&2; exit 1 ;;
esac
exec python3 - "$@" <<'PY'
import json, os, socket, sys
args = sys.argv[1:]
path = args[args.index("--gd-socket") + 1]
if os.path.exists(path):
    os.remove(path)
srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
srv.bind(path)
srv.listen(1)
print(json.dumps({"ready": True, "transport": "uds", "path": path, "token": "tok"}), flush=True)
conn, _ = srv.accept()
buf = b""
while True:
    data = conn.recv(4096)
    if not data:
        break
    buf += data
    while b"\n" in buf:
        line, buf = buf.split(b"\n", 1)
        req = json.loads(line)
        open('` + ops + `', "a").write(req.get("operation", "") + "\n")
        if req.get("operation") == "quit":
            conn.sendall(json.dumps({"id": req["id"], "ok": True, "stdout": "", "stderr": ""}).encode() + b"\n")
            sys.exit(0)
        conn.sendall(json.dumps({"id": req["id"], "ok": True, "stdout": "fake engine", "stderr": ""}).encode() + b"\n")
PY
`
	if err := os.WriteFile(engine, []byte(body), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write fake engine: %v", err)
	}
	return engine, args, ops, oneShot
}

// TestToolCallNeverSpawnsOneShotEngine proves no tool call falls back to the
// removed one-shot operations bridge. GODOT_PATH points at a fake that serves
// only the socket protocol and fails on any non-socket argv (the one-shot
// `<op> <json>` form or `--import`), so a successful mutation shows the request
// traveled over the session's socket and the server never spawned a one-shot
// engine from a tool call.
func TestToolCallNeverSpawnsOneShotEngine(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	engine, args, ops, oneShot := socketOnlyEngine(t)
	session := connectProjectWithEnv(t, root, "GODOT_PATH="+engine)

	requireToolText(t, session, "create_scene", map[string]any{"scenePath": "scenes/main.tscn"})

	if _, err := os.Stat(oneShot); err == nil {
		t.Errorf("the engine was spawned in a non-socket form:\n%s", readFile(t, args))
	}
	requireContains(t, readFile(t, args), "--gd-socket")
	spawnArgs := readFile(t, args)
	for _, unwanted := range []string{"--import", "--gd-daemon"} {
		if strings.Contains(spawnArgs, unwanted) {
			t.Errorf("spawn args carry %q:\n%s", unwanted, spawnArgs)
		}
	}
	requireContains(t, readFile(t, ops), "create_scene")
}
