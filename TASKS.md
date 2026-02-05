# Skelly Tasks (Next Roadmap)

Goal: make Skelly the default code-navigation context layer for LLM agents with high trust, low friction, and predictable outputs.

## Phase 1 - Correctness Hardening (Priority 0)

- [x] Fix pre-commit hook format drift: preserve selected output format (`text` vs `jsonl`) when hook runs `skelly update`.
- [x] Fix Python `from ... import ... as ...` alias extraction so imported member aliases resolve calls.
- [x] Fix TypeScript named-import parsing for brace groups (`import { a, b as c } ...`).
- [x] Fix TypeScript signature rendering to avoid double-colon return type output.
- [x] Fix graph resolver fallback for qualified receiver calls when alias lookup misses (continue broader lookup).
- [x] Fix PageRank sink handling by redistributing dangling-node rank mass each iteration.
- [x] Add regression tests and fixtures for every issue above.

## Phase 2 - DX: Zero-Prompt Agent Integration

- [x] Add `skelly init --llm codex,claude,cursor` to generate root-level integration hints.
- [x] Generate/update agent-facing files (`AGENTS.md`, `CLAUDE.md`, Cursor rules) that instruct tools to use Skelly outputs automatically.
- [x] Add root `CONTEXT.md` pointing to canonical context artifacts and common commands.
- [x] Add `skelly doctor` to validate setup, detect stale context, and suggest exact fix commands.
- [x] Ensure generated integration files are idempotent and safe to re-run.

## Phase 3 - Navigation Primitives for LLMs

- [x] Add `skelly symbol <name|id>` for direct symbol lookup.
- [x] Add `skelly callers <symbol>` and `skelly callees <symbol>` with confidence metadata.
- [x] Add `skelly trace <symbol> --depth N` for bounded multi-hop traversal.
- [x] Add `skelly path <from> <to>` for shortest call-path lookup when resolvable.
- [x] Support `--json` on navigation commands for tool/agent consumption.
- [x] Persist a compact adjacency index for fast traversal without full regeneration.

## Phase 4 - Enrichment UX and First-Run Value

- [x] During `skelly setup`, prompt for optional first-run enrich of all symbols.
- [x] Add enrich cache keys: `symbol_id + file_hash + prompt_version + agent_profile + model`.
- [x] Re-enrich only stale or missing records on incremental runs.
- [x] Store enrich metadata (`model`, `profile`, `prompt_version`, `updated_at`, `confidence`) in `enrich.jsonl`.
- [x] Add graceful degradation for timeouts/invalid agent output with partial-success reporting.
- [x] Add mode to enrich top-ranked symbols first (PageRank priority), then expand.

## Phase 5 - Cleanup and Maintainability

- [x] Remove deprecated code paths and stale TODOs that no longer match command semantics.
- [x] Consolidate duplicate import-alias parsing helpers across language parsers where practical.
- [x] Tighten state/output schema versioning rules and migration tests.
- [x] Audit CLI help text and README examples for command/flag consistency.
- [x] Add a small “architecture map” doc describing parser -> graph -> state -> output pipeline boundaries.

## Phase 6 - Validation and Release Criteria (v0.2)

- [x] Add end-to-end tests for agent integration file generation and idempotent re-run behavior.
- [x] Add integration tests for navigation commands on multi-file fixture repos.
- [x] Add enrichment cache hit/miss integration tests.
- [x] Add quality benchmark: resolver precision/recall on curated fixtures.
- [x] Add usability benchmark: “tokens to answer” and latency for common code-navigation questions.
- [x] Define and publish v0.2 exit criteria (correctness, DX, traversal utility, enrich stability).
