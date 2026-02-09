# Objective

Ensure parser directory traversal respects ignore rules and only includes intended files in incremental workflows.

# Requirements

- Keep parser contracts and output format compatibility intact.
- Limit edits to parser/ignore logic and related tests when required.
- Do not change unrelated language parser behavior.

# Expected Output

- Make minimal changes to restore expected scope behavior.
- Explain what was incorrectly included/excluded and why.
- Run the acceptance command and report the exact result.
