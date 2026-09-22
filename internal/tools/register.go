// Package tools implements the MCP tools exposed by the server.
package tools

import (
	"fmt"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// Category is a named group of tools selected with the --enable flag.
type Category string

const (
	CategoryProject   Category = "project"
	CategorySettings  Category = "settings"
	CategoryExport    Category = "export"
	CategoryScene     Category = "scene"
	CategoryNode      Category = "node"
	CategoryScript    Category = "script"
	CategoryResources Category = "resources"
)

// AllCategories lists every category in registration order.
var AllCategories = []Category{
	CategoryProject,
	CategorySettings,
	CategoryExport,
	CategoryScene,
	CategoryNode,
	CategoryScript,
	CategoryResources,
}

// categoryTools maps each category to the tool constructors it contains. Every
// tool belongs to exactly one category.
var categoryTools = map[Category][]func() mcp.Tool{
	CategoryProject: {
		getGodotVersion, listProjects, getProjectInfo, createProject,
	},
	CategorySettings: {
		readProjectSettings, modifyProjectSettings, setMainScene,
		manageAutoloads, manageInputMap, manageLayers, managePlugins,
		manageTranslations,
	},
	CategoryExport: {
		manageExportPresets, exportProject,
	},
	CategoryScene: {
		createScene, readScene, saveScene, exportMeshLibrary,
	},
	CategoryNode: {
		addNode, loadSprite, modifySceneNode, removeSceneNode,
		manageSceneStructure, manageSceneSignals,
	},
	CategoryScript: {
		createScript, createCSharpScript, attachScript,
		validateScript, validateScripts,
	},
	CategoryResources: {
		createResource, manageResource, manageShader, manageThemeResource,
		getUID, updateProjectUIDs,
	},
}

// DefaultCategories is the set registered when --enable is not passed.
var DefaultCategories = []Category{CategoryProject}

// Register adds the tools in the given categories to the server.
func Register(s *mcp.Server, categories ...Category) {
	for _, category := range categories {
		for _, newTool := range categoryTools[category] {
			s.RegisterTool(newTool())
		}
	}
}

// ResolveCategories turns raw --enable values into a category list. Empty input
// yields DefaultCategories. Each value may be a comma-separated list and "all"
// expands to every category; unknown names are an error.
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

// knownCategory reports whether category is one of AllCategories.
func knownCategory(category Category) bool {
	for _, known := range AllCategories {
		if known == category {
			return true
		}
	}
	return false
}
