package tools

import (
	"context"
	"fmt"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
)

// Session is the live engine a serving context may carry. The seam needs only
// its shape: the concrete session (internal/godot's long-lived Session) owns the
// process and its state, while the per-call Engine below binds it to one call's
// context. Run takes the call context so the engine honors the queue's
// cancellation, and Invalidate marks the engine's project-global state dirty so
// the next call respawns it.
type Session interface {
	Run(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error)
	Invalidate()
}

// engineKey is the context key under which a live engine session is carried,
// mirroring projectKey.
type engineKey struct{}

// WithEngine returns a context carrying a live engine session. runDaemon
// installs the session on the context it hands to daemon.Serve, so every
// handler and every queue job inherits it through the context chain. Installing
// it on the serving context, rather than at registration, keeps the seam free
// of any ordering constraint between building the session and registering
// tools.
func WithEngine(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, engineKey{}, s)
}

// Engine is the single entry point a session-eligible handler uses to reach a
// Godot engine. It binds the call context to whichever engine serves the call,
// so one-shot and warm-session calls are interchangeable and a handler never
// branches on mode. The wrapper is per call and cheap; the session it may
// delegate to is long-lived.
type Engine struct {
	ctx     context.Context
	session Session
}

// engineFrom returns the Engine bound to ctx: the installed session when one is
// present, otherwise the one-shot engine. A package-level setter is avoided
// because tests run in parallel in one process and would race on it.
func engineFrom(ctx context.Context) *Engine {
	s, _ := ctx.Value(engineKey{}).(Session)
	return &Engine{ctx: ctx, session: s}
}

// Run executes one operations-script call. With no session installed it is
// exactly a one-shot spawn; with a session it is a warm-engine call. Either way
// it returns the same tuple, so a caller cannot tell which engine was targeted.
func (e *Engine) Run(project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	if e.session != nil {
		// The session already describes the failure the same way the one-shot
		// path does, so callers cannot tell the transports apart; wrapping here
		// would make the session's message differ.
		return e.session.Run(e.ctx, project, operation, params) //nolint:wrapcheck // session errors are already caller-facing.
	}
	return runOneShot(e.ctx, project, operation, params)
}

// Invalidate marks the engine's project-global state dirty so the next call
// respawns it. It is a no-op when no session is installed.
func (e *Engine) Invalidate() {
	if e.session != nil {
		e.session.Invalidate()
	}
}

// runOneShot executes an operations-script operation as a one-shot engine
// spawn. It is the one-shot implementation behind Engine.Run and is exactly the
// spawn the tool layer used before the session existed.
func runOneShot(ctx context.Context, projectPath, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	scriptPath, err := godotops.OperationsScript()
	if err != nil {
		return "", "", 0, fmt.Errorf("materialize operations script: %w", err)
	}
	stdout, stderr, exitCode, err = godot.RunOperation(ctx, godot.Path(), projectPath, scriptPath, operation, params)
	if err != nil {
		return stdout, stderr, exitCode, fmt.Errorf("run godot operation: %w", err)
	}
	return stdout, stderr, exitCode, nil
}
