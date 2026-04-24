# Coverage Gaps Required Before POC

Produced during Plan Task 3 of `docs/superpowers/plans/2026-04-23-ent-code-reduction-phase-1-3.md`.

Inputs:
- `docs/superpowers/coverage/ent-integration-matrix.md` (ent integration scenarios)
- `docs/superpowers/coverage/service-api-go-matrix.md` (consumer test directories)

## Methodology

For each of the 8 file categories (`where`, `create`, `update`, `delete`, `mutation`, `client`, `entql`, `query`), we compared coverage signals across both matrices. A **critical** gap is one where the category has no meaningful regression signal in at least one matrix AND the refactor plan will modify that file category. A **should-fill** gap is one where one matrix has solid coverage but the other is thin — a refactor could hide a regression in the uncovered matrix. **Acceptable** gaps are categories that are fully covered in both matrices, or out-of-scope enough that the risk is already documented.

Task 1 threshold: at least one meaningful call per integration scenario. Task 2 threshold (stricter): multi-file signal across a directory.

## Cross-reference Summary

| Category | Integration coverage | Consumer coverage | Verdict |
|----------|---------------------|-------------------|---------|
| `where` | Strong — 10 of 15 scenarios | Strong — 6 of 17 dirs | Covered |
| `create` | Strong — 12 of 15 scenarios | Strong — 11 of 17 dirs | Covered |
| `update` | Good — 9 scenarios | Good — 5 dirs | Covered |
| `delete` | Good — 7 scenarios | Thin — 2 dirs (`cmd`, `ent/schema`) | Should-fill |
| `mutation` | Thin — `root` + `hooks` only | **Zero** across all 274 files | **CRITICAL** |
| `client` | Good — `root`, `gremlin`, `hooks`, `template` | Good — 5 dirs | Covered |
| `entql` | Thin — `root` + `edgeschema` only | Present in `resolvers` + `testutil` | Partially covered |
| `query` | Strong — 10 scenarios | Strong — 9 dirs | Covered |

---

## Critical (must fill before Phase 3)

### 1. `mutation`: No downstream regression net for mutation-state machinery

Phase 4 of the refactor will touch `internal/<schema>_mutation.go` (the split-mutation form used by the consumer) as well as the monolithic `mutation.go` in the integration suite. Task 2 confirmed **zero** consumer test files — across all 274 — call `OldX`, `ResetX`, `Fields()`, `AddedFields()`, or any other mutation-state method. Task 1 shows only `hooks` and `root` exercise mutation state in the integration suite, and they do so narrowly (`OldNumber`, `OldName`, `SetOp`, `IDs`). If a template change silently drops or renames a generated method on `mutation.go`, there is no consumer signal that will catch it.

- **Add to:** `entc/integration/hooks/hooks_test.go` — the existing hooks suite already sets up the `OldX` pattern; extend it with explicit assertions on `ResetX`, `Fields()`, `AddedFields()`, and `Mutation().Where()` to give the integration layer a more complete mutation-state surface.
- **Also add to:** `entc/integration/ent/schema/` — add a new sub-test in `integration_test.go` (root) or a dedicated `mutation_test.go` that exercises `.Mutation().IDs()`, `.Mutation().Op()`, and field-presence predicates (`HasField`, `FieldCleared`) across at least two entity types.
- **Rationale:** Task 1 explicitly flags "mutation state (OldX/SetOp/ResetX) is covered only in `hooks` and `root`." Task 2 explicitly confirms "mutation (0 files)" across the entire consumer. Phase 4 will modify the generated output; neither matrix provides a regression net wide enough to catch template regressions in this category.

### 2. `entql`: Integration coverage is concentrated in two scenarios; consumer coverage is present but tied to resolver/GraphQL layers

The `entql` category (`.Filter().Where(entql.…)`, typed filter methods like `.WhereName()`) is exercised by exactly two integration scenarios — `root` (the `entql_test.go` file) and `edgeschema`. The `where.tmpl` template drives the `where.go` predicates that `entql.go` delegates to; Phase 3 of the POC modifies `where.tmpl` directly. Consumer coverage exists in `resolvers` and `testutil` but operates through the GraphQL resolver layer, not via direct `entql` API calls, so a regression in `entql.go`'s typed filter methods may not surface through resolver tests.

- **Add to:** `entc/integration/edgeschema/edgeschema_test.go` — this scenario already calls entql filter methods on edge entities; add 2–3 additional sub-tests covering entql predicates on non-edge entities (e.g., `HasLikesWith`, `WhereStatus`) to give at least a second independently-structured entql scenario.
- **Add to:** `entc/integration/template/template_test.go` — the `template` scenario exercises custom code-generation; add a sub-test that invokes the generated entql `Filter()` method directly (the template scenario's glob predicate touches adjacent territory), confirming entql generation survives custom templates.
- **Rationale:** Task 1: "entql is nearly exclusive to the root suite … if template changes affect `entql.go` generation, the entql regression signal comes almost entirely from these two test files." Phase 3 modifies `where.tmpl`; entql predicates delegate into `where.go`. Both are in the direct change path, and the integration regression net is thinner than for any other category except mutation.

---

## Should-fill (fill in parallel with Phase 2)

### 1. `delete`: Consumer coverage is thin (2 directories)

The integration matrix shows good delete coverage (7 scenarios exercise `Delete()`, `DeleteOne()`, or cascade deletes). The consumer matrix shows only 2 of 17 directories exercise delete operations (`api-graphql/src/cmd` and `api-graphql/src/ent/schema`). The `testutil` directory — which accounts for 212 of 274 test files — shows only 21 files calling delete, which Task 2 notes as thin.

A template change that modifies `<entity>_delete.go`'s builder signature or removes a setter would not produce a consumer-side compile failure because the consumer's delete calls are concentrated in a small number of paths that are not exercised by the 212-file testutil suite.

- **Add to:** `api-graphql/src/testutil` — add fixture teardown helpers that explicitly call `client.Entity.Delete().Where(...)` for 3–5 high-frequency entities (`Listing`, `Contact`, `Property`, `User`, `Task`). These can double as cleanup in existing integration tests.
- **Rationale:** Integration has broad delete coverage; consumer has thin coverage. A refactor regression in `_delete.go` would pass all consumer tests and only be caught by integration, leaving business-logic-specific delete paths unguarded.

### 2. `client`: `client.Tx()` and `client.Use()` integration scenarios are gated behind `root` and `hooks`

Task 1 notes: "Tx/client hooks are thin outside root and hooks. Only `root` and `hooks` call `client.Tx()` directly or register global hooks via `client.Use()`." The consumer matrix shows 5 directories exercising client surfaces, but `txutil` only tests the `gen.Tx` interface (not entity-level transactions), and `entcontext_test` only tests context helpers. Direct `client.Tx()` usage in a consumer test that runs assertions inside the transaction body is rare.

- **Add to:** `entc/integration/edgefield/edgefield_test.go` — add a transaction sub-test that creates and queries edge-field entities inside a `client.Tx()` block, confirming rollback and commit paths work with the edge-field-specific generated client.
- **Rationale:** If `client.go` transaction helpers are modified, the regression signal is concentrated in just two integration scenarios. Adding one more transactional test in a different scenario broadens the signal without duplicating the hooks suite's hook-wiring tests.

---

## Acceptable gaps (document, don't fill)

### 1. `where`: Covered in both matrices; no gap

Ten integration scenarios and six consumer directories exercise predicate helpers non-trivially. The `where.go` category is the most broadly covered category across both matrices. Even though Phase 3 directly modifies `where.tmpl`, the regression net for generated output is the strongest of all eight categories. No fill required.

### 2. `create` and `query`: Covered in both matrices; no gap

Both categories are exercised across more than half the integration scenarios and more than half the consumer directories. The consumer's `testutil` alone (212 files, 105+ calling `.Create()`, 190+ calling `.Query()`) provides a strong regression signal. No fill required.

### 3. `update`: Covered in both matrices; acceptable thinness

Nine integration scenarios and five consumer directories exercise update operations. The consumer's thinner coverage relative to create/query is noted but acceptable — update builders share the same template machinery as create builders (`<entity>_create.go` and `<entity>_update.go` share the bulk of generated setter code). A regression in update-specific generation would be detectable via the integration matrix's 9 scenarios. No fill required before Phase 3.

### 4. `compose` and `initcheck` integration scenarios: Not a coverage gap

Task 1 correctly identifies these as non-test scenarios. `compose` is a Docker infrastructure directory; `initcheck` is a compile-only sanity check. Neither is expected to provide category-level coverage, and neither is in scope for gap-filling.

### 5. 19 untested consumer schemas: Risk documented, not actionable for this refactor

Task 2 identifies 19 schemas with no test coverage, split into history/audit tables (9), RLS-bypass views (3), user-preference tables (4), and Salesforce/sync infra (2). These represent genuine coverage debt, but filling them requires consumer-side work outside the ent fork. For the Phase 3/4 refactor, the relevant mitigation is: (a) the `where`, `create`, `query` categories used by these schemas are well-covered by other entities; (b) the `mutation` and `entql` gaps identified above are more urgent because they affect the template machinery shared by all 114 schemas including these 19. Note: the RLS-bypass views cluster (`unrestrictedescrow`, `unrestrictedlisting`, `unrestrictedproposal`) carries the highest security sensitivity of the 19 and should be tracked as a separate follow-up task.

---

## Observations

The refactor enters Phase 3 with a concentrated critical-gap profile: mutation-state machinery carries **zero** consumer regression signal and very thin integration signal, and it is squarely in Phase 4's modification path. The `entql` category is the secondary concern — present in two integration scenarios and present-but-indirect in the consumer, and in Phase 3's direct change path via `where.tmpl`. All other categories (`where`, `create`, `update`, `query`, `client`) have sufficient bilateral coverage to proceed with Phase 3 POC work. The overall risk posture is **moderate-high specifically for mutation and entql, low for everything else**: a disciplined fill of the two Critical gaps before Phase 4 begins substantially de-risks the refactor. Without those fills, a template regression in `mutation.go` or `entql.go` generation would likely survive the consumer's CI run and only be caught (if at all) by the narrow `hooks` and `root` integration scenarios.
