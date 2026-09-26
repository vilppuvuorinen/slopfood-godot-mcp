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
		getGodotVersion, getProjectInfo, createProject,
	},
	CategorySettings: {
		readProjectSettings, modifyProjectSettings, manageInputMap, readInputMap,
	},
	CategoryExport: {
		manageExportPresets, readExportPresets, exportProject,
	},
	CategoryScene: {
		createScene, readScene, exportMeshLibrary,
	},
	CategoryNode: {
		addNode, modifySceneNode, removeSceneNode,
		manageSceneStructure, manageSceneSignals, readSceneSignals,
	},
	CategoryScript: {
		attachScript, validateScripts,
	},
	CategoryResources: {
		manageResource, readResource, getUID, updateProjectUIDs,
		manageAnimation, readAnimation,
	},
}

// DefaultCategories is the set registered when --enable is not passed. It is a
// copy of AllCategories, not an alias, so a caller can mutate it without
// disturbing the canonical order.
var DefaultCategories = append([]Category(nil), AllCategories...)

// Register adds the tools in the given categories to the server, binding every
// project-scoped tool to project. The root is fixed for the life of the server,
// so tools take only project-relative paths and never a project argument.
func Register(s *mcp.Server, project string, categories ...Category) {
	for _, category := range categories {
		for _, newTool := range categoryTools[category] {
			tool := newTool()
			tool.Handler = bindProject(project, tool.Handler)
			s.RegisterTool(tool)
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
