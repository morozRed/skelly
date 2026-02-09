#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  benchmark/agent_ab/scripts/analyze.sh [options]

Options:
  --input <path>        Collected JSONL file from collect.sh.
  --output-json <path>  Output summary JSON. Default: <input_dir>/analysis.json
  --output-md <path>    Output markdown report. Default: <input_dir>/analysis.md
  -h, --help            Show this help.
EOF
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"

INPUT_FILE=""
OUT_JSON=""
OUT_MD=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input)
      INPUT_FILE="$2"
      shift 2
      ;;
    --output-json)
      OUT_JSON="$2"
      shift 2
      ;;
    --output-md)
      OUT_MD="$2"
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

if [[ -z "$INPUT_FILE" ]]; then
  echo "--input is required" >&2
  usage >&2
  exit 1
fi

if [[ "$INPUT_FILE" != /* ]]; then
  INPUT_FILE="$ROOT_DIR/$INPUT_FILE"
fi
if [[ ! -f "$INPUT_FILE" ]]; then
  echo "input file not found: $INPUT_FILE" >&2
  exit 1
fi

if [[ -z "$OUT_JSON" ]]; then
  OUT_JSON="$(dirname "$INPUT_FILE")/analysis.json"
elif [[ "$OUT_JSON" != /* ]]; then
  OUT_JSON="$ROOT_DIR/$OUT_JSON"
fi
if [[ -z "$OUT_MD" ]]; then
  OUT_MD="$(dirname "$INPUT_FILE")/analysis.md"
elif [[ "$OUT_MD" != /* ]]; then
  OUT_MD="$ROOT_DIR/$OUT_MD"
fi

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

mkdir -p "$(dirname "$OUT_JSON")" "$(dirname "$OUT_MD")"

jq -s '
  def median:
    sort as $a
    | ($a | length) as $n
    | if $n == 0 then null
      elif ($n % 2) == 1 then $a[($n / 2 | floor)]
      else (($a[($n / 2 | floor) - 1] + $a[($n / 2 | floor)]) / 2)
      end;
  def as01: if . then 1 else 0 end;
  def safe_div($n; $d): if $d == 0 then null else ($n / $d) end;
  def total_tokens:
    ((.input_tokens // 0) + (.output_tokens // 0) + (.cache_read_tokens // 0) + (.cache_write_tokens // 0));
  def non_cache_tokens:
    ((.input_tokens // 0) + (.output_tokens // 0));
  def infra_failed:
    ((.opencode_exit_code // 0) == 86)
    or (((.opencode_exit_code // 0) != 0) and ((.acceptance_exit_code // 0) == 99));
  def arm_summary($arm):
    (map(select(.arm == $arm))) as $rows
    | ($rows | length) as $runs
    | ($rows | map(.success | as01) | add // 0) as $successes
    | ($rows | map(.total_cost_usd // 0) | add // 0) as $total_cost
    | {
        arm: $arm,
        runs: $runs,
        successes: $successes,
        success_rate: safe_div($successes; $runs),
        total_cost_usd: $total_cost,
        avg_cost_usd: safe_div($total_cost; $runs),
        median_cost_usd: ($rows | map(.total_cost_usd // 0) | median),
        median_duration_seconds: ($rows | map(.duration_seconds // 0) | median),
        median_non_cache_tokens: ($rows | map(non_cache_tokens) | median),
        median_cache_read_tokens: ($rows | map(.cache_read_tokens // 0) | median),
        median_cache_write_tokens: ($rows | map(.cache_write_tokens // 0) | median),
        median_total_tokens: ($rows | map(total_tokens) | median),
        median_step_count: ($rows | map(.step_count // 0) | median),
        median_tool_call_count: ($rows | map(.tool_call_count // 0) | median),
        median_acceptance_cmd_count: ($rows | map(.acceptance_cmd_count // 0) | median),
        solved_per_dollar: (if $total_cost == 0 then null else ($successes / $total_cost) end)
      };
  def paired_rows:
    group_by(.task_id + "#" + (.repeat | tostring))
    | map({
        key: (.[0].task_id + "#" + (.[0].repeat | tostring)),
        with: (map(select(.arm == "with_skelly")) | .[0]),
        without: (map(select(.arm == "without_skelly")) | .[0])
      })
    | map(select(.with != null and .without != null));
  def paired_summary:
    paired_rows as $pairs
    | ($pairs | length) as $n
    | ($pairs | map((.with.success | as01) - (.without.success | as01))) as $pass_deltas
    | ($pairs | map((.with.total_cost_usd // 0) - (.without.total_cost_usd // 0))) as $cost_deltas
    | ($pairs | map((.with.duration_seconds // 0) - (.without.duration_seconds // 0))) as $duration_deltas
    | ($pairs | map((.with | non_cache_tokens) - (.without | non_cache_tokens))) as $non_cache_token_deltas
    | ($pairs | map((.with.cache_read_tokens // 0) - (.without.cache_read_tokens // 0))) as $cache_read_token_deltas
    | ($pairs | map((.with.cache_write_tokens // 0) - (.without.cache_write_tokens // 0))) as $cache_write_token_deltas
    | ($pairs | map((.with | total_tokens) - (.without | total_tokens))) as $token_deltas
    | ($pairs | map((.with.step_count // 0) - (.without.step_count // 0))) as $step_count_deltas
    | ($pairs | map((.with.tool_call_count // 0) - (.without.tool_call_count // 0))) as $tool_call_count_deltas
    | ($pairs | map((.with.acceptance_cmd_count // 0) - (.without.acceptance_cmd_count // 0))) as $acceptance_cmd_count_deltas
    | {
        paired_runs: $n,
        mean_pass_delta: safe_div(($pass_deltas | add // 0); $n),
        median_pass_delta: ($pass_deltas | median),
        mean_cost_delta_usd: safe_div(($cost_deltas | add // 0); $n),
        median_cost_delta_usd: ($cost_deltas | median),
        mean_duration_delta_seconds: safe_div(($duration_deltas | add // 0); $n),
        median_duration_delta_seconds: ($duration_deltas | median),
        mean_non_cache_token_delta: safe_div(($non_cache_token_deltas | add // 0); $n),
        median_non_cache_token_delta: ($non_cache_token_deltas | median),
        mean_cache_read_token_delta: safe_div(($cache_read_token_deltas | add // 0); $n),
        median_cache_read_token_delta: ($cache_read_token_deltas | median),
        mean_cache_write_token_delta: safe_div(($cache_write_token_deltas | add // 0); $n),
        median_cache_write_token_delta: ($cache_write_token_deltas | median),
        mean_token_delta: safe_div(($token_deltas | add // 0); $n),
        median_token_delta: ($token_deltas | median),
        mean_step_count_delta: safe_div(($step_count_deltas | add // 0); $n),
        median_step_count_delta: ($step_count_deltas | median),
        mean_tool_call_count_delta: safe_div(($tool_call_count_deltas | add // 0); $n),
        median_tool_call_count_delta: ($tool_call_count_deltas | median),
        mean_acceptance_cmd_count_delta: safe_div(($acceptance_cmd_count_deltas | add // 0); $n),
        median_acceptance_cmd_count_delta: ($acceptance_cmd_count_deltas | median)
      };
  def decision($with; $without):
    if ($with.success_rate == null or $without.success_rate == null) then
      "insufficient data"
    elif ($with.success_rate > $without.success_rate and (($with.avg_cost_usd // 0) <= ($without.avg_cost_usd // 0))) then
      "prefer with_skelly"
    elif ($with.success_rate < $without.success_rate and (($with.avg_cost_usd // 0) >= ($without.avg_cost_usd // 0))) then
      "prefer without_skelly"
    else
      "mixed results - inspect paired deltas"
    end;

  if length == 0 then
    {error: "no collected rows"}
  else
    . as $rows
    | ($rows | map(select(infra_failed | not))) as $effective
    | ($rows | map(select(infra_failed))) as $excluded
    | ($effective | arm_summary("with_skelly")) as $with
    | ($effective | arm_summary("without_skelly")) as $without
    | {
        generated_at_utc: (now | todateiso8601),
        run_count: ($rows | length),
        effective_run_count: ($effective | length),
        excluded_infra_failures: ($excluded | length),
        task_count: ([$rows[].task_id] | unique | length),
        arm_summaries: [$with, $without],
        paired: ($effective | paired_summary),
        decision: decision($with; $without)
      }
  end
' "$INPUT_FILE" > "$OUT_JSON"

{
  echo "# Agent A/B Benchmark Report"
  echo
  jq -r '
    if .error then
      "Error: \(.error)"
    else
      "Generated: \(.generated_at_utc)\nRuns: \(.run_count)\nEffective runs: \(.effective_run_count)\nExcluded infra failures: \(.excluded_infra_failures)\nTasks: \(.task_count)\nDecision: \(.decision)"
    end
  ' "$OUT_JSON"
  echo
  echo "## Arm Summaries"
  jq -r '
    if .arm_summaries then
      .arm_summaries[]
      | "- \(.arm): runs=\(.runs), success_rate=\(.success_rate), avg_cost_usd=\(.avg_cost_usd), median_duration_s=\(.median_duration_seconds), median_non_cache_tokens=\(.median_non_cache_tokens), median_cache_read_tokens=\(.median_cache_read_tokens), median_cache_write_tokens=\(.median_cache_write_tokens), median_total_tokens=\(.median_total_tokens), median_step_count=\(.median_step_count), median_tool_call_count=\(.median_tool_call_count), solved_per_dollar=\(.solved_per_dollar)"
    else
      "- unavailable"
    end
  ' "$OUT_JSON"
  echo
  echo "## Paired Deltas (with_skelly - without_skelly)"
  jq -r '
    if .paired then
      "- paired_runs=\(.paired.paired_runs)"
      + "\n- mean_pass_delta=\(.paired.mean_pass_delta)"
      + "\n- median_pass_delta=\(.paired.median_pass_delta)"
      + "\n- mean_cost_delta_usd=\(.paired.mean_cost_delta_usd)"
      + "\n- median_cost_delta_usd=\(.paired.median_cost_delta_usd)"
      + "\n- mean_duration_delta_seconds=\(.paired.mean_duration_delta_seconds)"
      + "\n- median_duration_delta_seconds=\(.paired.median_duration_delta_seconds)"
      + "\n- mean_non_cache_token_delta=\(.paired.mean_non_cache_token_delta)"
      + "\n- median_non_cache_token_delta=\(.paired.median_non_cache_token_delta)"
      + "\n- mean_cache_read_token_delta=\(.paired.mean_cache_read_token_delta)"
      + "\n- median_cache_read_token_delta=\(.paired.median_cache_read_token_delta)"
      + "\n- mean_cache_write_token_delta=\(.paired.mean_cache_write_token_delta)"
      + "\n- median_cache_write_token_delta=\(.paired.median_cache_write_token_delta)"
      + "\n- mean_token_delta=\(.paired.mean_token_delta)"
      + "\n- median_token_delta=\(.paired.median_token_delta)"
      + "\n- mean_step_count_delta=\(.paired.mean_step_count_delta)"
      + "\n- median_step_count_delta=\(.paired.median_step_count_delta)"
      + "\n- mean_tool_call_count_delta=\(.paired.mean_tool_call_count_delta)"
      + "\n- median_tool_call_count_delta=\(.paired.median_tool_call_count_delta)"
      + "\n- mean_acceptance_cmd_count_delta=\(.paired.mean_acceptance_cmd_count_delta)"
      + "\n- median_acceptance_cmd_count_delta=\(.paired.median_acceptance_cmd_count_delta)"
    else
      "- unavailable"
    end
  ' "$OUT_JSON"
} > "$OUT_MD"

echo "analysis json: $OUT_JSON"
echo "analysis md: $OUT_MD"
