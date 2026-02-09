# Agent A/B Benchmark Harness

This harness compares agent performance across two arms:

- `with_skelly`: workspace is initialized with Skelly context + managed adapter files.
- `without_skelly`: Skelly context/adapter artifacts are removed from the workspace.

The harness is designed for `opencode` and captures pass/fail, runtime, token usage, and cost.

## Directory Layout

- `benchmark/agent_ab/scripts/run.sh`: execute trial runs and persist raw artifacts.
- `benchmark/agent_ab/scripts/collect.sh`: normalize raw artifacts into `collected.jsonl`.
- `benchmark/agent_ab/scripts/analyze.sh`: compute arm and paired metrics.
- `benchmark/agent_ab/tasks/suite.template.json`: task suite template.
- `benchmark/agent_ab/tasks/prompts/TASK_PROMPT_TEMPLATE.md`: per-task prompt template.
- `benchmark/agent_ab/scoring/SCORING_SCHEMA.md`: scoring formulas and decision rule.
- `benchmark/agent_ab/scoring/run_record.schema.json`: JSON schema for collected rows.

## Prerequisites

- `opencode` available in `PATH`
- `jq`
- `git`
- network access from OpenCode runtime (for model/provider endpoints)

## Quick Start

Single command (run + collect + analyze):

```bash
benchmark/agent_ab/scripts/run.sh \
  --suite benchmark/agent_ab/tasks/suite.local.json \
  --repeats 3 \
  --model openai/gpt-5-mini \
  --agent codex \
  --collect-analyze
```

1. Copy and edit the suite template:

```bash
cp benchmark/agent_ab/tasks/suite.template.json benchmark/agent_ab/tasks/suite.local.json
```

2. Fill prompt files and acceptance commands in your suite.

3. Run benchmark:

```bash
benchmark/agent_ab/scripts/run.sh \
  --suite benchmark/agent_ab/tasks/suite.local.json \
  --repeats 3 \
  --model openai/gpt-5-mini \
  --agent codex \
  --collect-analyze
```

4. Optional manual collect normalized records:

```bash
benchmark/agent_ab/scripts/collect.sh \
  --results-dir benchmark/agent_ab/results/<run_id>
```

5. Optional manual analyze:

```bash
benchmark/agent_ab/scripts/analyze.sh \
  --input benchmark/agent_ab/results/<run_id>/collected.jsonl
```

Outputs:

- `analysis.json`: machine-readable summary
- `analysis.md`: concise human-readable report

## Notes

- Runs are executed in isolated per-trial workspaces under `results/<run_id>/workspaces/`.
- OpenCode state mode defaults to `system` so your existing `opencode` auth/providers/models are reused.
- Use `--opencode-state isolated` to redirect OpenCode cache/data under the run output directory (`.cache`, `.local/share`) for reproducibility.
- `opencode stats --project <workspace>` is used to attribute usage/cost per run.
- `benchmark/agent_ab/results/` is intended as generated output and should remain git-ignored.
