//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/install"
	"skillshare/internal/testutil"
)

func TestDiag_IntoMetadata(t *testing.T) {
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
	_ = result

	// Read the raw content of .metadata.json
	metaPath := filepath.Join(sb.SourcePath, ".metadata.json")
	data, err := os.ReadFile(metaPath)
	t.Logf("metadata.json read error: %v", err)
	t.Logf("metadata.json content: %s", string(data))

	// Also check the sidecar
	sidecarPath := filepath.Join(sb.SourcePath, "frontend", "pdf-skill", ".skillshare-meta.json")
	data2, err2 := os.ReadFile(sidecarPath)
	t.Logf("sidecar read error: %v", err2)
	t.Logf("sidecar content: %s", string(data2))

	store, _ := install.LoadMetadata(sb.SourcePath)
	t.Logf("store entries: %v", store.List())
}
