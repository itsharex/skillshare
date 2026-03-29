package resource

import (
	"os"
	"path/filepath"
	"strings"

	"skillshare/internal/utils"
)

// AgentKind handles single-file .md agent resources.
type AgentKind struct{}

var _ ResourceKind = AgentKind{}

func (AgentKind) Kind() string { return "agent" }

// Discover scans sourceDir for .md files, excluding conventional files
// (README.md, LICENSE.md, etc.) and hidden files.
func (AgentKind) Discover(sourceDir string) ([]DiscoveredResource, error) {
	walkRoot := utils.ResolveSymlink(sourceDir)

	var resources []DiscoveredResource

	err := filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			if info.Name() == ".git" || utils.IsHidden(info.Name()) && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		// Only .md files
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}

		// Skip conventional excludes
		if ConventionalExcludes[info.Name()] {
			return nil
		}

		// Skip hidden files
		if utils.IsHidden(info.Name()) {
			return nil
		}

		relPath, relErr := filepath.Rel(walkRoot, path)
		if relErr != nil {
			return nil
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		name := agentNameFromFile(path, info.Name())

		isNested := strings.Contains(relPath, "/")

		resources = append(resources, DiscoveredResource{
			Name:       name,
			Kind:       "agent",
			RelPath:    relPath,
			AbsPath:    path,
			IsNested:   isNested,
			FlatName:   AgentFlatName(relPath),
			SourcePath: filepath.Join(sourceDir, relPath),
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return resources, nil
}

// agentNameFromFile resolves an agent name. Checks frontmatter name field
// first, falls back to filename without .md extension.
func agentNameFromFile(filePath, fileName string) string {
	name := utils.ParseFrontmatterField(filePath, "name")
	if name != "" {
		return name
	}
	return strings.TrimSuffix(fileName, ".md")
}

// ResolveName extracts the agent name from an .md file.
// Checks frontmatter name field first, falls back to filename.
func (AgentKind) ResolveName(path string) string {
	return agentNameFromFile(path, filepath.Base(path))
}

// FlatName strips directory prefixes, keeping only the filename.
// Example: "curriculum/math-tutor.md" → "math-tutor.md"
func (AgentKind) FlatName(relPath string) string {
	return AgentFlatName(relPath)
}

// AgentFlatName is the standalone flat name computation for agents.
// Strips directory prefixes, keeping only the filename.
func AgentFlatName(relPath string) string {
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	return filepath.Base(relPath)
}

// CreateLink creates a file symlink from dst pointing to src.
func (AgentKind) CreateLink(src, dst string) error {
	return os.Symlink(src, dst)
}

func (AgentKind) SupportsAudit() bool   { return true }
func (AgentKind) SupportsTrack() bool   { return true }
func (AgentKind) SupportsCollect() bool { return true }
