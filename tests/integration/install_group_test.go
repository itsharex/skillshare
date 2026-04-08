//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/install"
	"skillshare/internal/testutil"
)

func TestInstall_Into_RecordsGroupField(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets: {}
`)

	// Create a local skill
	localSkill := filepath.Join(sb.Root, "pdf-skill")
	os.MkdirAll(localSkill, 0755)
	os.WriteFile(filepath.Join(localSkill, "SKILL.md"), []byte("# PDF Skill"), 0644)

	// Install with --into frontend
	result := sb.RunCLI("install", localSkill, "--into", "frontend")
	result.AssertSuccess(t)

	// Verify skill was installed into subdirectory
	if !sb.FileExists(filepath.Join(sb.SourcePath, "frontend", "pdf-skill", "SKILL.md")) {
		t.Error("skill should be installed to source/frontend/pdf-skill/")
	}

	// Read centralized metadata and verify group field
	store, err := install.LoadMetadata(sb.SourcePath)
	if err != nil {
		t.Fatalf("failed to load metadata: %v", err)
	}
	entry := store.Get("pdf-skill")
	if entry == nil {
		t.Fatal("expected metadata entry for pdf-skill")
	}
	if entry.Group != "frontend" {
		t.Errorf("metadata group = %q, want %q", entry.Group, "frontend")
	}
}

func TestInstall_Into_MultiLevel_RecordsGroupField(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets: {}
`)

	localSkill := filepath.Join(sb.Root, "ui-skill")
	os.MkdirAll(localSkill, 0755)
	os.WriteFile(filepath.Join(localSkill, "SKILL.md"), []byte("# UI Skill"), 0644)

	// Install with multi-level --into
	result := sb.RunCLI("install", localSkill, "--into", "frontend/vue")
	result.AssertSuccess(t)

	// Read centralized metadata and verify group field
	store, err := install.LoadMetadata(sb.SourcePath)
	if err != nil {
		t.Fatalf("failed to load metadata: %v", err)
	}
	entry := store.Get("ui-skill")
	if entry == nil {
		t.Fatal("expected metadata entry for ui-skill")
	}
	if entry.Group != "frontend/vue" {
		t.Errorf("metadata group = %q, want %q", entry.Group, "frontend/vue")
	}
}

func TestInstall_ConfigBased_WithGroup(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	// Create a local skill to use as source
	sourceSkill := filepath.Join(sb.Root, "source-pdf")
	os.MkdirAll(sourceSkill, 0755)
	os.WriteFile(filepath.Join(sourceSkill, "SKILL.md"), []byte("# PDF Skill"), 0644)

	// First install normally with --into to populate config
	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets: {}
`)
	result := sb.RunCLI("install", sourceSkill, "--into", "frontend")
	result.AssertSuccess(t)

	// Verify skill exists
	skillPath := filepath.Join(sb.SourcePath, "frontend", "source-pdf", "SKILL.md")
	if !sb.FileExists(skillPath) {
		t.Fatal("skill should exist after initial install")
	}

	// Verify metadata was stored correctly after install
	store, err := install.LoadMetadata(sb.SourcePath)
	if err != nil {
		t.Fatalf("failed to load metadata: %v", err)
	}
	entry := store.Get("source-pdf")
	if entry == nil {
		t.Fatal("expected metadata entry for source-pdf after --into install")
	}
	if entry.Group != "frontend" {
		t.Errorf("metadata group = %q, want %q", entry.Group, "frontend")
	}
}

func TestInstall_LegacySlashName_BackwardCompat(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	// Write config with legacy format (name contains slash, no group field)
	// This is the format that existed before the group field was added
	sourceSkill := filepath.Join(sb.Root, "source-pdf")
	os.MkdirAll(sourceSkill, 0755)
	os.WriteFile(filepath.Join(sourceSkill, "SKILL.md"), []byte("# PDF"), 0644)

	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets: {}
skills:
  - name: frontend/pdf
    source: ` + sourceSkill + `
`)

	// Config-based install with legacy slash name should still work
	result := sb.RunCLI("install")
	result.AssertSuccess(t)

	// Verify skill was installed correctly
	if !sb.FileExists(filepath.Join(sb.SourcePath, "frontend", "pdf", "SKILL.md")) {
		t.Error("legacy slash-name install should place skill at frontend/pdf/")
	}
}

func TestInstallProject_Into_RecordsGroupField(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	projectRoot := sb.SetupProjectDir("claude")

	// Create a source skill
	sourceSkill := filepath.Join(sb.Root, "my-skill")
	os.MkdirAll(sourceSkill, 0755)
	os.WriteFile(filepath.Join(sourceSkill, "SKILL.md"), []byte("---\nname: my-skill\n---\n# My Skill"), 0644)

	// Install with --into in project mode
	result := sb.RunCLIInDir(projectRoot, "install", sourceSkill, "--into", "tools", "-p")
	result.AssertSuccess(t)

	// Read centralized metadata and verify group field
	store, err := install.LoadMetadata(filepath.Join(projectRoot, ".skillshare", "skills"))
	if err != nil {
		t.Fatalf("failed to load metadata: %v", err)
	}
	entry := store.Get("my-skill")
	if entry == nil {
		t.Fatal("expected metadata entry for my-skill")
	}
	if entry.Group != "tools" {
		t.Errorf("metadata group = %q, want %q", entry.Group, "tools")
	}
}
