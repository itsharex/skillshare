//go:build !online

package integration

import (
	"testing"

	"skillshare/internal/testutil"
)

func TestList_Agents_KindFilter_NoTUI(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	createAgentSource(t, sb, map[string]string{
		"tutor.md": "# Tutor agent",
	})
	sb.CreateSkill("my-skill", map[string]string{
		"SKILL.md": "---\nname: my-skill\n---\n# Content",
	})
	sb.WriteConfig(`source: ` + sb.SourcePath + "\ntargets: {}\n")

	// list agents --no-tui should show only agents
	result := sb.RunCLI("list", "agents", "--no-tui")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "tutor")
	result.AssertOutputNotContains(t, "my-skill")

	// list all --no-tui should show both
	result = sb.RunCLI("list", "all", "--no-tui")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "tutor")
	result.AssertAnyOutputContains(t, "my-skill")

	// list (default) --no-tui should show only skills
	result = sb.RunCLI("list", "--no-tui")
	result.AssertSuccess(t)
	result.AssertOutputNotContains(t, "tutor")
	result.AssertAnyOutputContains(t, "my-skill")
}

func TestTrash_MergedList_IncludesAgents(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()

	createAgentSource(t, sb, map[string]string{
		"tutor.md": "# Tutor",
	})
	sb.WriteConfig(`source: ` + sb.SourcePath + "\ntargets: {}\n")

	// Uninstall agent to move to trash
	sb.RunCLI("uninstall", "agents", "tutor", "--force")

	// Trash agents list --no-tui should show the agent
	result := sb.RunCLI("trash", "agents", "list", "--no-tui")
	result.AssertSuccess(t)
	result.AssertAnyOutputContains(t, "tutor")
}
