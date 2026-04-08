package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadMetadataWithMigration loads .metadata.json, or migrates from old format if needed.
// kind is "" for skills directories, "agent" for agents directories.
func LoadMetadataWithMigration(dir, kind string) (*MetadataStore, error) {
	// Fast path: .metadata.json already exists
	metaPath := filepath.Join(dir, MetadataFileName)
	if _, err := os.Stat(metaPath); err == nil {
		return LoadMetadata(dir)
	}

	store := NewMetadataStore()

	// Phase 1: Migrate registry.yaml entries
	// Look in dir itself and its parent (registry.yaml may live in .skillshare/ while dir is .skillshare/skills/)
	migrateRegistryEntries(store, dir, kind)
	if parent := filepath.Dir(dir); parent != dir {
		migrateRegistryEntries(store, parent, kind)
	}

	// Phase 2: Migrate sidecar .skillshare-meta.json files
	if kind == "agent" {
		migrateAgentSidecars(store, dir)
	} else {
		migrateSkillSidecars(store, dir)
	}

	// Phase 3: Save if we found anything to migrate
	if len(store.Entries) > 0 {
		if err := store.Save(dir); err != nil {
			return store, err
		}
	}

	// Phase 4: Clean up old registry.yaml (in dir and parent)
	cleanupOldRegistry(dir)
	if parent := filepath.Dir(dir); parent != dir {
		cleanupOldRegistry(parent)
	}

	return store, nil
}

// localRegistryEntry mirrors config.SkillEntry without importing internal/config.
type localRegistryEntry struct {
	Name    string `yaml:"name"`
	Kind    string `yaml:"kind,omitempty"`
	Source  string `yaml:"source"`
	Tracked bool   `yaml:"tracked,omitempty"`
	Group   string `yaml:"group,omitempty"`
	Branch  string `yaml:"branch,omitempty"`
}

// localRegistry mirrors config.Registry without importing internal/config.
type localRegistry struct {
	Skills []localRegistryEntry `yaml:"skills,omitempty"`
}

// migrateRegistryEntries reads registry.yaml in dir and merges matching entries into store.
// For skills dirs (kind=""), agent entries are skipped.
// For agents dirs (kind="agent"), skill entries are skipped.
func migrateRegistryEntries(store *MetadataStore, dir, kind string) {
	registryPath := filepath.Join(dir, "registry.yaml")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return
	}

	var reg localRegistry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return
	}

	for _, e := range reg.Skills {
		if e.Name == "" || e.Source == "" {
			continue
		}

		isAgent := e.Kind == "agent"

		// Filter: skills dir skips agent entries, agents dir skips skill entries
		if kind == "agent" && !isAgent {
			continue
		}
		if kind == "" && isAgent {
			continue
		}

		entry := store.Get(e.Name)
		if entry == nil {
			entry = &MetadataEntry{}
			store.Set(e.Name, entry)
		}

		entry.Source = e.Source
		entry.Kind = e.Kind
		entry.Tracked = e.Tracked
		entry.Group = e.Group
		entry.Branch = e.Branch
	}
}

// migrateSkillSidecars walks subdirectories of dir, looks for .skillshare-meta.json
// inside each, reads as SkillMeta, merges fields into store entry, and removes old sidecar.
func migrateSkillSidecars(store *MetadataStore, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		skillName := de.Name()
		skillPath := filepath.Join(dir, skillName)
		walkSkillDir(store, skillPath, skillName, "")
	}
}

// walkSkillDir recursively walks a skill directory to find .skillshare-meta.json sidecars.
// group is the parent group prefix (empty for top-level skills).
func walkSkillDir(store *MetadataStore, skillPath, name, group string) {
	sidecarPath := filepath.Join(skillPath, MetaFileName)
	if _, err := os.Stat(sidecarPath); err == nil {
		// This directory has a sidecar — it's a leaf skill
		mergeSkillSidecar(store, name, group, sidecarPath)
		os.Remove(sidecarPath)
		return
	}

	// Check if this has subdirectories (nested skills)
	subEntries, err := os.ReadDir(skillPath)
	if err != nil {
		return
	}

	for _, sub := range subEntries {
		if sub.IsDir() {
			subGroup := name
			if group != "" {
				subGroup = group + "/" + name
			}
			walkSkillDir(store, filepath.Join(skillPath, sub.Name()), sub.Name(), subGroup)
		}
	}
}

// mergeSkillSidecar reads a SkillMeta sidecar and merges its fields into the store.
func mergeSkillSidecar(store *MetadataStore, name, group, sidecarPath string) {
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return
	}

	var meta SkillMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}

	entry := store.Get(name)
	if entry == nil {
		entry = &MetadataEntry{}
		store.Set(name, entry)
	}

	// Merge sidecar fields — sidecar has richer data
	if meta.Source != "" && entry.Source == "" {
		entry.Source = meta.Source
	}
	if meta.Kind != "" {
		entry.Kind = meta.Kind
	}
	if meta.Type != "" {
		entry.Type = meta.Type
	}
	if !meta.InstalledAt.IsZero() {
		entry.InstalledAt = meta.InstalledAt
	}
	if meta.RepoURL != "" {
		entry.RepoURL = meta.RepoURL
	}
	if meta.Subdir != "" {
		entry.Subdir = meta.Subdir
	}
	if meta.Version != "" {
		entry.Version = meta.Version
	}
	if meta.TreeHash != "" {
		entry.TreeHash = meta.TreeHash
	}
	if meta.FileHashes != nil {
		entry.FileHashes = meta.FileHashes
	}
	if meta.Branch != "" && entry.Branch == "" {
		entry.Branch = meta.Branch
	}
	if group != "" && entry.Group == "" {
		entry.Group = group
	}
}

// migrateAgentSidecars scans dir for *.skillshare-meta.json files, extracts agent name,
// reads as SkillMeta, merges into store with Kind="agent", removes old sidecar.
func migrateAgentSidecars(store *MetadataStore, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	const suffix = ".skillshare-meta.json"
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		if !strings.HasSuffix(de.Name(), suffix) {
			continue
		}

		agentName := strings.TrimSuffix(de.Name(), suffix)
		if agentName == "" {
			continue
		}

		sidecarPath := filepath.Join(dir, de.Name())
		data, err := os.ReadFile(sidecarPath)
		if err != nil {
			continue
		}

		var meta SkillMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		entry := store.Get(agentName)
		if entry == nil {
			entry = &MetadataEntry{}
			store.Set(agentName, entry)
		}

		if meta.Source != "" && entry.Source == "" {
			entry.Source = meta.Source
		}
		entry.Kind = "agent"
		if meta.Type != "" {
			entry.Type = meta.Type
		}
		if !meta.InstalledAt.IsZero() {
			entry.InstalledAt = meta.InstalledAt
		}
		if meta.RepoURL != "" {
			entry.RepoURL = meta.RepoURL
		}
		if meta.Subdir != "" {
			entry.Subdir = meta.Subdir
		}
		if meta.Version != "" {
			entry.Version = meta.Version
		}
		if meta.TreeHash != "" {
			entry.TreeHash = meta.TreeHash
		}
		if meta.FileHashes != nil {
			entry.FileHashes = meta.FileHashes
		}
		if meta.Branch != "" && entry.Branch == "" {
			entry.Branch = meta.Branch
		}

		os.Remove(sidecarPath)
	}
}

// cleanupOldRegistry removes registry.yaml from dir (best-effort, ignores errors).
func cleanupOldRegistry(dir string) {
	os.Remove(filepath.Join(dir, "registry.yaml"))
}
