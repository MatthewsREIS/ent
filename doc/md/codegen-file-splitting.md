---
id: codegen-file-splitting
title: Codegen File Splitting
---

`entc` supports optional, deterministic file splitting for generated Go assets.
The feature is configured with `gen.SplitConfig` (or `entc.Split(...)`) and is disabled by default.

## Defaults

- When `Split` is unset, code generation output is identical to existing behavior.
- Default mode is `type` splitting (`gen.SplitByType()`).
- If no include patterns are configured, only core built-in templates are eligible.
- Extension outputs are excluded by default (for example, `gql_*.go`).

## Selection Rules

Split selection evaluates both template names and generated output file names:

- Include patterns: opt files in.
- Exclude patterns: force files out.
- Exclude patterns take precedence over include patterns.

Examples:

- Template name match: `client`
- Output filename match: `mutation.go`
- Extension opt-in: `gql_*.go`

## Output Naming and Determinism

In `type` mode, a selected file is rewritten into deterministic parts:

- `<name>_base.go`
- `<name>_partNN_<type>.go`

`NN` is zero-padded and stable across runs for the same input, and type suffixes are deterministic.

## Stale File Cleanup

On regeneration, `entc` removes orphaned split outputs:

- Old unsplit file when a file is now split.
- Old split parts when a file is now unsplit.
- Old split files for deleted schema types.

This keeps generated directories clean across feature toggles and schema changes.
