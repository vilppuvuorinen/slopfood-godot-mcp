package tools

// normalizeReadPath accepts a project-relative or res:// path and returns the
// confined absolute path, or ok=false. The caller forwards the original string
// to the bridge; the bridge re-prepends res:// only when absent.
func normalizeReadPath(project, path string) (full string, ok bool) {
	if !validPath(project) || !validPath(path) { // validPath rejects "" and ".."
		return "", false
	}
	return confinePath(project, projectRelativePath(path)) // strips res://; rejects user://, abs
}
