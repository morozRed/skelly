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

## Phase 7 - Symbol Retrieval Quality (v0.3 Priority 0)

- [ ] Implement BM25-backed symbol retrieval over `name`, `signature`, `file`, and `doc`.
- [ ] Keep exact ID/name lookup as primary path; use BM25 as ranked fallback.
- [ ] Add `skelly symbol --fuzzy` and `--limit` with stable deterministic ordering.
- [ ] Persist a compact search index under `.skelly/.context/` and rebuild it during `generate`/`update`.
- [ ] Add tests for typo tolerance, partial-name matching, and deterministic ranking.

## Phase 8 - Optional LSP Foundation (v0.3 Priority 0)

- [ ] Add `internal/lsp` adapter package with capability probe, definition lookup, and references lookup.
- [ ] Map supported languages to preferred servers:
- [ ] Go -> `gopls`
- [ ] Python -> `pyright-langserver` (fallback `pylsp`)
- [ ] TypeScript/JavaScript -> `typescript-language-server`
- [ ] Ruby -> `solargraph`
- [ ] Extend `skelly doctor` JSON/text output with per-language LSP availability and reason fields.
- [ ] Ensure missing LSP servers never fail default parser-first workflows.

## Phase 9 - LSP-Enriched Navigation (Opt-In, Non-Core)

- [ ] Add `--lsp` flag to `callers`, `callees`, `trace`, and `path`.
- [ ] Keep default behavior unchanged when `--lsp` is not set.
- [ ] Annotate output provenance (`source=parser|lsp`) and confidence consistently in text and JSON.
- [ ] Add fallback messaging that points users to `skelly doctor` when LSP backends are unavailable.
- [ ] Add regression tests asserting identical output without `--lsp`.

## Phase 10 - Typed Context and Resolver Hardening

- [ ] Extend call/edge metadata to carry typed receiver hints when parser data can infer them.
- [ ] Preserve parser confidence semantics while adding explicit provenance for hybrid edges.
- [ ] Refine unresolved/heuristic edges with optional LSP evidence only for changed/impacted scope.
- [ ] Add fixture cases for dynamic dispatch ambiguity and same-name symbols across modules.
- [ ] Track and report resolved vs heuristic edge ratios in benchmark outputs.

## Phase 11 - Incremental Performance and Scale

- [ ] Reduce full-output regeneration work in `update` by reusing unaffected artifacts where safe.
- [ ] Add latency budgets for optional LSP refinement (`p50`, `p95`) and enforce timeout guardrails.
- [ ] Add query/result caching for LSP lookups keyed by language, file hash, symbol, and query type.
- [ ] Add large-repo benchmark scenarios with mixed-language trees and high fan-out call graphs.

## Phase 12 - Benchmark Expansion and Adoption Gates

- [ ] Extend agent A/B suite with BM25 symbol-discovery tasks and optional-LSP navigation tasks.
- [ ] Add quality metrics: symbol search recall@k, navigation precision/recall, ambiguity resolution rate.
- [ ] Preserve existing primary gates: success-rate, runtime, and non-cache token advantage.
- [ ] Require at least 5 repeats per task-arm pair before promotion decisions; prefer 10 for noisy suites.
- [ ] Publish v0.3 exit criteria and rollout recommendation from benchmark evidence.
