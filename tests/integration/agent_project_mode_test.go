//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/testutil"
)

// setupProjectWithAgents creates a project directory with skills, agents, and config.
// Returns the project root path.
func setupProjectWithAgents(t *testing.T, sb *testutil.Sandbox) string {
	t.Helper()

	projectDir := filepath.Join(sb.Root, "myproject")
	skillsDir := filepath.Join(projectDir, ".skillshare", "skills")
	agentsDir := filepath.Join(projectDir, ".skillshare", "agents")
	os.MkdirAll(skillsDir, 0755)
	os.MkdirAll(agentsDir, 0755)

	// Create a skill
	skillDir := filepath.Join(skillsDir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n# Content"), 0644)

	// Create an agent
	os.WriteFile(filepath.Join(agentsDir, "tutor.md"), []byte("# Tutor agent"), 0644)

	// Write project config with a target that has agent path
	claudeAgents := filepath.Join(projectDir, ".claude", "agents")
	os.MkdirAll(claudeAgents, 0755)
	claudeSkills := filepath.Join(projectDir, ".claude", "skills")
	os.MkdirAll(claudeSkills, 0755)

	configContent := `targets:
  - name: claude
    skills:
      path: ` + claudeSkills + `
    agents:
      path: ` + claudeAgents + `
`
	os.WriteFile(filepath.Join(projectDir, ".skillshare", "config.yaml"), []byte(configContent), 0644)

	// Global config (needed by CLI)
	sb.WriteConfig(`source: ` + sb.SourcePath + "\ntargets: {}\n")

	return projectDir
}

// --- status -p (always shows skills + agents) ---

func TestStatusProject_ShowsAgents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	result := sb.RunCLIInDir(projectDir, "status", "-p")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "Source") // skill section
	result.AssertAnyOutputContains(t, "Agents") // agent section
	result.AssertAnyOutputContains(t, "1 agents")
}

func TestStatusProject_JSON_IncludesAgents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	result := sb.RunCLIInDir(projectDir, "status", "-p", "--json")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, `"agents"`)
	result.AssertAnyOutputContains(t, `"count"`)
}

// --- check -p agents ---

func TestCheckProject_Agents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	result := sb.RunCLIInDir(projectDir, "check", "-p", "agents")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "tutor")
	result.AssertAnyOutputContains(t, "local")
}

func TestCheckProject_Agents_JSON(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	result := sb.RunCLIInDir(projectDir, "check", "-p", "agents", "--json")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, `"name"`)
	result.AssertAnyOutputContains(t, `"status"`)
}

// --- diff -p agents ---

func TestDiffProject_Agents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	// Before sync, diff should show agents as "add"
	result := sb.RunCLIInDir(projectDir, "diff", "-p", "agents", "--no-tui")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "tutor")
}

func TestDiffProject_Agents_JSON(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	result := sb.RunCLIInDir(projectDir, "diff", "-p", "agents", "--json")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, `"agent"`)
}

// --- collect -p agents ---

func TestCollectProject_Agents_NoLocal(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	// Sync agents first
	sb.RunCLIInDir(projectDir, "sync", "-p", "agents")

	// No local agents to collect
	result := sb.RunCLIInDir(projectDir, "collect", "-p", "agents")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "No local agents")
}

func TestCollectProject_Agents_CollectsLocal(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	// Create a local agent directly in target (not via sync)
	claudeAgents := filepath.Join(projectDir, ".claude", "agents")
	os.WriteFile(filepath.Join(claudeAgents, "local-agent.md"), []byte("# Local"), 0644)

	result := sb.RunCLIInDir(projectDir, "collect", "-p", "agents")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "collected")

	// Verify copied to project agents source
	agentsSource := filepath.Join(projectDir, ".skillshare", "agents")
	if _, err := os.Stat(filepath.Join(agentsSource, "local-agent.md")); err != nil {
		t.Error("local-agent.md should be collected to project agents source")
	}
}

// --- audit -p agents ---

func TestAuditProject_Agents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	result := sb.RunCLIInDir(projectDir, "audit", "-p", "agents")
	result.AssertSuccess(t)
	// Audit should scan agents, not error
	result.AssertOutputNotContains(t, "not yet supported")
}

func TestSyncProject_All_NestedAgentsSameBasename_FlattensAndStaysStable(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := filepath.Join(sb.Root, "nested-agents-project")
	skillsDir := filepath.Join(projectDir, ".skillshare", "skills")
	agentsDir := filepath.Join(projectDir, ".skillshare", "agents")
	claudeAgents := filepath.Join(projectDir, ".claude", "agents")
	cursorAgents := filepath.Join(projectDir, ".cursor", "agents")
	claudeSkills := filepath.Join(projectDir, ".claude", "skills")
	cursorSkills := filepath.Join(projectDir, ".cursor", "skills")

	for _, dir := range []string{
		filepath.Join(skillsDir, "sample-skill"),
		filepath.Join(agentsDir, "team-a"),
		filepath.Join(agentsDir, "team-b"),
		claudeAgents,
		cursorAgents,
		claudeSkills,
		cursorSkills,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(skillsDir, "sample-skill", "SKILL.md"), []byte("---\nname: sample-skill\n---\n# Sample"), 0o644); err != nil {
		t.Fatalf("write sample skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "team-a", "helper.md"), []byte("# Team A"), 0o644); err != nil {
		t.Fatalf("write team-a helper: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "team-b", "helper.md"), []byte("# Team B"), 0o644); err != nil {
		t.Fatalf("write team-b helper: %v", err)
	}

	configContent := `targets:
  - name: claude
    skills:
      path: ` + claudeSkills + `
    agents:
      path: ` + claudeAgents + `
  - name: cursor
    skills:
      path: ` + cursorSkills + `
    agents:
      path: ` + cursorAgents + `
`
	if err := os.WriteFile(filepath.Join(projectDir, ".skillshare", "config.yaml"), []byte(configContent), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	sb.WriteConfig(`source: ` + sb.SourcePath + "\ntargets: {}\n")

	first := sb.RunCLIInDir(projectDir, "sync", "-p", "--all")
	first.AssertSuccess(t)
	first.AssertAnyOutputContains(t, "Agent sync complete")
	first.AssertAnyOutputContains(t, "0 updated")

	second := sb.RunCLIInDir(projectDir, "sync", "-p", "--all")
	second.AssertSuccess(t)
	second.AssertAnyOutputContains(t, "Agent sync complete")
	second.AssertAnyOutputContains(t, "0 updated")

	for _, base := range []string{claudeAgents, cursorAgents} {
		for _, name := range []string{"team-a__helper.md", "team-b__helper.md"} {
			if _, err := os.Lstat(filepath.Join(base, name)); err != nil {
				t.Fatalf("expected synced agent %s in %s: %v", name, base, err)
			}
		}
	}
}

// --- default -p (skills only, unchanged) ---

func TestStatusProject_Default_ShowsBoth(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	projectDir := setupProjectWithAgents(t, sb)

	// status now always shows both skills and agents
	result := sb.RunCLIInDir(projectDir, "status", "-p")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "Source")
	result.AssertAnyOutputContains(t, "Agents")
}
