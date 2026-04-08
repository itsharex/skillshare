//go:build !online

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/testutil"
)

// --- status (always shows skills + agents) ---

func TestStatus_ShowsAgentInfo(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	createAgentSource(t, sb, map[string]string{
		"tutor.md":    "# Tutor agent",
		"reviewer.md": "# Reviewer agent",
	})
	claudeAgents := createAgentTarget(t, sb, "claude")

	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets:
  claude:
    skills:
      path: ` + sb.CreateTarget("claude") + `
    agents:
      path: ` + claudeAgents + `
`)

	result := sb.RunCLI("status")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "2 agents") // Source section
	result.AssertAnyOutputContains(t, "agents")   // Targets sub-item
	result.AssertAnyOutputContains(t, "linked")   // agent sync status
}

func TestStatus_JSON_IncludesAgents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	createAgentSource(t, sb, map[string]string{
		"tutor.md": "# Tutor agent",
	})

	sb.WriteConfig(`source: ` + sb.SourcePath + "\ntargets: {}\n")

	result := sb.RunCLI("status", "--json")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, `"agents"`)
	result.AssertAnyOutputContains(t, `"count"`)
}

func TestStatus_Default_ShowsBothSkillsAndAgents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	sb.CreateSkill("my-skill", map[string]string{
		"SKILL.md": "---\nname: my-skill\n---\n# Content",
	})
	createAgentSource(t, sb, map[string]string{
		"tutor.md": "# Tutor agent",
	})
	sb.WriteConfig(`source: ` + sb.SourcePath + "\ntargets: {}\n")

	result := sb.RunCLI("status")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "Source")   // source section with skills + agents
	result.AssertAnyOutputContains(t, "1 skills") // skills in source
	result.AssertAnyOutputContains(t, "1 agents") // agents in source
}

// --- diff agents ---

func TestDiff_Agents_JSON_IncludesKind(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	sb.CreateSkill("my-skill", map[string]string{
		"SKILL.md": "---\nname: my-skill\n---\n# Content",
	})
	claudeSkills := sb.CreateTarget("claude")

	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets:
  claude:
    skills:
      path: ` + claudeSkills + `
`)

	// Diff before sync should show items with kind field
	result := sb.RunCLI("diff", "--json")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, `"kind"`)
	result.AssertAnyOutputContains(t, `"skill"`)
}

// --- doctor agents ---

func TestDoctor_ChecksAgentSource(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	createAgentSource(t, sb, map[string]string{
		"tutor.md": "# Tutor agent",
	})
	sb.WriteConfig(`source: ` + sb.SourcePath + "\ntargets: {}\n")

	result := sb.RunCLI("doctor")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "Agents source")
	result.AssertAnyOutputContains(t, "1 agents")
}

func TestDoctor_AgentTargetDrift(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	createAgentSource(t, sb, map[string]string{
		"tutor.md":    "# Tutor agent",
		"reviewer.md": "# Reviewer agent",
	})
	claudeAgents := createAgentTarget(t, sb, "claude")

	// Only sync one agent manually (create symlink for tutor only)
	agentsDir := filepath.Join(filepath.Dir(sb.SourcePath), "agents")
	os.Symlink(
		filepath.Join(agentsDir, "tutor.md"),
		filepath.Join(claudeAgents, "tutor.md"),
	)

	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets:
  claude:
    skills:
      path: ` + sb.CreateTarget("claude") + `
    agents:
      path: ` + claudeAgents + `
`)

	result := sb.RunCLI("doctor")
	result.AssertSuccess(t)
	// Should detect drift (1/2 linked)
	result.AssertAnyOutputContains(t, "drift")
}

func TestDoctor_AgentTargetSynced(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	createAgentSource(t, sb, map[string]string{
		"tutor.md": "# Tutor agent",
	})
	claudeAgents := createAgentTarget(t, sb, "claude")

	sb.WriteConfig(`source: ` + sb.SourcePath + `
targets:
  claude:
    skills:
      path: ` + sb.CreateTarget("claude") + `
    agents:
      path: ` + claudeAgents + `
`)

	// Sync agents first
	sb.RunCLI("sync", "agents")

	result := sb.RunCLI("doctor")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "1 agents")
	result.AssertOutputNotContains(t, "drift")
}
