package tools

// normalizeReadPath accepts a project-relative or res:// path and returns the
// confined absolute path, or an error explaining the rejection. The caller
// forwards the original string to the bridge; the bridge re-prepends res://
// only when absent.
func normalizeReadPath(project, path string) (string, error) {
	if !validPath(project) || !validPath(path) { // validPath rejects "" and ".."
		return "", invalidPathError()
	}
	return confinePath(project, path) // accepts res://; rejects user:// and absolute paths
}
