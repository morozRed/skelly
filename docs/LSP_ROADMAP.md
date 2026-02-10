# LSP Integration Roadmap

## Goal

Add optional Language Server Protocol assistance to improve navigation and edge resolution, while preserving Skelly's parser-first deterministic outputs.

## Current Baseline (As Of February 9, 2026)

- `internal/cli/doctor.go` reports setup health and staleness, but no LSP capabilities.
- Navigation commands (`symbol/callers/callees/trace/path`) read only from `.skelly/.context/nav-index.json`.
- Graph confidence is currently parser-driven (`resolved`, `heuristic`) via `internal/graph/graph.go`.
- `update` already computes impacted subsets, then rebuilds full output from merged state snapshots.

This roadmap is adjusted to fit those seams directly.

## Principles

- Parser-first remains the default path for all commands.
- LSP usage is explicit and capability-driven.
- Missing LSP backends never hard-fail default workflows.
- Deterministic artifacts stay deterministic by default; LSP influence starts as transient.

## Phase 0: LSP Adapter Foundation (No Behavior Change)

### Scope

- Add `internal/lsp` package with a small provider interface:
  - capability probe
  - definition lookup
  - references lookup
- Add language-to-server mapping for currently supported parsers:
  - Go -> `gopls`
  - Python -> `pyright-langserver` (primary), `pylsp` (fallback)
  - TypeScript/JavaScript -> `typescript-language-server`
  - Ruby -> `solargraph`
- Define normalized result model that can map back to Skelly symbol identity (`file + line + symbol id`).

### Success Criteria

- New package compiles with no CLI behavior change.
- Unit tests cover capability probing and result normalization.

## Phase 1: Doctor Capability Reporting

### Scope

- Extend `DoctorSummary` in `internal/cli/summary.go` with an `lsp` field.
- In `RunDoctor`, report per-language:
  - language present in repository
  - detected server binary
  - available/unavailable reason
- Keep existing text output concise; add one summary line for LSP availability.

### JSON Shape (Target)

```json
"lsp": {
  "go": { "present": true, "server": "gopls", "available": true },
  "python": { "present": false, "server": "pyright-langserver", "available": false, "reason": "language_not_present" }
}
```

### Success Criteria

- `skelly doctor --json` includes stable LSP capability data.
- Existing doctor tests continue to pass with additive assertions.

## Phase 2: LSP-Enriched Navigation (Read-Only, Transient)

### Scope

- Add `--lsp` flag to existing nav commands first:
  - `callers`, `callees`, `trace`, `path`
- Resolve seed symbol through current nav index, then optionally augment traversal edges with LSP query results.
- Add explicit provenance in command output payloads (for example `confidence=lsp` or `source=lsp`), without changing persisted graph files yet.

### Why This Order

- Reuses existing UX and tests in `internal/cli/cli_test.go`.
- Avoids introducing two new top-level commands before proving integration quality.

### Success Criteria

- Without `--lsp`, outputs are unchanged.
- With `--lsp`, cross-file navigation precision improves on curated fixtures.

## Phase 3: Optional `definition` / `references` Commands

### Scope

- Add:
  - `skelly definition <symbol|file:line>`
  - `skelly references <symbol|file:line>`
- Default strategy:
  - resolve through nav index first
  - query LSP only when requested (`--lsp`) or when local resolution is ambiguous and user opted in
- Return JSON parity with existing command style.

### Success Criteria

- Commands degrade gracefully when no provider is available.
- Error messages include clear remediation (`run skelly doctor`, install server binary).

## Phase 4: Update-Time Edge Refinement (Opt-In)

### Scope

- Add `skelly update --lsp-refine`.
- Apply LSP only to changed/impacted sources already computed by update flow.
- Refine unresolved/heuristic edges only; parser-resolved edges remain authoritative unless confidence improves.

### Guardrails

- Per-language timeout budgets.
- Concurrency limits to avoid editor/server thrash.
- Query cache keyed by `(language, file hash, symbol id, query kind)`.

### Persistence Strategy

- Step A: transient refinement only (affects command/session output, not `.state.json` schema).
- Step B: persisted refinement metadata only after benchmark evidence justifies added complexity.

### Success Criteria

- Measurable increase in resolved edge ratio.
- Bounded update latency overhead.

## Benchmark And Quality Gates

- Compare parser-only vs `--lsp` / `--lsp-refine` on:
  - navigation accuracy on curated fixtures
  - resolved vs heuristic edge distribution
  - `update` latency and p95 overhead
  - benchmark token/call footprint in `benchmark/agent_ab`
- Add deterministic output checks ensuring default runs are unchanged when LSP flags are not used.

## Testing Plan

- Extend `internal/cli/cli_test.go`:
  - doctor JSON includes `lsp`
  - nav commands preserve behavior without `--lsp`
  - nav commands include LSP provenance when enabled
- Add focused tests for new `internal/lsp` adapter package.
- Add fixture-based integration tests for at least Go and TypeScript first.

## Rollout Order

1. Phase 0 (adapter + tests)
2. Phase 1 (doctor visibility)
3. Phase 2 (LSP flags on existing nav commands)
4. Phase 3 (definition/references)
5. Phase 4 (`update --lsp-refine`, transient first)

## Open Decisions

- Final confidence vocabulary: reuse `confidence` string vs add dedicated `source` field.
- Whether persisted LSP refinements live in `.state.json` or separate cache file.
- Which language servers become first-class CI-covered vs best-effort.
