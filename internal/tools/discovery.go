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

// projectStructure counts a project's files the way the reference server's
// getProjectStructure does: hidden entries are skipped, .tscn files are scenes,
// .gd/.gdscript/.cs files are scripts, common image/font/audio extensions are
// assets, and everything else lands in other.
type projectStructure struct {
	Scenes  int `json:"scenes"`
	Scripts int `json:"scripts"`
	Assets  int `json:"assets"`
	Other   int `json:"other"`
}

// assetExtensions mirrors the reference server's asset extension allowlist.
var assetExtensions = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "webp": true, "svg": true,
	"ttf": true, "wav": true, "mp3": true, "ogg": true,
}

// countProjectStructure recursively counts the files under projectPath.
func countProjectStructure(projectPath string) projectStructure {
	var structure projectStructure
	countFiles(projectPath, &structure)
	return structure
}

func countFiles(dir string, structure *projectStructure) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		if entry.IsDir() {
			countFiles(path, structure)
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		switch ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")); {
		case ext == "tscn":
			structure.Scenes++
		case ext == "gd" || ext == "gdscript" || ext == "cs":
			structure.Scripts++
		case assetExtensions[ext]:
			structure.Assets++
		default:
			structure.Other++
		}
	}
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
