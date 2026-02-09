# Objective

Ensure incremental JSONL update behavior stays correct and deterministic, with minimal unnecessary rewrites.

# Requirements

- Keep public CLI behavior stable.
- Focus only on files needed for the failing behavior.
- Prefer understanding dependency and impact paths before editing.

# Expected Output

- Apply minimal code/test changes if needed.
- Explain root cause and why the fix is sufficient.
- Run the acceptance command and report the exact result.
