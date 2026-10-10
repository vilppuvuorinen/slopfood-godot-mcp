package tools

// toolset binds the project root and the two engine seams every tool operates
// through: the warm editor session and the one-shot headless runner. Handlers
// are methods so neither dependency travels through the request context.
type toolset struct {
	project  string
	session  Session
	headless Headless
}

func newToolset(project string, session Session, headless Headless) *toolset {
	return &toolset{project: project, session: session, headless: headless}
}
