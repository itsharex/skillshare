# CLI E2E Runbook: Target Local Agents Visibility

Validates that `ss target` output exposes local-only agents in both plain-text
and JSON modes instead of collapsing them into a misleading "no agents" state.

**Origin**: v0.19.x — target agent summaries now report local agent counts when
the source has zero agents, and the CLI needs regression coverage for that
display contract.

## Scope

- Global `ss target claude -g` shows `no source agents yet (1 local)` when the
  Claude target has a local-only agent file
- Global `ss target list --json -g` returns `agentLocalCount` alongside zero
  expected/linked counts for that case
- Project `ss target claude -p` mirrors the same local-only agent summary
- Project `ss target list --json -p` returns the same local-only agent metadata

## Environment

Run inside devcontainer via mdproof.
Use explicit `-g` and `-p` flags to avoid auto-mode ambiguity.

## Steps

### 1. Global target info shows local-only agents when source is empty

```bash
set -e
rm -f ~/.config/skillshare/agents/local-only.md ~/.claude/agents/local-only.md
mkdir -p ~/.claude/agents

cat > ~/.claude/agents/local-only.md <<'EOF'
---
name: local-only
description: Local-only target agent
---
# Local Only
EOF

ss target claude -g
```

Expected:
- exit_code: 0
- Agents:
- .claude/agents
- Status:  no source agents yet (1 local)

### 2. Global target list JSON reports agentLocalCount for local-only agents

```bash
set -e
rm -f ~/.config/skillshare/agents/local-only.md ~/.claude/agents/local-only.md
mkdir -p ~/.claude/agents

cat > ~/.claude/agents/local-only.md <<'EOF'
---
name: local-only
description: Local-only target agent
---
# Local Only
EOF

ss target list --json -g
```

Expected:
- exit_code: 0
- jq: (.targets[] | select(.name == "claude").agentLocalCount) == 1
- jq: (.targets[] | select(.name == "claude").agentExpectedCount) == 0
- jq: (.targets[] | select(.name == "claude").agentLinkedCount) == 0
- jq: (.targets[] | select(.name == "claude").agentPath | type) == "string"

### 3. Project target info shows local-only agents when project source is empty

```bash
set -e
PROJECT=/tmp/target-local-agents-project-info
rm -rf "$PROJECT"
mkdir -p "$PROJECT/.skillshare/skills" "$PROJECT/.skillshare/agents" "$PROJECT/.claude/agents"

cat > "$PROJECT/.skillshare/config.yaml" <<'EOF'
targets:
  - claude
EOF

cat > "$PROJECT/.claude/agents/local-only.md" <<'EOF'
---
name: local-only
description: Local-only project target agent
---
# Local Only
EOF

cd "$PROJECT"
ss target claude -p
```

Expected:
- exit_code: 0
- Agents:
- .claude/agents
- Status:  no source agents yet (1 local)

### 4. Project target list JSON reports agentLocalCount for local-only agents

```bash
set -e
PROJECT=/tmp/target-local-agents-project-json
rm -rf "$PROJECT"
mkdir -p "$PROJECT/.skillshare/skills" "$PROJECT/.skillshare/agents" "$PROJECT/.claude/agents"

cat > "$PROJECT/.skillshare/config.yaml" <<'EOF'
targets:
  - claude
EOF

cat > "$PROJECT/.claude/agents/local-only.md" <<'EOF'
---
name: local-only
description: Local-only project target agent
---
# Local Only
EOF

cd "$PROJECT"
ss target list --json -p
```

Expected:
- exit_code: 0
- jq: .targets | length == 1
- jq: (.targets[0].name) == "claude"
- jq: (.targets[0].agentLocalCount) == 1
- jq: (.targets[0].agentExpectedCount) == 0
- jq: (.targets[0].agentLinkedCount) == 0
- jq: (.targets[0].agentPath | endswith("/.claude/agents")) == true

## Pass Criteria

- Plain-text target info makes local-only agents visible in both global and
  project mode
- JSON target list includes `agentLocalCount` for local-only agent targets
- The CLI no longer implies "no agents" when the target actually contains local
  agent files
