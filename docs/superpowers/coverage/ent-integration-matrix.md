# Ent Integration Test Coverage Matrix

Produced during Plan Task 1 of `docs/superpowers/plans/2026-04-23-ent-code-reduction-phase-1-3.md`.

Source: test files under `entc/integration/` on branch `i-h8-ent` at SHA `f88db35511fb82bfbd20486fe699a7c4d6df806b`.

## File-Category Key

All paths below are relative to each scenario's generated `ent/` root (e.g. `entc/integration/edgeschema/ent/`). Generated per-entity builder files are flat at that root (`<entity>_create.go`, `<entity>_update.go`, etc.), while predicate helpers live in an `<entity>/` subdirectory alongside `where.go`.

| Category   | Generated files exercised |
|------------|--------------------------|
| `where`    | `<entity>/where.go` — predicate helpers like `NameEQ`, `HasEdge`, `HasOwnerWith`, etc. |
| `create`   | `<entity>_create.go` — `Create()`, `CreateBulk()`, builder `.SetX()` / `.AddX()` chains |
| `update`   | `<entity>_update.go` — `Update()`, `UpdateOne()`, `.SetX()` / `.AddX()` / `.ClearX()` |
| `delete`   | `<entity>_delete.go` — `Delete()`, `DeleteOne()` |
| `mutation` | `mutation.go` — mutation-state reads: `.OldX()`, `.IDs()`, `.SetOp()`, `.ResetX()` (downstream consumer repos may split this into `internal/*_mutation.go`) |
| `client`   | `client.go` — `client.Use()`, `client.Tx()`, `client.Debug()`, transaction hooks |
| `entql`    | `entql.go` — `.Filter().Where(entql.…)`, typed filter methods like `.WhereName()` |
| `query`    | `<entity>_query.go` — query entrypoints, `With*` eager loading, `.Order()`, `.GroupBy()` |

**Threshold for ✓:** the scenario invokes the category's API surface non-trivially — at least one test calls a category-specific method meaningfully (beyond a single throwaway `Query().CountX(ctx)` or a compile-only import). Trivial calls that exist only to build up data for a different assertion are left blank.

## Coverage Matrix

| Scenario        | where | create | update | delete | mutation | client | entql | query | Notes |
|-----------------|:-----:|:------:|:------:|:------:|:--------:|:------:|:-----:|:-----:|-------|
| **root** (entql_test, index_test, integration_test, relation_test, type_test) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | Broad multi-dialect suite covering nearly all categories; `EntQL` sub-file is the only place top-level entql predicates are exercised |
| cascadelete     |       | ✓      |        | ✓      |          |        |       |       | Cascading deletes through FK edges; focus is on DB-level cascade, not predicate filtering. Uses `Query().CountX`/`OnlyIDX` only to assert delete side-effects — trivial under the ✓ threshold |
| compose         |       |        |        |        |          |        |       |       | Infrastructure only (gremlin-server Docker compose dir); no test file present |
| config          |       | ✓      |        |        |          |        |       |       | Schema-level SQL annotations (table names, column sizes, incremental IDs); inspects generated AST |
| customid        | ✓     | ✓      |        |        |          |        |       | ✓     | Non-default ID types (int, UUID, bytes, string, custom); upsert on ID conflict; `WithFriends`/`WithAccount` eager loads |
| edgefield       | ✓     | ✓      | ✓      |        |          |        |       | ✓     | FK-backed edge fields exposed on entities; `SetOwnerID`, `ClearOwnerID`, edge-field predicates |
| edgeschema      | ✓     | ✓      | ✓      | ✓      |          |        | ✓     | ✓     | Through-table edge schemas with composite and single IDs; exercises entql filters on edge entities |
| gremlin         | ✓     | ✓      | ✓      | ✓      |          | ✓      |       | ✓     | Gremlin-dialect mirror of root tests (sanity, relations, predicates, types); runs a subset without Tx/index tests |
| hooks           | ✓     | ✓      | ✓      | ✓      | ✓        | ✓      |       | ✓     | Schema and runtime hooks, `client.Use()`, `Tx()` inside hooks, `OldX()` mutation state, soft-delete intercept pattern |
| idtype          |       | ✓      |        |        |          |        |       | ✓     | Single test verifying int64 ID type; uses `WithSpouse` eager load and `Select` projection |
| initcheck       |       |        |        |        |          |        |       |       | Blank test that imports the generated `ent` package to verify the `init()` registration does not panic |
| json            | ✓     | ✓      | ✓      | ✓      |          |        |       |       | JSON field types (slices, URL, net.Addr, raw); sqljson predicate helpers exercised directly in `Predicates` sub-test |
| migrate         | ✓     | ✓      | ✓      | ✓      |          |        |       |       | Schema versioning V1→V2, Atlas versioned migrations, column rename/type-change, check constraints, partial indexes |
| multischema     | ✓     | ✓      | ✓      |        |          |        |       | ✓     | Cross-database-schema queries; `AlternateSchema` client option; `WithFriends`/`WithGroups` eager loads |
| privacy         |       | ✓      | ✓      |        |          |        |       |       | Privacy policies (allow/deny rules, viewer context); tests that policy enforcement blocks or permits create/update operations |
| template        | ✓     | ✓      |        |        |          | ✓      |       | ✓     | Custom code-generation templates (Node API, `HTTPClient` field injection, glob predicate, `HiddenData` method) |

## Observations

**Good breadth across create/update/delete/query/where.** The `root` scenario (the combined top-level `*_test.go` files) is the single largest and most complete suite — it exercises all eight categories and is the only test that covers transactions (`client.Tx`), eager-loading via `With*`, named-eager-loading, ordering, aggregate queries, constraint checks, and lock semantics in depth. Most per-scenario tests rely on this baseline and layer in their specific concern.

**entql is nearly exclusive to the root suite.** `entql_test.go` (root) and `edgeschema_test.go` are the only files that call `.Filter().Where(entql.…)` or typed filter methods (`.WhereName()`, `.WhereHasLikesWith()`). All other per-scenario suites produce `entql.go` files as part of code generation but never invoke them. If template changes affect `entql.go` generation, the entql regression signal comes almost entirely from these two test files.

**mutation state (OldX/SetOp/ResetX) is covered only in `hooks` and `root`.** The `hooks` suite is the primary exerciser of `OldNumber(ctx)`, `OldName(ctx)`, `SetOp`, and `IDs(ctx)`. The `root` suite also exercises `.Mutation().Where()` and `.Mutation().IDs()`. No other per-scenario suite reads from the mutation-state API, meaning `*_mutation.go` methods beyond field getters/setters are only regression-tested through those two suites.

**Tx/client hooks are thin outside root and hooks.** Only `root` and `hooks` call `client.Tx()` directly or register global hooks via `client.Use()`. The `gremlin` suite registers no hooks and skips the Tx test. `template` exercises `client.Use()` indirectly through a schema hook but does not test transactions. All other per-scenario tests open a plain client with no hook wiring.

**`compose` and `initcheck` have no meaningful generated-file coverage.** `compose` is a Docker infrastructure directory with no test file. `initcheck` is a one-liner blank test that exists solely to confirm the package-level `init()` does not crash. Neither scenario exercises any generated builder or predicate code.

**Coverage gaps to address before template changes:** If generated file categories are to be modified, the weakest regression signals are: (1) `entql.go` — only `root/entql_test.go` and `edgeschema` exercise it; (2) `*_mutation.go` OldX/Reset paths — only `hooks` and `root`; (3) transaction helpers inside `client.go` — only `root` and `hooks`. These gaps are the primary concern identified in Task 2 (downstream consumer tests) and Task 3 (gap analysis).
