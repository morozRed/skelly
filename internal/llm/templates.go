package llm

import "fmt"

func BuildSkellySkillContent() string {
	return `# Skelly Skill

Use Skelly context artifacts and commands before opening many source files.

Workflow:
1. Run skelly doctor to validate context freshness.
2. If stale, run skelly update.
3. Prefer .skelly/.context/manifest.json, symbols.jsonl, and edges.jsonl when present.
4. Fall back to index.txt and graph.txt for text mode repos.
5. Use skelly status before major changes to understand impacted files.
`
}

func BuildContextBlock() string {
	return `# Skelly Context

This repository uses Skelly as the canonical code context layer for LLM tools.

- Canonical skill instructions: .skelly/skills/skelly.md
- Context directory: .skelly/.context/
- Preferred machine-readable artifacts:
  - .skelly/.context/symbols.jsonl
  - .skelly/.context/edges.jsonl
  - .skelly/.context/manifest.json

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
4. Use CONTEXT.md for canonical artifact paths.
`, agentName)
}

func BuildCursorRuleContent() string {
	return `---
description: Use Skelly context artifacts for code navigation
alwaysApply: true
---

Run skelly doctor first. If context is stale, run skelly update.
Use .skelly/skills/skelly.md and CONTEXT.md as primary guidance.
Prefer .skelly/.context/symbols.jsonl, .skelly/.context/edges.jsonl, and .skelly/.context/manifest.json when available.
`
}
