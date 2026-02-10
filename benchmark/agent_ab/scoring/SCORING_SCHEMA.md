# Scoring Schema

This schema defines how to score `with_skelly` vs `without_skelly` benchmark runs.

## Run-Level Record

Each run is one JSON object in `collected.jsonl` and follows `run_record.schema.json`.

Required fields used in scoring:

- `task_id`
- `arm`
- `repeat`
- `success`
- `duration_seconds`
- `total_cost_usd`
- `input_tokens`
- `output_tokens`
- `cache_read_tokens`
- `cache_write_tokens`

## Derived Metrics

Per run:

- `total_tokens = input_tokens + output_tokens + cache_read_tokens + cache_write_tokens`
- `non_cache_tokens = input_tokens + output_tokens`

Per arm:

- `runs = count(runs)`
- `successes = count(success == true)`
- `success_rate = successes / runs`
- `total_cost_usd = sum(total_cost_usd)`
- `avg_cost_usd = total_cost_usd / runs`
- `median_cost_usd`
- `median_duration_seconds`
- `median_total_tokens`
- `solved_per_dollar = successes / total_cost_usd` (null when total cost is zero)

Paired delta (for each `(task_id, repeat)` present in both arms):

- `pass_delta = success(with_skelly) - success(without_skelly)` with success mapped to `{0,1}`
- `cost_delta_usd = cost(with_skelly) - cost(without_skelly)`
- `duration_delta_seconds = duration(with_skelly) - duration(without_skelly)`
- `token_delta = total_tokens(with_skelly) - total_tokens(without_skelly)`

Aggregate paired metrics:

- mean/median of each delta above

## Decision Rule

Primary gate (aligned with `docs/BENCHMARK_SUCCESS_CRITERIA.md`):

- `prefer with_skelly` when all of the following hold:
  - success-rate gate: `success_rate(with_skelly) >= success_rate(without_skelly)`
  - runtime gate: `median_duration_seconds(with_skelly) <= median_duration_seconds(without_skelly)`
  - token gate: `median_non_cache_tokens(with_skelly) <= median_non_cache_tokens(without_skelly)`
- `prefer without_skelly` when all three gates flip in the opposite direction.
- otherwise `mixed results - inspect paired deltas`.

Secondary diagnostics remain non-blocking and should explain gate regressions:

- cache-read/write shifts
- orchestration deltas (`step_count`, `tool_call_count`, `acceptance_cmd_count`)

## Statistical Notes

- Use at least 5 repeats per task-arm pair before promotion decisions.
- Prefer 10 repeats for higher-variance suites.
- Prefer paired interpretation over aggregate-only means.
- For publication-quality results, add bootstrap confidence intervals on paired deltas.
