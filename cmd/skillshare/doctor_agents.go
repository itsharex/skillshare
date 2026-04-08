package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"skillshare/internal/config"
	"skillshare/internal/ui"
)

// checkAgentTargetInline validates the agent target for a single target,
// printing as an indented sub-item under the target name in doctor output.
func checkAgentTargetInline(name string, target config.TargetConfig, builtinAgents map[string]config.TargetConfig, agentCount int, result *doctorResult) {
	agentPath := resolveAgentTargetPath(target, builtinAgents, name)
	if agentPath == "" {
		return
	}

	info, err := os.Stat(agentPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("  agents   %s[merge] not created%s\n", ui.Gray, ui.Reset)
			result.addCheck("agent_target_"+name, checkPass,
				fmt.Sprintf("Agent target %s: not created yet", name), nil)
			return
		}
		fmt.Printf("  agents   %s[merge] error: %s%s\n", ui.Red, err.Error(), ui.Reset)
		result.addError()
		result.addCheck("agent_target_"+name, checkError,
			fmt.Sprintf("Agent target %s: %v", name, err), nil)
		return
	}

	if !info.IsDir() {
		fmt.Printf("  agents   %s[merge] error: not a directory%s\n", ui.Red, ui.Reset)
		result.addError()
		result.addCheck("agent_target_"+name, checkError,
			fmt.Sprintf("Agent target %s: path is not a directory", name), nil)
		return
	}

	linked, broken := countAgentLinksAndBroken(agentPath)
	if broken > 0 {
		msg := fmt.Sprintf("[merge] %d linked, %d broken", linked, broken)
		fmt.Printf("  agents   %s%s%s\n", ui.Yellow, msg, ui.Reset)
		result.addWarning()
		result.addCheck("agent_target_"+name, checkWarning,
			fmt.Sprintf("Agent target %s: %s", name, msg), nil)
		return
	}

	if linked != agentCount && agentCount > 0 {
		msg := fmt.Sprintf("[merge] drift (%d/%d linked)", linked, agentCount)
		fmt.Printf("  agents   %s%s%s\n", ui.Yellow, msg, ui.Reset)
		result.addWarning()
		result.addCheck("agent_target_"+name, checkWarning,
			fmt.Sprintf("Agent target %s: drift (%d/%d agents linked)", name, linked, agentCount), nil)
		return
	}

	msg := fmt.Sprintf("[merge] synced (%d/%d linked)", linked, agentCount)
	fmt.Printf("  agents   %s%s%s\n", ui.Green, msg, ui.Reset)
	result.addCheck("agent_target_"+name, checkPass,
		fmt.Sprintf("Agent target %s: %d agents synced", name, linked), nil)
}

// countAgentLinksAndBroken counts .md symlinks and broken symlinks in a directory.
func countAgentLinksAndBroken(dir string) (linked, broken int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		// It's a symlink — check if target exists (os.Stat follows symlinks)
		if _, statErr := os.Stat(filepath.Join(dir, e.Name())); statErr != nil {
			broken++
		} else {
			linked++
		}
	}
	return linked, broken
}
