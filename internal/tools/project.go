package tools

// toolset binds the per-daemon engine session and the project root that every
// tool operates against. Handlers are methods so neither dependency travels
// through the request context.
type toolset struct {
	project string
	session Session
}

func newToolset(project string, session Session) *toolset {
	return &toolset{project: project, session: session}
}
