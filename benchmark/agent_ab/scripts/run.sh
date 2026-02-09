#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  benchmark/agent_ab/scripts/run.sh [options]

Options:
  --suite <path>         Path to suite JSON file.
  --out <path>           Output directory for this benchmark run.
  --repeats <n>          Number of repeats per (task, arm). Default: 1
  --model <provider/id>  OpenCode model (optional).
  --agent <name>         OpenCode agent name (optional).
  --opencode-bin <path>  OpenCode binary path. Default: opencode
  --opencode-state <m>   OpenCode state mode: system|isolated. Default: system
  --arms <csv>           Comma-separated arms. Default: with_skelly,without_skelly
  --dry-run              Prepare workspaces and metadata, skip OpenCode + acceptance.
  -h, --help             Show this help.
EOF
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"

SUITE_FILE="$ROOT_DIR/benchmark/agent_ab/tasks/suite.template.json"
OUT_DIR="$ROOT_DIR/benchmark/agent_ab/results/$(date -u +%Y%m%dT%H%M%SZ)"
REPEATS=1
MODEL="${MODEL:-}"
AGENT="${AGENT:-}"
OPENCODE_BIN="${OPENCODE_BIN:-opencode}"
OPENCODE_STATE_MODE="system"
ARMS_CSV="with_skelly,without_skelly"
DRY_RUN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --suite)
      SUITE_FILE="$2"
      shift 2
      ;;
    --out)
      OUT_DIR="$2"
      shift 2
      ;;
    --repeats)
      REPEATS="$2"
      shift 2
      ;;
    --model)
      MODEL="$2"
      shift 2
      ;;
    --agent)
      AGENT="$2"
      shift 2
      ;;
    --opencode-bin)
      OPENCODE_BIN="$2"
      shift 2
      ;;
    --opencode-state)
      OPENCODE_STATE_MODE="$2"
      shift 2
      ;;
    --arms)
      ARMS_CSV="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift 1
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

if [[ "$REPEATS" =~ [^0-9] || "$REPEATS" -lt 1 ]]; then
  echo "--repeats must be a positive integer" >&2
  exit 1
fi
if [[ "$OPENCODE_STATE_MODE" != "system" && "$OPENCODE_STATE_MODE" != "isolated" ]]; then
  echo "--opencode-state must be one of: system, isolated" >&2
  exit 1
fi

if [[ "$SUITE_FILE" != /* ]]; then
  SUITE_FILE="$ROOT_DIR/$SUITE_FILE"
fi
if [[ "$OUT_DIR" != /* ]]; then
  OUT_DIR="$ROOT_DIR/$OUT_DIR"
fi

if [[ ! -f "$SUITE_FILE" ]]; then
  echo "suite file not found: $SUITE_FILE" >&2
  exit 1
fi

command -v git >/dev/null 2>&1 || { echo "git is required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }
if [[ "$DRY_RUN" -ne 1 ]]; then
  command -v "$OPENCODE_BIN" >/dev/null 2>&1 || {
    echo "OpenCode binary not found: $OPENCODE_BIN" >&2
    exit 1
  }
fi

jq -e '.tasks and (.tasks | type == "array") and (.tasks | length > 0)' "$SUITE_FILE" >/dev/null || {
  echo "invalid suite file: expected non-empty .tasks array" >&2
  exit 1
}

mkdir -p "$OUT_DIR/runs" "$OUT_DIR/workspaces" "$OUT_DIR/.gocache"
if [[ "$OPENCODE_STATE_MODE" == "isolated" ]]; then
  mkdir -p "$OUT_DIR/.cache" "$OUT_DIR/.local/share"
fi
cp "$SUITE_FILE" "$OUT_DIR/suite.json"

jq -n \
  --arg suite_file "$SUITE_FILE" \
  --arg out_dir "$OUT_DIR" \
  --arg model "$MODEL" \
  --arg agent "$AGENT" \
  --arg opencode_state "$OPENCODE_STATE_MODE" \
  --arg arms "$ARMS_CSV" \
  --argjson repeats "$REPEATS" \
  --argjson dry_run "$DRY_RUN" \
  '{
    created_at_utc: (now | todateiso8601),
    suite_file: $suite_file,
    out_dir: $out_dir,
    model: (if $model == "" then null else $model end),
    agent: (if $agent == "" then null else $agent end),
    opencode_state: $opencode_state,
    arms: ($arms | split(",")),
    repeats: $repeats,
    dry_run: ($dry_run == 1)
  }' > "$OUT_DIR/config.json"

arm_preamble() {
  case "$1" in
    with_skelly)
      cat <<'EOF'
Benchmark arm: with_skelly

Use Skelly with a minimal loop:
1. Run go run ./cmd/skelly doctor exactly once at start.
2. Run go run ./cmd/skelly update --format jsonl only if doctor reports stale context.
3. Use Skelly CLI commands first: symbol, callers, callees, trace, path, status.
4. Run acceptance early after first concrete conclusion.
5. If acceptance passes and no code changes are required, stop (no extra verification loops).
6. Avoid direct reads of .skelly/.context/* unless CLI output is insufficient.
EOF
      ;;
    without_skelly)
      cat <<'EOF'
Benchmark arm: without_skelly

Do not use Skelly commands or Skelly context artifacts.
Work directly from source files and tests.
EOF
      ;;
    *)
      echo "Benchmark arm: $1"
      ;;
  esac
}

prepare_arm_workspace() {
  local arm="$1"
  local workspace="$2"
  local log_file="$3"
  local rc=0

  case "$arm" in
    with_skelly)
      (
        cd "$workspace"
        rm -rf .skelly AGENTS.md CONTEXT.md CLAUDE.md .cursor/rules/skelly-context.mdc
        GOCACHE="$OUT_DIR/.gocache" go run ./cmd/skelly init --llm codex,claude,cursor --format jsonl
        GOCACHE="$OUT_DIR/.gocache" go run ./cmd/skelly doctor
      ) >>"$log_file" 2>&1 || rc=$?

      if [[ "$rc" -eq 0 && -f "$ROOT_DIR/.skelly/skills/skelly.md" ]]; then
        mkdir -p "$workspace/.skelly/skills"
        cp "$ROOT_DIR/.skelly/skills/skelly.md" "$workspace/.skelly/skills/skelly.md" >>"$log_file" 2>&1 || rc=$?
      fi

      if [[ "$rc" -eq 0 ]]; then
        for adapter_file in AGENTS.md CONTEXT.md CLAUDE.md; do
          if [[ -f "$ROOT_DIR/$adapter_file" ]]; then
            cp "$ROOT_DIR/$adapter_file" "$workspace/$adapter_file" >>"$log_file" 2>&1 || rc=$?
          fi
        done
        if [[ -f "$ROOT_DIR/.cursor/rules/skelly-context.mdc" ]]; then
          mkdir -p "$workspace/.cursor/rules"
          cp "$ROOT_DIR/.cursor/rules/skelly-context.mdc" "$workspace/.cursor/rules/skelly-context.mdc" >>"$log_file" 2>&1 || rc=$?
        fi
      fi
      ;;
    without_skelly)
      (
        cd "$workspace"
        rm -rf .skelly AGENTS.md CLAUDE.md CONTEXT.md .cursor/rules/skelly-context.mdc
      ) >>"$log_file" 2>&1 || rc=$?
      ;;
    *)
      echo "no arm-specific setup for '$arm'" >>"$log_file"
      ;;
  esac
  return "$rc"
}

run_setup_commands() {
  local commands_json="$1"
  local workspace="$2"
  local log_file="$3"
  local cmd
  local rc=0

  while IFS= read -r cmd; do
    [[ -z "$cmd" ]] && continue
    (
      cd "$workspace"
      GOCACHE="$OUT_DIR/.gocache" bash -lc "$cmd"
    ) >>"$log_file" 2>&1 || rc=$?
    if [[ "$rc" -ne 0 ]]; then
      echo "setup command failed: $cmd" >>"$log_file"
      return "$rc"
    fi
  done < <(printf '%s' "$commands_json" | jq -r '.[]')

  return 0
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

extract_session_id_from_list() {
  local workspace="$1"
  local run_id="$2"
  local list_json
  local sid

  list_json="$(
    if [[ "$OPENCODE_STATE_MODE" == "isolated" ]]; then
      (
        cd "$workspace"
        XDG_CACHE_HOME="$OUT_DIR/.cache" \
        XDG_DATA_HOME="$OUT_DIR/.local/share" \
        "$OPENCODE_BIN" session list --format json -n 50
      ) 2>/dev/null || true
    else
      (
        cd "$workspace"
        "$OPENCODE_BIN" session list --format json -n 50
      ) 2>/dev/null || true
    fi
  )"

  if [[ -z "$list_json" ]]; then
    return 0
  fi

  sid="$(printf '%s' "$list_json" | jq -r --arg run_id "$run_id" '
      [
        .. | objects
        | select((.title? // "") == $run_id)
        | (.id? // .sessionID? // .sessionId? // .session_id?)
      ]
      | map(select(type == "string" and test("^ses_")))
      | .[0] // empty
    ' 2>/dev/null || true)"
  if [[ -n "$sid" ]]; then
    printf '%s\n' "$sid"
  fi
}

build_prompt() {
  local arm="$1"
  local task_id="$2"
  local prompt_file="$3"
  local acceptance_cmd="$4"
  local out_file="$5"

  {
    arm_preamble "$arm"
    echo
    echo "Task ID: $task_id"
    echo "Acceptance command (must pass): $acceptance_cmd"
    echo
    cat "$prompt_file"
    echo
    echo "When done, output:"
    echo "1) short change summary"
    echo "2) exact acceptance command and result"
  } > "$out_file"
}

echo "results dir: $OUT_DIR"

declare -a ARMS
IFS=',' read -r -a ARMS <<<"$ARMS_CSV"

while IFS= read -r task_json; do
  task_id="$(printf '%s' "$task_json" | jq -r '.id')"
  prompt_rel="$(printf '%s' "$task_json" | jq -r '.prompt_file')"
  acceptance_cmd="$(printf '%s' "$task_json" | jq -r '.acceptance_command')"
  setup_commands="$(printf '%s' "$task_json" | jq -c '.setup_commands // []')"
  timeout_seconds="$(printf '%s' "$task_json" | jq -r '.timeout_seconds // 1800')"

  if [[ "$prompt_rel" == /* ]]; then
    prompt_file="$prompt_rel"
  else
    prompt_file="$ROOT_DIR/$prompt_rel"
  fi

  if [[ ! -f "$prompt_file" ]]; then
    echo "prompt file missing for task '$task_id': $prompt_file" >&2
    exit 1
  fi

  for arm in "${ARMS[@]}"; do
    for repeat_idx in $(seq 1 "$REPEATS"); do
      run_stamp="$(date -u +%Y%m%dT%H%M%SZ)"
      run_id="${run_stamp}__${task_id}__${arm}__r${repeat_idx}"
      run_dir="$OUT_DIR/runs/$run_id"
      workspace="$OUT_DIR/workspaces/$run_id"

      mkdir -p "$run_dir" "$workspace"
      echo "running: task=$task_id arm=$arm repeat=$repeat_idx run_id=$run_id"

      (
        cd "$ROOT_DIR"
        git archive --format=tar HEAD | tar -xf - -C "$workspace"
      ) >"$run_dir/workspace_setup.log" 2>&1

      setup_exit_code=0
      arm_setup_exit_code=0
      opencode_exit_code=99
      acceptance_exit_code=99
      session_id=""

      run_setup_commands "$setup_commands" "$workspace" "$run_dir/task_setup.log" || setup_exit_code=$?
      prepare_arm_workspace "$arm" "$workspace" "$run_dir/arm_setup.log" || arm_setup_exit_code=$?

      build_prompt "$arm" "$task_id" "$prompt_file" "$acceptance_cmd" "$run_dir/prompt.txt"

      started_at_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      start_epoch="$(date +%s)"

      if [[ "$DRY_RUN" -eq 1 || "$setup_exit_code" -ne 0 || "$arm_setup_exit_code" -ne 0 ]]; then
        echo "skipped OpenCode run (dry-run or setup failed)" >"$run_dir/opencode.stderr.log"
        : >"$run_dir/opencode.events.jsonl"
      else
        prompt_payload="$(cat "$run_dir/prompt.txt")"
        declare -a opencode_cmd
        opencode_cmd=("$OPENCODE_BIN" run --format json --title "$run_id")
        if [[ -n "$MODEL" ]]; then
          opencode_cmd+=(--model "$MODEL")
        fi
        if [[ -n "$AGENT" ]]; then
          opencode_cmd+=(--agent "$AGENT")
        fi
        opencode_cmd+=("$prompt_payload")

        set +e
        if [[ "$OPENCODE_STATE_MODE" == "isolated" ]]; then
          (
            cd "$workspace"
            XDG_CACHE_HOME="$OUT_DIR/.cache" \
            XDG_DATA_HOME="$OUT_DIR/.local/share" \
            "${opencode_cmd[@]}"
          ) >"$run_dir/opencode.events.jsonl" 2>"$run_dir/opencode.stderr.log"
        else
          (
            cd "$workspace"
            "${opencode_cmd[@]}"
          ) >"$run_dir/opencode.events.jsonl" 2>"$run_dir/opencode.stderr.log"
        fi
        opencode_exit_code=$?
        set -e

        if grep -Eq "ProviderModelNotFoundError|ModelNotFoundError|Unable to connect|InstallFailedError|Failed to fetch models\\.dev" "$run_dir/opencode.stderr.log"; then
          # OpenCode can print fatal errors to stderr while still returning exit code 0 in some cases.
          opencode_exit_code=86
        fi

        session_id="$(extract_session_id_from_events "$run_dir/opencode.events.jsonl")"
        if [[ -z "$session_id" ]]; then
          session_id="$(extract_session_id_from_list "$workspace" "$run_id")"
        fi

        if [[ -n "$session_id" ]]; then
          if [[ "$OPENCODE_STATE_MODE" == "isolated" ]]; then
            (
              cd "$workspace"
              XDG_CACHE_HOME="$OUT_DIR/.cache" \
              XDG_DATA_HOME="$OUT_DIR/.local/share" \
              "$OPENCODE_BIN" export "$session_id"
            ) >"$run_dir/session.export.json" 2>"$run_dir/session.export.stderr.log" || true
          else
            (
              cd "$workspace"
              "$OPENCODE_BIN" export "$session_id"
            ) >"$run_dir/session.export.json" 2>"$run_dir/session.export.stderr.log" || true
          fi
        fi
      fi

      if [[ "$DRY_RUN" -eq 1 || "$setup_exit_code" -ne 0 || "$arm_setup_exit_code" -ne 0 || "$opencode_exit_code" -ne 0 ]]; then
        echo "skipped acceptance (dry-run/setup failure/opencode failure)" >"$run_dir/acceptance.log"
      else
        set +e
        (
          cd "$workspace"
          GOCACHE="$OUT_DIR/.gocache" bash -lc "$acceptance_cmd"
        ) >"$run_dir/acceptance.log" 2>&1
        acceptance_exit_code=$?
        set -e
      fi

      if [[ "$OPENCODE_STATE_MODE" == "isolated" ]]; then
        (
          cd "$workspace"
          XDG_CACHE_HOME="$OUT_DIR/.cache" \
          XDG_DATA_HOME="$OUT_DIR/.local/share" \
          "$OPENCODE_BIN" stats --project "$workspace"
        ) >"$run_dir/opencode_stats.txt" 2>"$run_dir/opencode_stats.stderr.log" || true
      else
        (
          cd "$workspace"
          "$OPENCODE_BIN" stats --project "$workspace"
        ) >"$run_dir/opencode_stats.txt" 2>"$run_dir/opencode_stats.stderr.log" || true
      fi

      end_epoch="$(date +%s)"
      ended_at_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      duration_seconds=$((end_epoch - start_epoch))

      if [[ "$acceptance_exit_code" -eq 0 ]]; then
        success=true
      else
        success=false
      fi

      jq -n \
        --arg run_id "$run_id" \
        --arg task_id "$task_id" \
        --arg arm "$arm" \
        --arg workspace "$workspace" \
        --arg prompt_file "$prompt_rel" \
        --arg acceptance_command "$acceptance_cmd" \
        --arg started_at_utc "$started_at_utc" \
        --arg ended_at_utc "$ended_at_utc" \
        --arg session_id "$session_id" \
        --argjson repeat "$repeat_idx" \
        --argjson timeout_seconds "$timeout_seconds" \
        --argjson setup_exit_code "$setup_exit_code" \
        --argjson arm_setup_exit_code "$arm_setup_exit_code" \
        --argjson opencode_exit_code "$opencode_exit_code" \
        --argjson acceptance_exit_code "$acceptance_exit_code" \
        --argjson duration_seconds "$duration_seconds" \
        --argjson success "$success" \
        '{
          run_id: $run_id,
          task_id: $task_id,
          arm: $arm,
          repeat: $repeat,
          timeout_seconds: $timeout_seconds,
          prompt_file: $prompt_file,
          acceptance_command: $acceptance_command,
          workspace: $workspace,
          started_at_utc: $started_at_utc,
          ended_at_utc: $ended_at_utc,
          duration_seconds: $duration_seconds,
          setup_exit_code: $setup_exit_code,
          arm_setup_exit_code: $arm_setup_exit_code,
          opencode_exit_code: $opencode_exit_code,
          acceptance_exit_code: $acceptance_exit_code,
          success: $success,
          session_id: (if $session_id == "" then null else $session_id end)
        }' >"$run_dir/run_meta.json"
    done
  done
done < <(jq -c '.tasks[]' "$SUITE_FILE")

echo "benchmark runs complete: $OUT_DIR"
echo "next:"
echo "  benchmark/agent_ab/scripts/collect.sh --results-dir \"$OUT_DIR\""
echo "  benchmark/agent_ab/scripts/analyze.sh --input \"$OUT_DIR/collected.jsonl\""
