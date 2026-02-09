# Benchmark Success Criteria

## Objective

Define explicit promotion criteria for adopting Skelly workflow defaults based on benchmark outcomes.

## Primary Gates

For a benchmark suite to be considered a **Skelly win**, all of the following must hold:

1. Pass-rate gate:
   - `with_skelly` success rate is greater than or equal to `without_skelly`.
2. Runtime gate:
   - `with_skelly` median duration is less than or equal to `without_skelly`.
3. Token gate:
   - `with_skelly` median **non-cache tokens** (`input + output`) is less than or equal to `without_skelly`.

## Secondary Diagnostics (non-blocking)

- Cache behavior:
  - Track `cache_read` and `cache_write` separately.
  - Large cache-read increases require explanation (extra loops, bigger prompt context, etc).
- Orchestration overhead:
  - Compare `step_count`, `tool_call_count`, and `acceptance_cmd_count`.
  - Unexpected increases should trigger prompt/policy review.

## Statistical Minimum

- At least 5 repeats per task-arm pair before making decisions.
- Prefer 10 repeats for suites with high variance.

## Failure Classification

Runs with infrastructure failures (model/provider unavailable, network outages) must be excluded from decision summaries and reported separately.

## Reporting Format

Every benchmark report should include:

- Arm medians for: duration, non-cache tokens, cache read/write, total tokens.
- Paired deltas for the same metrics.
- Orchestration deltas: steps, tool calls, acceptance command count.
- Final decision: `prefer with_skelly`, `prefer without_skelly`, or `mixed`.
