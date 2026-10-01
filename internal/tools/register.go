package tools

import (
	"fmt"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

type Category string

const (
	CategoryProject Category = "project"
	CategoryExport  Category = "export"
	CategoryScene   Category = "scene"
	CategoryNode    Category = "node"
	CategoryTilemap Category = "tilemap"
	CategoryScript  Category = "script"
	CategoryAssets  Category = "assets"
	CategoryAnim    Category = "animation"
	CategoryTileset Category = "tileset"
	CategoryTheme   Category = "theme"
)

var AllCategories = []Category{
	CategoryProject,
	CategoryExport,
	CategoryScene,
	CategoryNode,
	CategoryTilemap,
	CategoryScript,
	CategoryAssets,
	CategoryAnim,
	CategoryTileset,
	CategoryTheme,
}

var categoryTools = map[Category][]func() mcp.Tool{
	CategoryProject: {
		getGodotVersion, createProject, validateProject, rescanProject,
	},
	CategoryExport: {
		exportProject, exportMeshLibrary,
	},
	CategoryScene: {
		createScene, readScene,
	},
	CategoryNode: {
		addNode, modifySceneNode, removeSceneNode,
		renameSceneNode, duplicateSceneNode, moveSceneNode,
	},
	CategoryTilemap: {
		readTilemap,
	},
	CategoryScript: {
		attachScript, validateScripts, runScript,
	},
	CategoryAssets: {
		createResource, modifyResource, readBinaryResource,
	},
	CategoryAnim: {
		readAnimation,
	},
	CategoryTileset: {
		readTileset,
	},
	CategoryTheme: {
		readTheme,
	},
}

// A copy, not an alias, so callers can mutate it without disturbing AllCategories.
var DefaultCategories = append([]Category(nil), AllCategories...)

func Register(s *mcp.Server, project string, categories ...Category) {
	for _, category := range categories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			tool.Handler = bindProject(project, tool.Handler)
			s.RegisterTool(tool)
		}
	}
}

func ResolveCategories(raw []string) ([]Category, error) {
	if len(raw) == 0 {
		return append([]Category(nil), DefaultCategories...), nil
	}
	seen := make(map[Category]bool)
	resolved := make([]Category, 0, len(raw))
	add := func(category Category) {
		if !seen[category] {
			seen[category] = true
			resolved = append(resolved, category)
		}
	}
	for _, value := range raw {
		for _, name := range strings.Split(value, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if name == "all" {
				for _, category := range AllCategories {
					add(category)
				}
				continue
			}
			category := Category(name)
			if !knownCategory(category) {
				return nil, fmt.Errorf("unknown category %q", name)
			}
			add(category)
		}
	}
	if len(resolved) == 0 {
		return append([]Category(nil), DefaultCategories...), nil
	}
	return resolved, nil
}

func knownCategory(category Category) bool {
	for _, known := range AllCategories {
		if known == category {
			return true
		}
	}
	return false
}
