package tools

import (
	"os"
	"path/filepath"
	"strings"
)

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
