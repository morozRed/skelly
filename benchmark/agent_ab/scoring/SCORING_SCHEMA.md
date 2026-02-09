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

Decision heuristic:

- `prefer with_skelly` if success rate is higher and average cost is not higher.
- `prefer without_skelly` if success rate is lower and average cost is not lower.
- otherwise `mixed results - inspect paired deltas`.

You can tighten this with explicit thresholds, for example:

- rollout gate: `success_rate_delta >= +0.05` and `avg_cost_delta <= +0.00`
- efficiency gate: `success_rate_delta >= 0` and `median_total_tokens_delta <= -0.10 * baseline`

## Statistical Notes

- Use at least 3 repeats per task for pilot analysis.
- Prefer paired interpretation over aggregate-only means.
- For publication-quality results, add bootstrap confidence intervals on paired deltas.
