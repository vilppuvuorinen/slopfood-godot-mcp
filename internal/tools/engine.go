package tools

import (
	"context"
	"errors"
)

type Session interface {
	Run(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error)
}

type engineKey struct{}

func WithEngine(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, engineKey{}, s)
}

type Engine struct {
	ctx     context.Context
	session Session
}

// A package-level setter would race when tests run in parallel.
func engineFrom(ctx context.Context) *Engine {
	s, _ := ctx.Value(engineKey{}).(Session)
	return &Engine{ctx: ctx, session: s}
}

func (e *Engine) Run(project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	if e.session == nil {
		return "", "", 0, errors.New("engine session required: no engine installed on the context")
	}
	return e.session.Run(e.ctx, project, operation, params) //nolint:wrapcheck // session errors are already caller-facing.
}
