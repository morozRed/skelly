#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  benchmark/agent_ab/scripts/run_all.sh [run.sh options]

Description:
  Runs benchmark/agent_ab/scripts/run.sh and then automatically runs:
  1) benchmark/agent_ab/scripts/collect.sh
  2) benchmark/agent_ab/scripts/analyze.sh

  It wires the same results directory through all steps.

Examples:
  benchmark/agent_ab/scripts/run_all.sh \
    --suite benchmark/agent_ab/tasks/suite.local.json \
    --repeats 3 \
    --model openai/gpt-5-mini \
    --agent codex

  benchmark/agent_ab/scripts/run_all.sh \
    --suite benchmark/agent_ab/tasks/suite.local.json \
    --repeats 1 \
    --dry-run
EOF
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

RUN_SCRIPT="$SCRIPT_DIR/run.sh"
COLLECT_SCRIPT="$SCRIPT_DIR/collect.sh"
ANALYZE_SCRIPT="$SCRIPT_DIR/analyze.sh"

for required in "$RUN_SCRIPT" "$COLLECT_SCRIPT" "$ANALYZE_SCRIPT"; do
  if [[ ! -x "$required" ]]; then
    echo "required executable script not found: $required" >&2
    exit 1
  fi
done

OUT_DIR="$ROOT_DIR/benchmark/agent_ab/results/$(date -u +%Y%m%dT%H%M%SZ)"

echo "run_all: output directory: $OUT_DIR"
"$RUN_SCRIPT" --out "$OUT_DIR" "$@"
"$COLLECT_SCRIPT" --results-dir "$OUT_DIR"
"$ANALYZE_SCRIPT" --input "$OUT_DIR/collected.jsonl"

echo "run_all: complete"
echo "run_all: report: $OUT_DIR/analysis.md"
echo "run_all: json:   $OUT_DIR/analysis.json"
