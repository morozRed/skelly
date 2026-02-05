# skelly 🦴

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
go install github.com/skelly-dev/skelly/cmd/skelly@latest
```

## Usage

```bash
# Guided first run: picks agent profile, then runs generate + enrich
skelly setup

# Initialize .skelly/.context/ in your project
skelly init

# Initialize + generate LLM adapter files (Codex/Claude/Cursor)
skelly init --llm codex,claude,cursor

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

# Enrich changed symbols (default scope is changed)
skelly enrich --agent local

# Enrich all symbols and cap work
skelly enrich --agent local --scope all --max-symbols 200

# Enrich with PageRank-priority ordering
skelly enrich --agent local --scope all --order pagerank

# Dry-run target preview
skelly enrich --agent local --dry-run

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
skelly callers Login
skelly callees Login
skelly trace Login --depth 2
skelly path Login ValidateToken

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

Ran on `github.com/skelly-dev/skelly` itself on February 5, 2026:

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

Configure enrich agents in `.skelly/agents.yaml`:

```yaml
profiles:
  local:
    command: ["python3", ".skelly/mock_agent.py"]
    timeout: 5s
    prompt_template: |
      Summarize {{ .Symbol.Name }} in {{ .Symbol.Path }}
```

You can pass dynamic values to command args using placeholders. Example:

```yaml
profiles:
  codex:
    command: ["codex", "e", PROMPT, JSON_SCHEMA]
    timeout: 30s
```

Supported placeholders in `command`:
- `PROMPT`
- `JSON_SCHEMA` (path to `.skelly/enrich-output-schema.json`)
- `JSON_SCHEMA_JSON` (schema JSON string)
- `INPUT_JSON`
- `REQUEST_JSON`
- `INPUT_JSON_FILE` (path to temp input JSON file)
- `REQUEST_JSON_FILE` (path to temp request JSON file)
- `AGENT`
- `SCOPE`
- `SCHEMA_VERSION`

The command receives JSON on stdin:

```json
{"agent":"local","scope":"changed","prompt":"...","input":{...},"output_schema":{...},"schema_version":"enrich-output-v1"}
```

And must return JSON on stdout:

```json
{"summary":"...","purpose":"...","side_effects":"...","confidence":"low|medium|high"}
```

`output_schema` is a JSON Schema object that defines the required response shape.

If `.skelly/agents.yaml` is missing, `skelly enrich` auto-creates:
- `.skelly/agents.yaml` with a `local` profile
- `.skelly/default_agent.py` as a minimal working agent

## Current Behavior

- Incremental updates parse only changed/new files and reuse cached symbol snapshots for unchanged files.
- `--format text|jsonl` is supported for `generate` and `update` (default: `text`).
- `enrich` supports `--agent`, `--scope changed|all`, `--order source|pagerank`, `--max-symbols`, `--timeout`, and `--dry-run`.
- `setup` is an interactive shortcut for first-time usage:
  - prompts for agent profile (or accepts `--agent`)
  - prompts whether to run first-run full enrich (`scope=all`) or changed-only
  - runs `generate`
  - runs `enrich`
- `init --llm ...` generates managed LLM adapter files (`AGENTS.md`, `CLAUDE.md`, `.cursor/rules/skelly-context.mdc`) plus `CONTEXT.md`.
- `doctor` reports setup health, stale context, and suggested remediation commands.
- Navigation commands (`symbol`, `callers`, `callees`, `trace`, `path`) read from `.skelly/.context/nav-index.json`.
- `enrich` uses cache keys: `symbol_id + file_hash + prompt_version + agent_profile + model`.
- `enrich` stores cache/output records in `.skelly/.context/enrich.jsonl` and re-enriches only misses/stale entries.
- `enrich` tolerates partial agent failures and records per-symbol error status instead of failing the whole run.
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

## v0.1 Exit Criteria

- Correct: parser/graph/state unit + integration tests pass.
- Deterministic: repeated runs produce stable IDs and stable output files.
- Incremental: `skelly update` parses changed files and reuses cached snapshots.
- Operational: CI runs `go vet`, `go test`, lint, and snapshot-style integration checks.

## v0.2 Exit Criteria

- Correctness hardening fixes are covered by regression tests.
- DX setup supports `init --llm ...` and `doctor`.
- Navigation primitives (`symbol`, `callers`, `callees`, `trace`, `path`) are available with `--json`.
- Enrich caching is active with stable cache keys and partial-failure resilience.
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
