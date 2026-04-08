//go:build !online

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"skillshare/internal/install"
	"skillshare/internal/testutil"
	"testing"
)

func TestDebug_NestedInstall(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	setupGlobalConfig(sb)

	remoteRepo := filepath.Join(sb.Root, "nested.git")
	workClone := filepath.Join(sb.Root, "nested-work")
	gitInit(t, remoteRepo, true)
	gitClone(t, remoteRepo, workClone)

	for _, name := range []string{"keep-nested", "stale-nested"} {
		os.MkdirAll(filepath.Join(workClone, "skills", name), 0755)
		os.WriteFile(filepath.Join(workClone, "skills", name, "SKILL.md"),
			[]byte("---\nname: "+name+"\n---\n# "+name), 0644)
	}
	gitAddCommit(t, workClone, "add skills")
	gitPush(t, workClone)

	for _, name := range []string{"keep-nested", "stale-nested"} {
		r := sb.RunCLI("install", "file://"+remoteRepo+"//skills/"+name, "--into", "mygroup", "--skip-audit")
		fmt.Printf("Install %s stdout: %s\n", name, r.Stdout)
		fmt.Printf("Install %s stderr: %s\n", name, r.Stderr)
		fmt.Printf("Install %s exit: %d\n", name, r.ExitCode)
		r.AssertSuccess(t)
	}

	fmt.Println("\n=== Files after install ===")
	filepath.Walk(sb.SourcePath, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			rel, _ := filepath.Rel(sb.SourcePath, path)
			fmt.Println(" -", rel)
		}
		return nil
	})

	metaPath := filepath.Join(sb.SourcePath, ".metadata.json")
	data, err := os.ReadFile(metaPath)
	fmt.Printf("\n.metadata.json (%v): %s\n", err, data)

	store, loadErr := install.LoadMetadata(sb.SourcePath)
	fmt.Printf("store entries (%v): %v\n", loadErr, store.List())
}
