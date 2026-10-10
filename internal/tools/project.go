package tools

// toolset binds the project root and the engine seam every tool operates
// through. Handlers are methods so the dependency travels not through the
// request context.
type toolset struct {
	project string
	engine  Engine
}

func newToolset(project string, engine Engine) *toolset {
	return &toolset{project: project, engine: engine}
}
