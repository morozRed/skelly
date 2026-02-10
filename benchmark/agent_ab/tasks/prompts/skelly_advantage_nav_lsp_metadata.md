# Objective

Ensure optional `--lsp` navigation mode returns stable provenance metadata without changing parser-first defaults.

# Requirements

- Keep behavior unchanged when `--lsp` is not provided.
- When `--lsp` is provided, include consistent source/provenance metadata.
- Avoid broad refactors unrelated to navigation output.

# Expected Output

- Apply the minimal code/test updates needed for metadata correctness.
- Explain how fallback behavior is preserved.
- Run the acceptance command and report the exact result.
