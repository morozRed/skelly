package llm

import "fmt"

func BuildSkellySkillContent() string {
	return `# Skelly Skill

Use Skelly CLI commands before opening many source files.

Workflow:
1. Run skelly doctor to validate context freshness.
2. If stale, run skelly update.
3. Use navigation commands first: skelly symbol, skelly callers, skelly callees, skelly trace, skelly path.
4. Use skelly status before major changes to understand impacted files.
5. Avoid reading .skelly/.context/* files directly unless debugging or CLI output is insufficient.
`
}

func BuildContextBlock() string {
	return `# Skelly Context

This repository uses Skelly as the canonical code context layer for LLM tools.

- Canonical skill instructions: .skelly/skills/skelly.md
- Context directory: .skelly/.context/
- Preferred workflow: use Skelly CLI commands for navigation and impact analysis.
  - skelly symbol <target>
  - skelly callers <target>
  - skelly callees <target>
  - skelly trace <target>
  - skelly path <from> <to>
  - skelly status
- Treat .skelly/.context/* files as implementation detail; avoid direct reads unless debugging.

Recommended command sequence:
1. skelly doctor
2. skelly update (if doctor reports stale context)
3. skelly status (to inspect impact)
`
}

func BuildRootAdapterBlock(agentName string) string {
	return fmt.Sprintf(`# Skelly Integration (%s)

Use Skelly outputs before broad code reads.

1. Run skelly doctor at session start.
2. If stale, run skelly update.
3. Follow .skelly/skills/skelly.md.
4. Prefer Skelly CLI commands over direct reads of .skelly/.context/* files.
`, agentName)
}

func BuildCursorRuleContent() string {
	return `---
description: Use Skelly CLI for code navigation
alwaysApply: true
---

Run skelly doctor first. If context is stale, run skelly update.
Use .skelly/skills/skelly.md and CONTEXT.md as primary guidance.
Prefer Skelly CLI commands (symbol/callers/callees/trace/path/status) for navigation and impact analysis.
Avoid direct reads of .skelly/.context/* unless debugging or CLI output is insufficient.
`
}
