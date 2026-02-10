<h1 align="center">skelly 🦴</h1>

Generate LLM-friendly codebase structure maps. Extract the skeleton of your code—functions, classes, dependencies, and call graphs—into token-efficient text files.

## Why?

LLMs need context to understand your codebase, but sending full source files wastes tokens and context window. Skelly creates a compressed, structured representation that captures:

- Function/method signatures
- Class/struct definitions  
- Import dependencies
- Call relationships
- Symbol importance (via PageRank)

The output is ~5-10% of your codebase size while preserving the information LLMs need for refactoring, code review, and understanding.

## Installation

```bash
go install github.com/morozRed/skelly/cmd/skelly@latest
```

## Usage

```bash
# First run: initialize and generate context
skelly init

# Initialize .skelly/.context/ in your project (skip auto-generate)
skelly init --no-generate

# Initialize + generate LLM adapter files (Codex/Claude/Cursor)
skelly init --llm codex,claude,cursor

# Note: --llm writes adapter/context files only.

# Generate context files (all supported languages)
skelly generate

# Generate JSONL artifacts for RAG/LLM pipelines
skelly generate --format jsonl

# Generate only selected languages
skelly generate --lang go --lang python

# Update only changed files (incremental)
skelly update

# Incremental update with JSONL output
skelly update --format jsonl

# Agent-authored symbol description
# target accepts file path, file:symbol, file:line, or stable symbol id
skelly enrich internal/parser/parser.go:ParseDirectory "Parses a directory and normalizes symbol metadata for indexing."

# Show what update would regenerate
skelly status

# Validate setup + staleness + integrations
skelly doctor

# Explain why each file is impacted
skelly update --explain

# Machine-readable output for CI
skelly update --json

# Navigation primitives
skelly symbol Login
skelly symbol Logn --fuzzy --limit 5
skelly callers Login
skelly callers Login --lsp
skelly callees Login
skelly trace Login --depth 2
skelly path Login ValidateToken
skelly definition internal/cli/root.go:11
skelly references RunDoctor

# Install git pre-commit hook for auto-updates
skelly install-hook
```

## Output Structure

```
.skelly/
└── .context/
    ├── .state.json        # File hashes, snapshots, deps, output hashes
    ├── index.txt          # (text format) overview: key symbols, file list
    ├── graph.txt          # (text format) dependency adjacency list
    ├── modules/           # (text format) per-module breakdown
    ├── symbols.jsonl      # (jsonl format) one symbol record per line
    ├── edges.jsonl        # (jsonl format) one edge record per line
    ├── manifest.json      # (jsonl format) schema version + counts + hashes
    ├── nav-index.json     # navigation index for symbol/callers/callees/trace/path
    ├── search-index.json  # BM25 search index for fuzzy symbol lookup
    └── enrich.jsonl       # (enrich command) symbol enrichment records
```

## Example Output

```
# Module: auth

## auth/login.go
imports: [context, errors, db/users, utils/crypto]

### Login [func]
sig: func Login(ctx context.Context, email, password string) (*Session, error)
calls: [users.go:FindByEmail, crypto.go:VerifyHash, session.go:Create]
called_by: [api/handlers.go:HandleLogin]

### Logout [func]
sig: func Logout(ctx context.Context, sessionID string) error
calls: [session.go:Delete]
```

## Verified On This Repo

Ran on `github.com/morozRed/skelly` itself on February 5, 2026:

```bash
go run ./cmd/skelly generate --format jsonl
go run ./cmd/skelly update --format jsonl
```

Observed:

- `generate`: `scanned=25 parsed=25 rewritten=3 duration=93ms`
- `update` (no source changes): `scanned=25 parsed=0 reused=25 rewritten=0 duration=19ms`
- `.skelly/.context/manifest.json` reported:
  - `files: 25`
  - `symbols: 275`
  - `edges: 292`

## Supported Languages

- Go
- Python
- Ruby
- TypeScript/JavaScript

## Architecture

- Pipeline map and component boundaries: `docs/ARCHITECTURE.md`

## Configuration

Create `.skellyignore` (same syntax as `.gitignore`) to exclude files:

```
*_test.go
vendor/
fixtures/
```

Built-in excludes are applied by default (`.git/`, `.skelly/`, `.context/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, `__pycache__/`) and can be overridden with negation rules in `.skellyignore`.

`skelly enrich` is agent-facing annotation UX. It updates exactly one symbol entry in `.skelly/.context/enrich.jsonl`:

```bash
skelly enrich <target> "<description>"
```

`<target>` supports:
- `path/to/file.go`
- `path/to/file.go:SymbolName`
- `path/to/file.go:123`
- stable symbol id (`path|line|kind|name|hash`)

## Current Behavior

- Incremental updates parse only changed/new files and reuse cached symbol snapshots for unchanged files.
- `--format text|jsonl` is supported for `generate` and `update` (default: `text`).
- `enrich <target> "<description>"` writes one manual/agent-provided symbol description.
- `setup` is deprecated (hidden); use `init` instead.
- `init` creates `.skelly/.context/`, optionally generates LLM adapter files, and auto-runs `generate` unless `--no-generate` is passed.
- `init --llm ...` generates managed LLM adapter files (`AGENTS.md`, `CLAUDE.md`, `.cursor/rules/skelly-context.mdc`) plus `CONTEXT.md`.
- `doctor` reports setup health, stale context, and suggested remediation commands.
- `doctor --json` reports optional LSP capability probes per supported language.
- Navigation commands (`symbol`, `callers`, `callees`, `trace`, `path`, `definition`, `references`) read from `.skelly/.context/nav-index.json`.
- `callers/callees/trace/path/definition/references --lsp` keeps parser output as source of truth and annotates output provenance (`source=parser`) with LSP capability metadata.
- `symbol --fuzzy` uses BM25 ranking over `name`, `signature`, `file`, and `doc` via `.skelly/.context/search-index.json`.
- `enrich` stores symbol records in `.skelly/.context/enrich.jsonl` and upserts by cache key.
- State includes parser versioning, per-file hashes, per-file symbols/imports, dependency links, and generated output hashes.
- Calls are stored as structured call sites (name, qualifier/receiver, arity, line, raw expression).
- Graph edges include confidence metadata (`resolved`, `heuristic`); ambiguous candidates stay unresolved (no edge).
- Resolver order is strict: receiver/scope -> same file -> import alias/module -> global fallback.
- Outputs are deterministic (stable symbol IDs, sorted files/symbols/edges) to minimize noisy diffs.

## Current Limitations

- Call resolution is scope-aware at file/module level but still heuristic; deep type-aware resolution is not implemented.
- `skelly update` currently rebuilds graph outputs from the full cached symbol set (not impacted subset only).
- `.skellyignore` aims for gitignore-like behavior but does not implement every edge case from Git's matcher.

## Performance Benchmark

Run:

```bash
# Parse + graph throughput
go test -bench BenchmarkParseAndGraph_MediumRepo ./internal/bench -run ^$ -benchmem

# Resolver quality and navigation usability metrics
go test -bench BenchmarkResolverQuality_Curated ./internal/bench -run ^$ -benchmem
go test -bench BenchmarkNavigationUsability_CommonQueries ./internal/bench -run ^$ -benchmem
```

## Agent A/B Benchmark

Use the OpenCode harness under `benchmark/agent_ab/` to compare agent outcomes with and without Skelly context:

```bash
# One command: run + collect + analyze
benchmark/agent_ab/scripts/run.sh --suite benchmark/agent_ab/tasks/suite.example.json --repeats 1 --collect-analyze

# Optional: dry-run workspace preparation only
benchmark/agent_ab/scripts/run.sh --suite benchmark/agent_ab/tasks/suite.example.json --repeats 1 --dry-run

# Optional: manual post-processing
benchmark/agent_ab/scripts/collect.sh --results-dir benchmark/agent_ab/results/<run_id>
benchmark/agent_ab/scripts/analyze.sh --input benchmark/agent_ab/results/<run_id>/collected.jsonl
```

`run.sh` uses your normal OpenCode config by default (`--opencode-state system`); pass `--opencode-state isolated` for isolated benchmark state.  
Set `--model provider/model` explicitly for reproducible runs. The harness fails fast on deprecated models (for example `anthropic/claude-opus-4-6`).
Only generated benchmark artifacts under `benchmark/agent_ab/results/` are intended to be git-ignored.

### Latest Benchmark Snapshot

From a local run on February 9, 2026 (`--repeats 5`, task `cli_update_incremental_jsonl`):

- Runs: 10 total (5 `with_skelly`, 5 `without_skelly`)
- Success rate: 100% for both arms
- Median duration: 22s (`with_skelly`) vs 58s (`without_skelly`)
- Median non-cache tokens: 22,858 (`with_skelly`) vs 79,423 (`without_skelly`)
- Median tool calls: 2 (`with_skelly`) vs 12 (`without_skelly`)

Reproduce:

```bash
benchmark/agent_ab/scripts/run.sh \
  --suite benchmark/agent_ab/tasks/suite.skelly_advantage.json \
  --repeats 5 \
  --model openai/gpt-5-mini \
  --collect-analyze
```

## v0.1 Exit Criteria

- Correct: parser/graph/state unit + integration tests pass.
- Deterministic: repeated runs produce stable IDs and stable output files.
- Incremental: `skelly update` parses changed files and reuses cached snapshots.
- Operational: CI runs `go vet`, `go test`, lint, and snapshot-style integration checks.

## v0.2 Exit Criteria

- Correctness hardening fixes are covered by regression tests.
- DX setup supports `init --llm ...` and `doctor`.
- Navigation primitives (`symbol`, `callers`, `callees`, `trace`, `path`) are available with `--json`.
- Enrich supports agent-authored per-symbol descriptions via `skelly enrich <target> "<description>"`.
- Benchmarks report:
  - resolver precision/recall on curated fixtures
  - query-pack token footprint for common navigation flows

## Roadmap

- [ ] v0.1: Basic parsing and output
- [ ] v0.2: Call graph extraction (not just definitions)
- [ ] v0.3: BM25 index for fast symbol lookup
- [ ] v0.4: Quantized embeddings for semantic search
- [ ] v0.5: MCP server mode

## License

MIT
