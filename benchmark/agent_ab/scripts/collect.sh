#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  benchmark/agent_ab/scripts/collect.sh [options]

Options:
  --results-dir <path>  Run output directory from run.sh.
  --output <path>       Output JSONL path. Default: <results-dir>/collected.jsonl
  -h, --help            Show this help.
EOF
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"

RESULTS_DIR=""
OUTPUT_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --results-dir)
      RESULTS_DIR="$2"
      shift 2
      ;;
    --output)
      OUTPUT_FILE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$RESULTS_DIR" ]]; then
  echo "--results-dir is required" >&2
  usage >&2
  exit 1
fi

if [[ "$RESULTS_DIR" != /* ]]; then
  RESULTS_DIR="$ROOT_DIR/$RESULTS_DIR"
fi
if [[ ! -d "$RESULTS_DIR/runs" ]]; then
  echo "runs directory not found: $RESULTS_DIR/runs" >&2
  exit 1
fi

if [[ -z "$OUTPUT_FILE" ]]; then
  OUTPUT_FILE="$RESULTS_DIR/collected.jsonl"
elif [[ "$OUTPUT_FILE" != /* ]]; then
  OUTPUT_FILE="$ROOT_DIR/$OUTPUT_FILE"
fi

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

mkdir -p "$(dirname "$OUTPUT_FILE")"
: > "$OUTPUT_FILE"

extract_metric() {
  local label="$1"
  local file="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi
  awk -v label="$label" '
    index($0, label) {
      line = $0
      gsub(/│/, "", line)
      sub("^.*" label "[[:space:]]*", "", line)
      gsub(/[[:space:]]+$/, "", line)
      print line
      exit
    }
  ' "$file"
}

normalize_number() {
  local raw="$1"
  raw="${raw//,/}"
  raw="${raw//\$/}"
  raw="$(printf '%s' "$raw" | tr -d '[:space:]')"
  if [[ -z "$raw" || "$raw" == "-" ]]; then
    printf '%s\n' ""
  else
    printf '%s\n' "$raw"
  fi
}

extract_session_id_from_events() {
  local events_file="$1"
  if [[ ! -s "$events_file" ]]; then
    return 0
  fi
  jq -Rr '
    fromjson?
    | .. | objects
    | (.sessionID? // .sessionId? // .session_id? // .id?)
    | select(type == "string")
  ' "$events_file" | grep '^ses_' | head -n1 || true
}

runs_found=0
for run_dir in "$RESULTS_DIR"/runs/*; do
  [[ -d "$run_dir" ]] || continue
  meta_file="$run_dir/run_meta.json"
  [[ -f "$meta_file" ]] || continue
  runs_found=$((runs_found + 1))

  stats_file="$run_dir/opencode_stats.txt"
  events_file="$run_dir/opencode.events.jsonl"
  session_file="$run_dir/session.export.json"

  total_cost="$(normalize_number "$(extract_metric "Total Cost" "$stats_file")")"
  input_tokens="$(normalize_number "$(extract_metric "Input" "$stats_file")")"
  output_tokens="$(normalize_number "$(extract_metric "Output" "$stats_file")")"
  cache_read_tokens="$(normalize_number "$(extract_metric "Cache Read" "$stats_file")")"
  cache_write_tokens="$(normalize_number "$(extract_metric "Cache Write" "$stats_file")")"

  if [[ -s "$session_file" ]]; then
    session_total_cost="$(jq -r '[.messages[] | .info.cost? | numbers] | add // empty' "$session_file" 2>/dev/null || true)"
    session_input_tokens="$(jq -r '[.messages[] | .info.tokens.input? | numbers] | add // empty' "$session_file" 2>/dev/null || true)"
    session_output_tokens="$(jq -r '[.messages[] | .info.tokens.output? | numbers] | add // empty' "$session_file" 2>/dev/null || true)"
    session_cache_read_tokens="$(jq -r '[.messages[] | .info.tokens.cache.read? | numbers] | add // empty' "$session_file" 2>/dev/null || true)"
    session_cache_write_tokens="$(jq -r '[.messages[] | .info.tokens.cache.write? | numbers] | add // empty' "$session_file" 2>/dev/null || true)"

    if [[ -n "$session_total_cost" ]]; then
      total_cost="$(normalize_number "$session_total_cost")"
    fi
    if [[ -n "$session_input_tokens" ]]; then
      input_tokens="$(normalize_number "$session_input_tokens")"
    fi
    if [[ -n "$session_output_tokens" ]]; then
      output_tokens="$(normalize_number "$session_output_tokens")"
    fi
    if [[ -n "$session_cache_read_tokens" ]]; then
      cache_read_tokens="$(normalize_number "$session_cache_read_tokens")"
    fi
    if [[ -n "$session_cache_write_tokens" ]]; then
      cache_write_tokens="$(normalize_number "$session_cache_write_tokens")"
    fi
  fi

  session_id="$(jq -r '.session_id // empty' "$meta_file")"
  if [[ -z "$session_id" ]]; then
    session_id="$(extract_session_id_from_events "$events_file")"
  fi

  step_count=""
  tool_call_count=""
  task_tool_call_count=""
  skelly_doctor_count=""
  skelly_update_count=""
  skelly_symbol_count=""
  acceptance_cmd_count=""
  acceptance_first_pass=""

  acceptance_cmd="$(jq -r '.acceptance_command // empty' "$meta_file")"
  if [[ -s "$events_file" ]]; then
    step_count="$(jq -Rr 'fromjson? | select(.type == "step_finish") | 1' "$events_file" | wc -l | tr -d '[:space:]')"
    tool_call_count="$(jq -Rr 'fromjson? | select(.type == "tool_use") | 1' "$events_file" | wc -l | tr -d '[:space:]')"
    task_tool_call_count="$(jq -Rr 'fromjson? | select(.type == "tool_use") | select((.part.tool? // "") == "task") | 1' "$events_file" | wc -l | tr -d '[:space:]')"
    skelly_doctor_count="$(jq -Rr 'fromjson? | select(.type == "tool_use") | (.part.state.input.command? // "") | select(type == "string" and test("^go run \\./cmd/skelly doctor($| )")) | 1' "$events_file" | wc -l | tr -d '[:space:]')"
    skelly_update_count="$(jq -Rr 'fromjson? | select(.type == "tool_use") | (.part.state.input.command? // "") | select(type == "string" and test("^go run \\./cmd/skelly update($| )")) | 1' "$events_file" | wc -l | tr -d '[:space:]')"
    skelly_symbol_count="$(jq -Rr 'fromjson? | select(.type == "tool_use") | (.part.state.input.command? // "") | select(type == "string" and test("^go run \\./cmd/skelly symbol($| )")) | 1' "$events_file" | wc -l | tr -d '[:space:]')"

    if [[ -n "$acceptance_cmd" ]]; then
      acceptance_cmd_count="$(jq -Rr --arg cmd "$acceptance_cmd" '
        fromjson?
        | select(.type == "tool_use")
        | (.part.state.input.command? // "")
        | select(type == "string" and (. == $cmd or startswith($cmd + " ")))
        | 1
      ' "$events_file" | wc -l | tr -d '[:space:]')"

      first_acceptance_exit="$(jq -Rr --arg cmd "$acceptance_cmd" '
        fromjson?
        | select(.type == "tool_use")
        | {command: (.part.state.input.command? // ""), exit: (.part.state.metadata.exit? // empty)}
        | select((.command | type) == "string" and (.command == $cmd or (.command | startswith($cmd + " "))))
        | .exit
      ' "$events_file" | head -n1)"
      if [[ "$first_acceptance_exit" == "0" ]]; then
        acceptance_first_pass="true"
      elif [[ -n "$first_acceptance_exit" ]]; then
        acceptance_first_pass="false"
      fi
    fi
  fi

  jq -n \
    --argfile meta "$meta_file" \
    --arg session_id "$session_id" \
    --arg total_cost "$total_cost" \
    --arg input_tokens "$input_tokens" \
    --arg output_tokens "$output_tokens" \
    --arg cache_read_tokens "$cache_read_tokens" \
    --arg cache_write_tokens "$cache_write_tokens" \
    --arg step_count "$step_count" \
    --arg tool_call_count "$tool_call_count" \
    --arg task_tool_call_count "$task_tool_call_count" \
    --arg skelly_doctor_count "$skelly_doctor_count" \
    --arg skelly_update_count "$skelly_update_count" \
    --arg skelly_symbol_count "$skelly_symbol_count" \
    --arg acceptance_cmd_count "$acceptance_cmd_count" \
    --arg acceptance_first_pass "$acceptance_first_pass" \
    '
    {
      run_id: $meta.run_id,
      task_id: $meta.task_id,
      arm: $meta.arm,
      repeat: $meta.repeat,
      success: $meta.success,
      duration_seconds: $meta.duration_seconds,
      opencode_exit_code: $meta.opencode_exit_code,
      acceptance_exit_code: $meta.acceptance_exit_code,
      setup_exit_code: $meta.setup_exit_code,
      arm_setup_exit_code: $meta.arm_setup_exit_code,
      prompt_file: $meta.prompt_file,
      acceptance_command: $meta.acceptance_command,
      workspace: $meta.workspace,
      started_at_utc: $meta.started_at_utc,
      ended_at_utc: $meta.ended_at_utc,
      session_id: (if $session_id == "" then null else $session_id end),
      total_cost_usd: ($total_cost | if . == "" then null else tonumber end),
      input_tokens: ($input_tokens | if . == "" then null else tonumber end),
      output_tokens: ($output_tokens | if . == "" then null else tonumber end),
      cache_read_tokens: ($cache_read_tokens | if . == "" then null else tonumber end),
      cache_write_tokens: ($cache_write_tokens | if . == "" then null else tonumber end),
      step_count: ($step_count | if . == "" then null else tonumber end),
      tool_call_count: ($tool_call_count | if . == "" then null else tonumber end),
      task_tool_call_count: ($task_tool_call_count | if . == "" then null else tonumber end),
      skelly_doctor_count: ($skelly_doctor_count | if . == "" then null else tonumber end),
      skelly_update_count: ($skelly_update_count | if . == "" then null else tonumber end),
      skelly_symbol_count: ($skelly_symbol_count | if . == "" then null else tonumber end),
      acceptance_cmd_count: ($acceptance_cmd_count | if . == "" then null else tonumber end),
      acceptance_first_pass: (
        if $acceptance_first_pass == "" then null
        elif $acceptance_first_pass == "true" then true
        elif $acceptance_first_pass == "false" then false
        else null
        end
      )
    }
    ' >> "$OUTPUT_FILE"
done

if [[ "$runs_found" -eq 0 ]]; then
  echo "no run metadata found under: $RESULTS_DIR/runs" >&2
  exit 1
fi

echo "collected records: $runs_found"
echo "output: $OUTPUT_FILE"
