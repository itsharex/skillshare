package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MetadataFileName is the centralized metadata file stored in each directory.
const MetadataFileName = ".metadata.json"

// MetadataStore holds all entries for a single directory (skills/ or agents/).
type MetadataStore struct {
	Version int                       `json:"version"`
	Entries map[string]*MetadataEntry `json:"entries"`
}

// MetadataEntry merges the old SkillMeta + RegistryEntry fields.
type MetadataEntry struct {
	// Registry fields
	Source  string `json:"source"`
	Kind    string `json:"kind,omitempty"`
	Type    string `json:"type,omitempty"`
	Tracked bool   `json:"tracked,omitempty"`
	Group   string `json:"group,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Into    string `json:"into,omitempty"`
	Name    string `json:"-"` // runtime only, not persisted (map key is the name)

	// Meta fields
	InstalledAt time.Time         `json:"installed_at,omitzero"`
	RepoURL     string            `json:"repo_url,omitempty"`
	Subdir      string            `json:"subdir,omitempty"`
	Version     string            `json:"version,omitempty"`
	TreeHash    string            `json:"tree_hash,omitempty"`
	FileHashes  map[string]string `json:"file_hashes,omitempty"`
}

// NewMetadataStore returns an empty store with version 1.
func NewMetadataStore() *MetadataStore {
	return &MetadataStore{
		Version: 1,
		Entries: make(map[string]*MetadataEntry),
	}
}

// Get returns the entry for the given name, or nil if not found.
func (s *MetadataStore) Get(name string) *MetadataEntry {
	return s.Entries[name]
}

// Set adds or replaces an entry.
func (s *MetadataStore) Set(name string, entry *MetadataEntry) {
	s.Entries[name] = entry
}

// Remove deletes an entry by name.
func (s *MetadataStore) Remove(name string) {
	delete(s.Entries, name)
}

// Has returns true if an entry exists for the given name.
func (s *MetadataStore) Has(name string) bool {
	_, ok := s.Entries[name]
	return ok
}

// GetByPath looks up an entry by its full relative path (e.g. "mygroup/keep-nested").
// It first tries a direct key lookup, then falls back to matching group+basename.
// This handles the case where entries are stored with basename keys but have a Group field.
func (s *MetadataStore) GetByPath(relPath string) *MetadataEntry {
	// Direct lookup (works for top-level skills where key == relPath)
	if e := s.Entries[relPath]; e != nil {
		return e
	}
	// Basename + group lookup (for nested skills stored with basename key)
	base := filepath.Base(relPath)
	group := ""
	if dir := filepath.Dir(relPath); dir != "." {
		group = filepath.ToSlash(dir)
	}
	if e := s.Entries[base]; e != nil && e.Group == group {
		return e
	}
	return nil
}

// List returns sorted entry names.
func (s *MetadataStore) List() []string {
	names := make([]string, 0, len(s.Entries))
	for name := range s.Entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EffectiveKind returns "skill" if Kind is empty.
func (e *MetadataEntry) EffectiveKind() string {
	if e.Kind == "" {
		return "skill"
	}
	return e.Kind
}

// FullName returns "group/name" if Group is set, otherwise Name.
func (e *MetadataEntry) FullName() string {
	if e.Group != "" {
		return e.Group + "/" + e.Name
	}
	return e.Name
}

// LoadMetadata reads .metadata.json from the given directory.
// Returns an empty store (version 1) if the file does not exist.
func LoadMetadata(dir string) (*MetadataStore, error) {
	path := filepath.Join(dir, MetadataFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewMetadataStore(), nil
		}
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var store MetadataStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}
	if store.Entries == nil {
		store.Entries = make(map[string]*MetadataEntry)
	}
	return &store, nil
}

// Save writes .metadata.json atomically (temp file → rename).
func (s *MetadataStore) Save(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	data = append(data, '\n')

	target := filepath.Join(dir, MetadataFileName)
	tmp, err := os.CreateTemp(dir, ".metadata-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

// MetadataPath returns the .metadata.json path for the given directory.
func MetadataPath(dir string) string {
	return filepath.Join(dir, MetadataFileName)
}

// SetFromSource creates an entry from a Source and stores it. Returns the entry.
func (s *MetadataStore) SetFromSource(name string, src *Source) *MetadataEntry {
	entry := &MetadataEntry{
		Source:      src.Raw,
		Type:        src.MetaType(),
		InstalledAt: time.Now(),
		Branch:      src.Branch,
	}
	if src.IsGit() {
		entry.RepoURL = src.CloneURL
	}
	if src.HasSubdir() {
		entry.Subdir = strings.ReplaceAll(src.Subdir, "\\", "/")
	}
	s.Entries[name] = entry
	return entry
}

// ComputeEntryHashes walks skillPath and populates FileHashes with sha256 digests.
// Delegates to ComputeFileHashes in meta.go.
func (e *MetadataEntry) ComputeEntryHashes(skillPath string) error {
	hashes, err := ComputeFileHashes(skillPath)
	if err != nil {
		return err
	}
	e.FileHashes = hashes
	return nil
}

// RefreshHashes recomputes file hashes for an entry that already has them.
// No-op if entry doesn't exist or has no FileHashes.
func (s *MetadataStore) RefreshHashes(name, skillPath string) {
	entry := s.Get(name)
	if entry == nil || entry.FileHashes == nil {
		return
	}
	hashes, err := ComputeFileHashes(skillPath)
	if err != nil {
		return
	}
	entry.FileHashes = hashes
}
