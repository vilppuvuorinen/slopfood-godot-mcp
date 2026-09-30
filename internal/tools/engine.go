package tools

import (
	"context"
	"errors"
)

// Session is the live engine a serving context may carry. The seam needs only
// its shape: the concrete session (internal/godot's long-lived Session) owns the
// process and its state, while the per-call Engine below binds it to one call's
// context. Run takes the call context so the engine honors the queue's
// cancellation.
type Session interface {
	Run(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error)
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
// Godot engine. It binds the call context to the live session that serves the
// call. The wrapper is per call and cheap; the session it delegates to is
// long-lived.
type Engine struct {
	ctx     context.Context
	session Session
}

// engineFrom returns the Engine bound to ctx: the installed session when one is
// present, otherwise an engine with no session. A package-level setter is
// avoided because tests run in parallel in one process and would race on it.
func engineFrom(ctx context.Context) *Engine {
	s, _ := ctx.Value(engineKey{}).(Session)
	return &Engine{ctx: ctx, session: s}
}

// Run executes one operations-script call through the installed session. A
// serving context always installs one: runDaemon installs the production
// session and tests install a double. A context without one is a programming
// error, reported rather than silently falling back to a separate engine.
func (e *Engine) Run(project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	if e.session == nil {
		return "", "", 0, errors.New("engine session required: no engine installed on the context")
	}
	// The session already describes the failure the same way a caller expects,
	// so wrapping here would change its message.
	return e.session.Run(e.ctx, project, operation, params) //nolint:wrapcheck // session errors are already caller-facing.
}
