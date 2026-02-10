# Objective

Ensure fuzzy symbol lookup remains reliable and deterministic for typo and partial-name queries.

# Requirements

- Keep exact `symbol` lookup behavior as the primary path.
- Keep fuzzy fallback deterministic for equivalent scores.
- Prefer the smallest change set that restores acceptance.

# Expected Output

- Apply only the code/test changes required for fuzzy retrieval behavior.
- Explain why ranking and fallback are stable.
- Run the acceptance command and report the exact result.
