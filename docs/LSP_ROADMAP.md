# LSP Integration Roadmap

## Goal

Add optional Language Server Protocol assistance to improve symbol precision and navigation quality, while keeping Skelly's parser-first workflow fast and deterministic.

## Principles

- Keep existing parser + graph pipeline as default.
- Make LSP usage opt-in and capability-driven.
- Fall back gracefully when no language server is available.
- Avoid introducing non-deterministic output into core artifacts by default.

## Phase 1: Environment Detection (Doctor)

### Scope

- Extend `skelly doctor` to report LSP availability by language.
- Check common binaries in `PATH`:
  - Go: `gopls`
  - Python: `pyright-langserver` / `pylsp`
  - TypeScript: `typescript-language-server`
  - Ruby: `solargraph`

### Output

- Add `lsp` section in doctor JSON with per-language availability.
- Print concise text summary in non-JSON doctor mode.

### Success Criteria

- `doctor` clearly indicates which LSP backends are available/missing.
- No behavior change for existing commands.

## Phase 2: Opt-in LSP Query Commands

### Scope

- Add optional commands for high-value targeted lookups:
  - `skelly definition <symbol|file:line>`
  - `skelly references <symbol|file:line>`
- Resolve IDs through existing nav index first, then enrich with LSP results.

### Flags

- `--lsp` to enable language-server lookup.
- `--json` output parity with existing nav commands.

### Success Criteria

- Commands work without LSP (degrade to existing behavior/error hints).
- With LSP available, query precision improves for cross-file resolution cases.

## Phase 3: Hybrid Edge Refinement During Update

### Scope

- Keep parser-derived graph as base.
- Use LSP only to refine unresolved/heuristic edges in changed/impacted files.
- Store provenance on refined edges (e.g., `confidence=lsp`).

### Guardrails

- Limit runtime with per-language timeout budgets.
- Cache LSP query results by file hash.
- Make feature opt-in via `skelly update --lsp-refine`.

### Success Criteria

- Higher ratio of resolved edges on complex codebases.
- Incremental runtime impact remains bounded and measurable.

## Benchmark Plan

- Compare parser-only vs parser+LSP on:
  - edge confidence distribution
  - navigation accuracy (sampled symbol queries)
  - update duration
  - downstream agent token usage on benchmark tasks

## Open Questions

- Should refined edges be persisted in `.state.json` or generated transiently?
- Which language servers should be first-class supported vs best-effort?
- How to normalize symbol identity across parser and LSP ecosystems?
