# Skelly Architecture Map

This document describes the core pipeline and ownership boundaries in Skelly.

## Pipeline

1. `Parser` (`pkg/languages`, `internal/parser`)
   - Parses source files into `parser.FileSymbols`.
   - Extracts symbols, imports, import aliases, and call sites.
   - Normalizes symbol IDs and call site metadata.

2. `Graph` (`internal/graph`)
   - Builds graph nodes from symbols (`symbol_id` as node identity).
   - Resolves calls into edges with confidence labels.
   - Computes PageRank for symbol importance.

3. `State` (`internal/state`)
   - Stores per-file hash + symbol snapshots for incremental updates.
   - Stores dependency links for impacted-file closure.
   - Stores output hashes for rewrite minimization.

4. `Output` (`internal/output`)
   - Writes deterministic text or JSONL artifacts.
   - Maintains artifact sets per format and removes stale files.
   - Writes navigation index (`nav-index.json`) for fast query commands.

5. `CLI` (`cmd/skelly`)
   - Orchestrates `init/generate/update/status/doctor`.
   - Provides navigation commands: `symbol/callers/callees/trace/path`.
   - Runs enrich pipeline with cache-aware incremental behavior.

## Data Contracts

- Parse contract:
  - `parser.FileSymbols` is language-agnostic extraction output.
- Graph contract:
  - `graph.Node` contains symbol + in/out edges + confidence.
- State contract:
  - `.skelly/.context/.state.json` is the source of incremental truth.
- Context artifacts:
  - Text mode: `index.txt`, `graph.txt`, `modules/*.txt`.
  - JSONL mode: `symbols.jsonl`, `edges.jsonl`, `manifest.json`.
  - Navigation mode: `nav-index.json`.
  - Enrich cache/output: `enrich.jsonl`.

## Incremental Boundaries

- `generate`:
  - Full parse + full graph + full output rewrite-if-changed.
- `update`:
  - Parse changed files only.
  - Recompute impacted dependency closure.
  - Rebuild full output from merged state snapshots.
- `enrich`:
  - Targets changed/all files by scope.
  - Uses cache key:
    - `symbol_id + file_hash + prompt_version + agent_profile + model`
  - Re-enriches only misses/stale entries.

## Operational Invariants

- Deterministic output ordering.
- Stable symbol IDs across runs.
- Confidence-tagged edges (`resolved`, `heuristic`; ambiguous unresolved).
- Non-fatal parse issues surfaced as warnings.
