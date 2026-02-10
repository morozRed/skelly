# Objective

Ensure `definition` and `references` navigation commands stay correct for both symbol and `file:line` targets.

# Requirements

- Keep parser-first behavior deterministic.
- Avoid regressions in existing navigation commands.
- Use minimal changes to satisfy acceptance.

# Expected Output

- Apply only changes required for definition/reference command correctness.
- Explain why target resolution is stable for `symbol` and `file:line` inputs.
- Run the acceptance command and report the exact result.
