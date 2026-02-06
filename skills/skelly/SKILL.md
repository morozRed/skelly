---
name: skelly
description: Use the Skelly CLI to build or update code context, query symbol and call-graph navigation, and attach per-symbol descriptions via `skelly enrich` with target and description arguments. Trigger when tasks require fast codebase mapping or targeted symbol annotation before broad file reads.
---

# Skelly Skill

Use this skill when working in repositories that use `skelly` for codebase context.

## Quick Rules

- Prefer Skelly artifacts and navigation commands before opening many source files.
- Keep context fresh before graph-based queries.
- Use `skelly enrich <target> <description>` to store agent-authored symbol descriptions in `.skelly/.context/enrich.jsonl`.

## Workflow

1. Validate context health:
   - `skelly doctor`
2. Refresh context when needed:
   - First run: `skelly init && skelly generate`
   - Incremental: `skelly update`
3. Query graph/navigation to find exact symbols and file positions:
   - `skelly symbol <name|id>`
   - `skelly callers <name|id>`
   - `skelly callees <name|id>`
   - `skelly trace <name|id> --depth 2`
   - `skelly path <from> <to>`
4. Add targeted symbol descriptions as you analyze:
   - `skelly enrich <target> "<description>"`

## Enrich Target Formats

Use one of:

- `path/to/file.go`
- `path/to/file.go:SymbolName`
- `path/to/file.go:123`
- Stable symbol id: `path|line|kind|name|hash`

If target is ambiguous, refine with `file:symbol` or `file:line`.

## JSON Mode

When machine-readable output is needed, prefer `--json` on query commands:

- `skelly symbol Login --json`
- `skelly callers Login --json`
- `skelly update --json`

## Artifacts To Prefer

- `.skelly/.context/manifest.json`
- `.skelly/.context/symbols.jsonl`
- `.skelly/.context/edges.jsonl`
- `.skelly/.context/nav-index.json`
- `.skelly/.context/enrich.jsonl`

## Constraints

- `skelly enrich` depends on generated state; run `skelly generate` first if state is missing.
- Call graph resolution is heuristic; verify uncertain edges in source before risky edits.
