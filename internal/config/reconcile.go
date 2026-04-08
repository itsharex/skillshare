package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"skillshare/internal/install"
	"skillshare/internal/utils"
)

// ReconcileGlobalSkills scans the global source directory for remotely-installed
// skills (those with install metadata or tracked repos) and ensures they are
// present in the MetadataStore. This is the global-mode counterpart of
// ReconcileProjectSkills.
func ReconcileGlobalSkills(cfg *Config, store *install.MetadataStore) error {
	sourcePath := cfg.Source
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return nil // no skills dir yet
	}

	changed := false

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
		if utils.IsHidden(d.Name()) {
			return filepath.SkipDir
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		relPath, relErr := filepath.Rel(walkRoot, path)
		if relErr != nil {
			return nil
		}

		var source string
		tracked := isGitRepo(path)

		existing := store.Get(filepath.ToSlash(relPath))
		if existing != nil && existing.Source != "" {
			source = existing.Source
		} else if tracked {
			source = gitRemoteOrigin(path)
		}
		if source == "" {
			return nil
		}

		fullPath := filepath.ToSlash(relPath)
		live[fullPath] = true

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
		} else {
			entry := &install.MetadataEntry{
				Source:  source,
				Tracked: tracked,
				Branch:  branch,
			}
			if idx := strings.LastIndex(fullPath, "/"); idx >= 0 {
				entry.Group = fullPath[:idx]
			}
			store.Set(fullPath, entry)
			changed = true
		}

		if tracked {
			return filepath.SkipDir
		}
		if existing != nil && existing.Source != "" {
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan global skills: %w", err)
	}

	// Prune stale entries: skills in store but no longer on disk
	for _, name := range store.List() {
		if !live[name] {
			store.Remove(name)
			changed = true
		}
	}

	if changed {
		if err := store.Save(sourcePath); err != nil {
			return err
		}
	}

	return nil
}
