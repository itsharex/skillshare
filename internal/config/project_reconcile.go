package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"skillshare/internal/install"
	"skillshare/internal/utils"
)

// ReconcileProjectSkills scans the project source directory recursively for
// remotely-installed skills (those with install metadata or tracked repos)
// and ensures they are present in the MetadataStore.
// It also updates .skillshare/.gitignore for each tracked skill.
func ReconcileProjectSkills(projectRoot string, projectCfg *ProjectConfig, store *install.MetadataStore, sourcePath string) error {
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return nil // no skills dir yet
	}

	changed := false

	// Collect gitignore entries during walk, then batch-update once at the end.
	var gitignoreEntries []string

	walkRoot := utils.ResolveSymlink(sourcePath)
	live := map[string]bool{} // tracks skills actually found on disk
	err := filepath.WalkDir(walkRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == walkRoot {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden directories
		if utils.IsHidden(d.Name()) {
			return filepath.SkipDir
		}
		// Skip .git directories
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		relPath, relErr := filepath.Rel(walkRoot, path)
		if relErr != nil {
			return nil
		}

		fullPath := filepath.ToSlash(relPath)

		// Extract basename (key) and group from the relative path.
		// The store uses basename as key and Group for the parent path.
		name := fullPath
		group := ""
		if idx := strings.LastIndex(fullPath, "/"); idx >= 0 {
			group = fullPath[:idx]
			name = fullPath[idx+1:]
		}

		// Determine source and tracked status
		var source string
		tracked := isGitRepo(path)

		existing := store.Get(name)
		if existing != nil && existing.Source != "" {
			source = existing.Source
		} else if tracked {
			// Tracked repos have no store entry yet; derive source from git remote
			source = gitRemoteOrigin(path)
		}
		if source == "" {
			// Not an installed skill — continue walking deeper
			return nil
		}

		live[name] = true

		// Determine branch: from store entry or git (tracked repos)
		var branch string
		if existing != nil && existing.Branch != "" {
			branch = existing.Branch
		} else if tracked {
			branch = gitCurrentBranch(path)
		}

		if existing != nil {
			if existing.Source != source {
				existing.Source = source
				changed = true
			}
			if existing.Tracked != tracked {
				existing.Tracked = tracked
				changed = true
			}
			if existing.Branch != branch {
				existing.Branch = branch
				changed = true
			}
			if existing.Group != group {
				existing.Group = group
				changed = true
			}
		} else {
			entry := &install.MetadataEntry{
				Source:  source,
				Tracked: tracked,
				Branch:  branch,
				Group:   group,
			}
			store.Set(name, entry)
			changed = true
		}

		gitignoreEntries = append(gitignoreEntries, filepath.Join("skills", fullPath))

		// If it's a tracked repo (has .git), don't recurse into it
		if tracked {
			return filepath.SkipDir
		}

		// If it has a source, it's a leaf skill — don't recurse
		if existing != nil && existing.Source != "" {
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan project skills: %w", err)
	}

	// Prune stale entries: skills in store but no longer on disk
	for _, name := range store.List() {
		if !live[name] {
			store.Remove(name)
			changed = true
		}
	}

	// Batch-update .gitignore once (reads/writes the file only once instead of per-skill).
	if len(gitignoreEntries) > 0 {
		if err := install.UpdateGitIgnoreBatch(filepath.Join(projectRoot, ".skillshare"), gitignoreEntries); err != nil {
			return fmt.Errorf("failed to update .skillshare/.gitignore: %w", err)
		}
	}

	if changed {
		if err := store.Save(sourcePath); err != nil {
			return err
		}
	}

	return nil
}

// ReconcileProjectAgents scans the project agents source directory for
// installed agents and ensures they are present in the MetadataStore.
// Also updates .skillshare/.gitignore for each agent.
func ReconcileProjectAgents(projectRoot string, store *install.MetadataStore, agentsSourcePath string) error {
	if _, err := os.Stat(agentsSourcePath); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(agentsSourcePath)
	if err != nil {
		return nil
	}

	changed := false
	var gitignoreEntries []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}

		agentName := strings.TrimSuffix(name, ".md")

		// Check store for this agent
		existing := store.Get(agentName)
		if existing == nil || existing.Source == "" {
			continue // local agent, not installed
		}

		// Ensure kind is set
		if existing.Kind != "agent" {
			existing.Kind = "agent"
			changed = true
		}

		gitignoreEntries = append(gitignoreEntries, filepath.Join("agents", name))
	}

	if len(gitignoreEntries) > 0 {
		if err := install.UpdateGitIgnoreBatch(filepath.Join(projectRoot, ".skillshare"), gitignoreEntries); err != nil {
			return fmt.Errorf("failed to update .skillshare/.gitignore for agents: %w", err)
		}
	}

	if changed {
		if err := store.Save(agentsSourcePath); err != nil {
			return err
		}
	}

	return nil
}

// isGitRepo checks if the given path is a git repository (has .git/ directory or file).
func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// gitCurrentBranch returns the current branch name for a git repo, or "" on failure.
func gitCurrentBranch(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitRemoteOrigin returns the "origin" remote URL for a git repo, or "" on failure.
func gitRemoteOrigin(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
