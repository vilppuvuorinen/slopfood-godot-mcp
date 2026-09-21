package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// godotProject is one entry in the list_projects response.
type godotProject struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// findGodotProjects mirrors the reference server's project scan: it checks the
// target directory itself, then its immediate subdirectories, recursing when
// requested. Hidden directories are skipped and unreadable directories are
// ignored, matching the reference's tolerant behavior.
func findGodotProjects(directory string, recursive bool) []godotProject {
	projects := make([]godotProject, 0)
	if isGodotProject(directory) {
		projects = append(projects, godotProject{Path: directory, Name: filepath.Base(directory)})
	}

	for _, name := range subdirectories(directory) {
		subdir := filepath.Join(directory, name)
		if isGodotProject(subdir) {
			projects = append(projects, godotProject{Path: subdir, Name: name})
			continue
		}
		if recursive {
			projects = append(projects, findGodotProjects(subdir, true)...)
		}
	}
	return projects
}

// isGodotProject reports whether directory contains a project.godot file.
func isGodotProject(directory string) bool {
	info, err := os.Stat(filepath.Join(directory, "project.godot"))
	return err == nil && !info.IsDir()
}

// subdirectories returns the non-hidden immediate subdirectory names of
// directory, in lexical order. Unreadable directories yield no entries.
func subdirectories(directory string) []string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	return names
}
