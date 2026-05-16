# ent codegen reduction — epic design

**Status:** In progress (6 of 7 PRs complete as of 2026-05-16)
**Date:** 2026-05-15 (created); progress tracker updated 2026-05-16
**Worktree:** `.claude/worktrees/wiggly-singing-pancake` on branch `worktree-wiggly-singing-pancake`
**Stack tool:** git-spice (deferred — all commits stay local until end-of-epic per [[feedback-no-prs-until-end-of-epic]])

## 0. Progress

| PR | Title | Status | Plan | Commits range (top → bottom) |
|---|---|---|---|---|
| 0 | baseline (bench harness + entc/gen fixes) | ✅ complete | `docs/superpowers/plans/2026-05-15-codegen-epic-pr0-baseline.md` | `f3471df35` … `21bb66022` |
| 1 | loader UX / bootstrap mode (DX win) | ✅ complete | `docs/superpowers/plans/2026-05-15-codegen-epic-pr1-loader-ux.md` | `7fa162c05` … `6371e0b27` |
| 2 | feature flags (NoUpdate / NoDelete / `entc.ReadOnly`) | ✅ complete | `docs/superpowers/plans/2026-05-15-codegen-epic-pr2-feature-flags.md` | `1982c3389` … `348be29dd` |
| 3 | predicate collapse | ✅ complete | `docs/superpowers/plans/2026-05-16-codegen-epic-pr3-predicate-collapse.md` | `077a968ac` … `e1ff5131a` |
| 4 | generic builders | ✅ complete | `docs/superpowers/plans/2026-05-16-codegen-epic-pr4-generic-builders.md` | `b582f7c2b` … `5abd2073d` |
| 5 | mutation collapse (biggest LOC pull, ~355K) | ✅ complete | `docs/superpowers/plans/2026-05-16-codegen-epic-pr5-mutation-collapse.md` | `6bdc7d5ab` … `7b9242f8a` |
| 6 | per-entity packages (biggest compile-time win) | ⏳ next | not yet written | — |

**Branch state:** all commits live on `worktree-wiggly-singing-pancake`. `master` is at `origin/master` (`7e9d99b1435d541286a773ca128be1a1931d6cc8`), untouched. No upstream configured; no PRs opened. To resume in a new session, see §11.

## 1. Problem

The matthewsreis fork of `entgo.io/ent` generates volumes of Go code that bring CI to its knees at production scale.

**Bloat shape** (consumer: `service-api-go/api-graphql`, 134 entities):

- 1.69M LOC across 2,501 generated files, 65 MB on disk
- ≈ 12,600 LOC per entity
- Dominant per-entity contributors **that the compiler sees**:
  - `where.go` ≈ 1K LOC of per-(field × op) predicate factories
  - `create.go` / `update.go` per-field setter explosions
  - `internal/<entity>_mutation.go` ≈ one struct field per column/edge per entity (355K LOC aggregate across all entities)
- `internal/schema.go` is 2.3 MB / 72K-LOC, **but is excluded from `go build` via `//go:build tools`** — it does not contribute to compile-time cost. See the regression test at `entc/internal/snapshot_buildtag_test.go`. The snapshot still costs gen-time write I/O and inflates the regen diff, but isn't a compile-cost hotspot.
- 1,286 GraphQL-binding files at the root of `ent/gen`

**CI cost** (PR MatthewsREIS/gemini#3844 — an 11-line `go.mod` bump triggers a 28-min CI):

- `generate and build` job: 13 min wall
- `task generate`: 5m52s wall, ~46 min CPU, 7 GB peak RSS
- `go build` of main binary: 3 min, **12.8 GB peak RSS**, 9.88M disk writes
- Workflow pins `GOMEMLIMIT: 28GiB` and has a built-in stale-cache fallback (`"Warm generation failed — clearing stale cache and retrying cold"`)

**Architectural force.** ent's design motto is *"100% statically typed and explicit API."* Each entity gets a sealed per-entity predicate type (`predicate.User` vs `predicate.Post`) and a sealed `OrderField` constructor. That guarantee is what forces every method signature to be entity-concrete (`UserQuery.Where(predicate.User)`) instead of generic — and that's what blows up the LOC.

**Developer-workflow pain (separate from compile cost, addressed by PR 1).** The loader (`entc/load/load.go:118`) runs `packages.Load` with `NeedTypes | NeedTypesInfo` — full Go type-checking of the schema package and everything it imports. If a hook in `ent/schema/user.go` references `m.SetNewField(v)` after a new field is added but before codegen has run, the schema package fails to type-check (`has no field or method SetNewField`) and codegen aborts. `IsBuildError` (entc/internal/snapshot.go:218-234) does not match this error class, so snapshot recovery doesn't trigger; even if it did, snapshot replay regenerates the *old* schema's code, which still has no `SetNewField`. The current workaround is "comment out the hook, regenerate, uncomment" — a known DX paper-cut for the team.

**Already shipped in this fork.** `runtime/entbuilder/` provides `SimpleField[C,N,M,T]`, `NillableField[C,N,M,T]`, `FieldWithScanner[C,N,M,T]` — generic descriptor helpers used by the `create` template. Per `COMPACT_HELPERS_RESULTS.md`:

| Metric | Master | Compact helpers | Delta |
|---|---|---|---|
| Generation wall time | 43.9s | 18.6s | **−57%** |
| Build wall time | 11.6s | 11.8s | +1.7% (noise) |
| Generated LOC | (baseline) | (baseline) | **−99K lines** |

**This is the central observation of the epic:** LOC reduction via generics alone does not move build time *at small fixture scale*. Build time at scale is dominated by per-package working-set size and per-package serialization. We must attack both LOC and the compile model.

**Baseline state in this worktree.** Four pre-existing `entc/gen` regression test failures (template-output snapshots that have drifted from current codegen output). PR 0 fixes them so the stack starts from green.

## 2. Goals and non-goals

**Goal:** Make Go builds materially faster for projects consuming ent-generated code at 100+ entities, measured by:

- `task generate` wall time and peak RSS
- `go build` wall time and peak RSS
- Total LOC of generated code

**Non-goals:**

- Changing ent's runtime semantics or query behavior
- Maintaining bit-for-bit API compatibility — one-release deprecation shims are acceptable
- Switching ORMs or abandoning codegen
- Reducing the 1,286 GraphQL-binding files — separate epic
- Tree-shaking based on consumer usage — v2

## 3. Approach

Seven PRs in a **git-spice stack**, branch prefix `codegen-epic/`. Each attacks a different layer and ships with its own measurement gate. Four layers of attack, applied in order:

1. **Fix the developer-workflow deadlock** (PR 1): make codegen survive schema-side code referencing not-yet-generated types so the team stops paying the "comment out, regenerate, uncomment" tax.
2. **Eliminate work outright or let consumers opt out** (PR 2): negative feature flags for categories the consumer doesn't need.
3. **Shrink what each entity emits** (PRs 3, 4, 5): generics, helpers, and reflection/unsafe push per-entity boilerplate into shared runtime.
4. **Shard the compile model** (PR 6): split the giant root `ent` package into per-entity sub-packages so the Go compiler can parallelize and sees smaller per-package working sets.

Order rationale: PR 1 first because it's the active DX pain. Then cheap structural changes to validate the bench methodology. API-visible changes in the middle. Biggest structural refactor (per-package split) last so it doesn't confound earlier measurements.

**Note (2026-05-15):** The original PR 2 ("helper de-inlining in templates") was DROPPED after investigation showed the dedup work was already shipped — upstream commit `68a453357` minimized `where.tmpl` to 1-line `sql.FieldEQ`-style delegations, and this fork's COMPACT_HELPERS work already shipped descriptor helpers in `runtime/entbuilder/` that compact `create.tmpl`/`update.tmpl`. PRs 3-7 are renumbered to 2-6.

### Bloat layer → PR mapping

| Layer | LOC contribution (134-entity baseline, est.) | Attacked by PR |
|---|---|---|
| Schema-build deadlock when hooks reference not-yet-generated types | DX, not LOC | 1 |
| Selective generation knobs | varies (consumer choice) | 2 |
| `where.go` predicate factories | ~134K (~1K × 134 entities) | 3 |
| `_query.go` / `_update.go` / `_delete.go` builders | ~150–200K | 4 |
| `internal/<entity>_mutation.go` | 355K | 5 |
| Type-checker working set (per-package symbol table size) | structural | 6 |

## 4. The stack

### PR 0 — `codegen-epic/00-baseline`

**Scope.** Bench harness + stabilize the test baseline.

**Changes:**
- New `internal/bench/` (or `cmd/bench-codegen/`) tool that:
  - Runs the ent test schemas through `entc.Generate`
  - Times each generation pass and the subsequent `go build ./...`
  - Records: gen wall, gen peak RSS, build wall, build peak RSS, total LOC, per-file LOC for the top-20 largest files, generated file count
  - Outputs newline-delimited JSON for cross-PR diffing
- Fix the four pre-existing `entc/gen` regression failures:
  - `TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport` (gofmt error in generated `create.go`)
  - `TestGraph_Gen_AssignGeneratedUserIntIDAcceptsInt` (undefined `userCreateDescriptor`)
  - `TestGraph_Gen_SQLModifierDeleteBuilderHasModify` (`_d.modifiers undefined` on `*TaskDelete`)
  - `TestGraph_Gen_SQLSchemaConfigHooksInDescriptorPaths` (missing `joinT.Schema(_q.schemaConfig.UserGroups)` in generated `user_query.go`)
- Document the procedure for running the bench against a downstream consumer in `internal/bench/README.md`.

**Acceptance:**
- `go test ./entc/gen/...` passes clean
- Bench tool produces JSON output for the in-repo test schemas
- Bench output committed as `internal/bench/baseline.jsonl` for the stack to diff against

**Risk:** Low.

**Consumer impact:** None.

---

### PR 1 — `codegen-epic/01-loader-ux`

**Scope.** End the "comment-out-hook, regenerate, uncomment" workflow paper-cut. Two complementary changes: make the snapshot recovery path trigger on the error class that's actually hitting users, and add a bootstrap mode that lets codegen complete even when hooks reference not-yet-generated types.

**Changes:**

1. **Extend `IsBuildError`** (entc/internal/snapshot.go:218-234) to match the type-checker error class produced when schema-side Go references symbols that don't exist in the generated package yet:
   - `undefined:`
   - `has no field or method`
   - `undeclared name`
   - `cannot use`
   - Any additional substrings surfaced by the team's recent incident history (grep CI logs)

   Each match must be unit-tested. Today these errors fall through `mayRecover` (entc/entc.go:443) and abort codegen — extending the match opens the recovery door.

2. **Bootstrap mode** — new `entc.SkipHookCompilation()` option and `--skip-hook-compilation` CLI flag (final naming TBD during implementation). When set:
   - entc copies the schema package source to a tmpdir
   - AST-strips the method bodies of `Hooks() []ent.Hook`, `Policy() ent.Policy`, and `Interceptors() []ent.Interceptor` on every Schema-implementing type, replacing the body with `return nil` (preserving signatures so the type still implements `ent.Schema`)
   - Loader runs `packages.Load` against the stripped copy — succeeds because the references to generated types have been removed with the hook bodies
   - Codegen runs against the schema definitions (Fields, Edges, Mixins, Annotations, Indexes, Config) extracted from the stripped package
   - The user's real schema package is never modified; once codegen completes, it compiles normally because the new generated methods now exist

3. **Auto-fallback path.** When loader fails AND `IsBuildError` matches AND `FeatureSnapshot` is on, try in order:
   a. Snapshot restore (existing path — useful for merge conflicts and hand-edits to generated code)
   b. If that fails or produces stale output, bootstrap-mode reload with the *current* schema
   c. If both fail, surface a clear error message pointing at the bootstrap flag in case the user wants to opt in manually

**Known limitations** (documented, not fixed in this PR):
- Hooks defined in **subpackages** of the schema package that themselves import the generated `ent` package — stripping the schema package's `Hooks()` body doesn't help, because the subpackage's bad imports still fail to compile. Workaround: keep hooks that touch generated mutation types in a package that is *not* imported by the schema package (most teams already do this).
- Schemas whose `Fields()` / `Edges()` methods compute results from runtime values — bootstrap mode still runs those methods; only `Hooks` / `Policy` / `Interceptors` get stripped. If `Fields()` itself references a generated type, you're still stuck.

**Acceptance:**
- New regression test (`entc/internal/bootstrap_test.go` or similar): schema with a `Hooks()` method that references `m.SetNewField(v)` where `NewField` is not yet generated — codegen succeeds via the bootstrap path
- New regression test: schema with a `Policy()` referencing not-yet-generated mutation methods — same outcome
- New unit tests for each new `IsBuildError` substring
- Documentation page (`doc/md/devx-bootstrap.md` or similar) explaining when to use the flag and what it doesn't cover
- Existing `TestSnapshot_Restore` and `TestSnapshotIsBuildTagged` continue to pass

**Risk:** Medium. AST surgery on user source code is fiddly but mechanical; the surface area is three Schema-interface methods. Failure mode is benign — bootstrap mode falls back to surfacing the original error.

**Consumer impact:** Additive — opt-in flag. Existing behavior unchanged when the flag isn't set. The extended `IsBuildError` matching could cause snapshot recovery to trigger on errors it previously ignored; this is intended, but recovery's preexisting limitation (snapshot only knows the old schema) means it might regenerate stale code in some edge cases. Mitigated by the auto-fallback ordering above.

---

### PR 2 — `codegen-epic/02-feature-flags`

(Was PR 3 in the original numbering — promoted after the original PR 2 "helper de-inlining" was dropped on 2026-05-15.)

**Scope.** Add negative feature flags so consumers can opt out of categories of generation.

**Changes:**
- Extend `entc/gen/feature.go`:
  - `FeatureNoMutations` — skip generating mutation types
  - `FeatureNoUpdate` — skip update builders
  - `FeatureNoDelete` — skip delete builders
  - `FeatureNoGraphQL` — suppress entgql annotation output if present
- Per-entity annotation: `entc.ReadOnly()` on a schema disables update/delete on that entity even when global flags say keep them
- Defaults: every flag off (current behavior preserved)

**Acceptance:**
- All existing tests pass with defaults
- New tests cover each feature flag (a test schema with the flag on produces no output for the suppressed category)
- Documentation page added under `doc/md/`

**Risk:** Low. Additive only.

**Consumer impact:** None unless enabled.

---

### PR 3 — `codegen-epic/03-predicate-collapse`

**Scope.** Replace per-(field × op) predicate factories with one generic op per type + field constants.

**Changes:**
- New shared package `entgo.io/ent/where` exporting:
  - `func EQ[T comparable](field string, v T) sql.P`
  - `func NEQ[T comparable](field string, v T) sql.P`
  - `func In[T comparable](field string, vs ...T) sql.P` (and `NotIn`)
  - `func GT[T constraints.Ordered](field string, v T) sql.P` (and `GTE`, `LT`, `LTE`)
  - String-specific: `Contains`, `HasPrefix`, `HasSuffix`, `ContainsFold`, etc.
- Generated `<entity>/where.go` shrinks: emits the field constants (already present today) and a subset of deprecated wrappers for migration
- Deprecation policy: keep `user.NameEQ(x)` as `// Deprecated: use where.EQ(user.FieldName, x)` for one release

**Acceptance:**
- All existing tests pass
- New tests cover the generic op functions across all supported scalar types
- LOC delta target: 70–80% reduction in `where.go` per entity at consumer scale
- Regression fixtures regenerated; fixture diffs reviewed alongside template diffs

**Risk:** Medium. Public API change with deprecation shim; possible Go generic type-inference quirks at call sites with literal arguments.

**Consumer impact:** Call sites change from `user.NameEQ(x)` to `where.EQ(user.FieldName, x)`. One-release deprecation shim. New `MIGRATION.md` section.

---

### PR 4 — `codegen-epic/04-generic-builders`

**Scope.** Extend the `runtime/entbuilder` descriptor pattern (already used for create) to query, update, delete, and edge loaders.

**Changes:**
- New types in `runtime/entbuilder`:
  - `Query[T, P]` with shared `All`, `Count`, `Exist`, `IDs`, `First`, `Only`, `Limit`, `Offset`, `Order`, `Clone`, `GroupBy`, `Select`, `Aggregate`
  - `Update[T]`, `Delete[T]` similar
- Generated `<entity>_query.go` becomes a thin file: `type UserQuery = entbuilder.Query[User, predicate.User]` + entity-specific edge accessors (`WithGroups` etc., which can't be made generic without losing edge-relation type names)
- Same shape for `_update.go` and `_delete.go`

**Acceptance:**
- All existing tests pass
- LOC delta target: 60% reduction in `_query.go` + `_update.go` + `_delete.go` at consumer scale
- Gen-time RSS reduction (fewer files, shorter templates) — follows COMPACT_HELPERS precedent
- **Build-time RSS measurement at consumer scale is the critical test:** does generics-based reduction move build time at 134-entity scale even when it didn't at small fixture scale? Record carefully; results inform whether PR 6's package-split is the only build-time lever or one of several.

**Risk:** Medium. Already-proven pattern (`SimpleField` etc. shipped via COMPACT_HELPERS), but new ground for query semantics. Likely escape hatches: per-entity overrides for `GroupBy`/`Select` typing with arbitrary `any` results.

**Consumer impact:** Mostly transparent — the exported type `*ent.UserQuery` resolves through the type alias. Edge case: code doing `reflect.TypeOf((*ent.UserQuery)(nil))` may see the underlying generic type name. Document.

---

### PR 5 — `codegen-epic/05-mutation-collapse`

**Scope.** Drop per-entity `<entity>_mutation.go` files. Replace with generic `Mutation[T]` backed by a per-entity field descriptor.

**Changes:**
- New type in `runtime/entbuilder`: `Mutation[T any]` with:
  - `Field(name string) (any, bool)` — typed value access via descriptor
  - `SetField(name string, v any) error`
  - `ClearField(name string)`
  - `ResetField(name string)`
  - `Op() Op`, `Type() string`, etc. — unchanged
- Per-entity codegen emits a `mutationDesc` constant: field-name → (offset, type, nillable, has-default, …). One small table per entity replaces ~1300 method/field decls.
- Generic mutation uses the descriptor for dispatch.
- Two implementations to prototype and benchmark within this PR:
  - **A) `unsafe.Offsetof` table** — ~2 ns/access, requires confidence in struct layout invariants
  - **B) Cached `reflect` indices** — ~80 ns/access (cached `map[reflect.Type]map[string]int`), no `unsafe`
- Migration shim: behind opt-in `FeatureTypedMutation` (from PR 2), codegen synthesizes the old typed methods (`SetName`, `Name`, `ClearName`) as 1-line wrappers around the generic API. Shipped for one release, removed in the next.

**Acceptance:**
- All existing tests pass; hook integration tests carefully reviewed
- LOC delta: 300K+ reduction in `/internal/` at consumer scale
- Build-time RSS target: 30%+ reduction (largest single contributor to type-checker symbol-table size)
- Hook benchmark: mutation field access ≤ 100 ns mean (acceptance gate for choosing between A and B)

**Risk:** High. Hook surface change. Some downstream consumers may have hand-written code referencing typed mutation fields directly. Mitigations: `FeatureTypedMutation` opt-in shim, thorough `MIGRATION.md`, ahead-of-release communication.

**Consumer impact:** Hook authors update from `m.SetName(v)` / `m.Name()` to `m.SetField(user.FieldName, v)` / `m.Field(user.FieldName)`, or opt into `FeatureTypedMutation` for one release while migrating.

**Migration angle — generic API already exists today.** Every generated `<entity>_mutation.go` already ships the generic methods `Field(name) (ent.Value, bool)`, `SetField(name, v) error`, `AddedField`, `AddField`, `ClearField`, `ResetField`, `FieldCleared`, `OldField`, `Fields()` (see e.g. `entc/integration/privacy/ent/internal/task_mutation.go:452-578`). Hook authors can adopt this API **before this PR ships** as a one-time migration; doing so also resolves the chicken-and-egg pain that PR 1 addresses from a different angle (string-keyed access doesn't depend on per-field methods being generated). PR 5's contribution is to make this the *only* API and delete the typed methods (and the 355K LOC of struct fields backing them). Document this migration path explicitly in `MIGRATION.md` at the start of PR 5, ahead of any deprecation shim work.

---

### PR 6 — `codegen-epic/06-per-entity-packages`

**Scope.** Move per-entity files from root `ent` package into `ent/<entity>` sub-packages. Make root `ent` a thin facade.

**Changes:**
- Codegen output layout:
  - Before: `ent/user_query.go`, `ent/user_create.go`, `ent/user_update.go`, `ent/user_delete.go`, `ent/user.go`
  - After: `ent/user/query.go`, `ent/user/create.go`, `ent/user/update.go`, `ent/user/delete.go`, `ent/user/entity.go` (consolidates with existing `ent/user/where.go`, `ent/user/shared.go`)
- Root `ent/` retains:
  - `client.go` with `User() *user.Client` dispatch
  - Type aliases: `type UserQuery = user.Query` etc. (for backwards compatibility with `*ent.UserQuery` references)
  - `ent.go`, `tx.go`, `config.go` (global, one per schema)
- Per-entity sub-packages depend on shared runtime (`runtime/entbuilder`, `dialect/sql/sqlgraph`) but NOT on the root `ent` package or on each other.

**Cyclic-import mitigation:** Entity types and predicate types (referenced across packages via M2M/O2M edges) live in `ent/user/types/` leaf sub-packages. Builders in `ent/user/` depend on those leaf packages. This keeps edge relationships acyclic.

**Acceptance:**
- All existing tests pass
- Bench at consumer scale: `go build` wall and RSS show ≥ 40% RSS reduction due to package-parallel compilation and smaller per-package working sets
- `go test ./...` runs in parallel per-package

**Risk:** High. Massive structural refactor. Risk of cyclic imports between per-entity packages when relations are bidirectional. Mitigated by the leaf-package pattern above.

**Consumer impact:** Type aliases handle most user-facing `*ent.UserQuery` references. Hand-written code reaching into `ent.UserMutation` fields directly needs path updates. Migration: regenerate, run `goimports`, fix remaining direct references.

## 5. Cross-cutting concerns

### 5.1 Measurement methodology

Two-tier measurement. Every PR records numbers at two scales:

1. **In-repo fixture scale** — ent's own integration test schemas. Fast, deterministic, runs in CI. Mandatory for every PR.
2. **Consumer scale** — `service-api-go/api-graphql` (134 entities, the real workload). Run manually before merging high-LOC-impact PRs (3, 4, 5, 6). Tracked in `BENCH_RESULTS.md` per PR. PR 1 is a DX-only change and doesn't move these numbers; its acceptance gates are the bootstrap-mode regression tests.

**Metrics per fixture:**

- `task generate` wall time
- `task generate` peak RSS (via `/usr/bin/time -v` or platform equivalent)
- `go build ./...` wall time
- `go build ./...` peak RSS
- Total LOC of generated output
- Per-file LOC for the top 20 largest generated files
- Number of generated files

**PR description requirement:** every PR includes a `## Measurement` section with these metrics versus the prior PR. No exceptions.

### 5.2 Regression fixture handling

Many `entc/gen` regression tests compare against snapshot output of generated code. PRs that change template output (3, 4, 5, 6) regenerate fixtures as part of the PR. PR 0 adds a `make regen-fixtures` target so this is mechanical. Reviewers diff template changes alongside fixture changes; the bench tool includes a fixture-diff summarizer to keep review tractable. PR 1 changes the loader, not templates — no fixture churn. PR 2 (feature flags) is additive defaults-off — no fixture churn either.

### 5.3 Deprecation policy

PRs 3 and 5 ship deprecation shims for **one release**. Shim files are clearly marked `// Deprecated: ...`. Removed in the release after. PR 5's shim is itself gated by an opt-in feature flag (`FeatureTypedMutation`) so consumers who don't need it pay no cost.

### 5.4 Consumer re-baseline (recommended, not blocking)

Before PR 0 lands, run codegen against `service-api-go/api-graphql` with the current matthewsreis fork (which already includes COMPACT_HELPERS). The 99K-LOC reduction from `SimpleField` may not yet have been absorbed by the consumer's regenerated output. If a significant chunk of the 1.69M baseline vanishes from a fresh regen, the per-PR headroom estimates in §4 are revised downward. Optional but informative.

### 5.5 PR ordering interactions

- PR 5 (mutation collapse) depends on PR 2 (feature flags) for the `FeatureTypedMutation` compat shim.
- PR 6 (per-entity packages) must come **after** all template-shrinking PRs (3, 4, 5). Splitting before shrinking shards the bloat instead of removing it.

## 6. Decisions deferred

- **PR 5: `unsafe.Offsetof` vs cached `reflect`** for mutation field access. Prototype both within PR 5; choose based on bench numbers and consumer concerns about shipping `unsafe` in a generated-code path.
- **`FeatureTypedMutation` lifetime past one release.** Default: remove after one release. Revisit based on downstream feedback during PR 5's release cycle.
- **Per-entity sub-package naming.** ent currently uses singular (`ent/user/where.go`). Default: stay singular (`ent/user/query.go`). Bikeshed welcome.

## 7. Risks

| Risk | PR(s) | Mitigation |
|---|---|---|
| LOC reduction does not translate to build-time wins at scale (COMPACT_HELPERS was neutral at fixture scale) | 3, 4, 5 | Measure at consumer scale before merging. If 3/4 land neutral, accelerate PR 6 (which moves build time via package parallelism regardless of LOC). |
| `unsafe.Offsetof` mutation table introduces subtle bugs on struct layout changes | 6 | Fall back to cached-reflect; add init-time validation that descriptor matches struct |
| Per-entity package split creates cyclic imports (M2M edges referencing each other) | 7 | Leaf `ent/<entity>/types` sub-packages for entity + predicate types; builders depend on leaves without depending on each other |
| Downstream consumers have hand-written code against deprecated APIs | 3, 5 | One-release deprecation shims, `MIGRATION.md` entries, ahead-of-release communication |
| `entc/gen` regression fixtures churn enormously per PR, making review hard | All template PRs | Bench tool includes fixture-diff summarizer; reviewers focus on template + bench, fixtures verified mechanically |
| Generic instantiation cost (post-Go-1.18 GC-shape stenciling) erodes the LOC win at scale | 4, 5 | Measure; cap generic depth in templates; some methods stay non-generic if benchmarks demand |
| Bootstrap-mode AST stripping of `Hooks` / `Policy` / `Interceptors` fails to satisfy `ent.Schema` (e.g., the stripped stub returns wrong type) and breaks the loader | 1 | Comprehensive test suite of schema-interface signatures; explicit signature check after stripping; fallback to surfacing the original loader error rather than masking it |
| Bootstrap mode doesn't cover hooks in **subpackages** that import the generated `ent` (documented limitation) | 1 | Documented in PR 1; the practical guidance is "keep generated-type-touching hooks in a package not imported by your schema package" — already standard ent practice for most teams |

## 8. Success metrics for the epic as a whole

At consumer scale (`service-api-go/api-graphql`, 134 entities). Numbers are targets to validate; revise after PR 0 + consumer re-baseline.

| Metric | Baseline | Target |
|---|---|---|
| Total generated LOC | 1.69M | ≤ 850K (≥ 50% reduction) |
| `task generate` wall | 5m52s | ≤ 2m30s |
| `task generate` peak RSS | 7 GB | ≤ 3 GB |
| `go build` wall (full binary) | 3 min | ≤ 1m30s |
| `go build` peak RSS | 12.8 GB | ≤ 5 GB |
| Generated file count | 2,501 | not a metric — per-entity split may *raise* file count while lowering compile cost |

If COMPACT_HELPERS-style reduction continues to be neutral on build time at scale (i.e., generic instantiation cost cancels LOC savings), success becomes anchored on PR 6's package-parallelism win. If both kinds of reduction compound as we expect, the targets above are conservative.

## 9. Out of scope

- Reducing the 1,286 GraphQL-binding files (`ent/gen/gql_*.go`) — separate epic
- Tree-shaking / dead-code-elimination based on consumer usage analysis — v2
- Switching to a different ORM or abandoning codegen wholesale
- Struct composition with embedded bases as a top-level pattern — generics subsume it
- An interface-based polymorphism layer — generics subsume it

## 10. Implementation cadence

- PR 0 first; unblocks measurement.
- PR 1 (loader UX) next; shipped DX win.
- PRs 2 (feature flags), 3 (predicate collapse), 4 (generic builders) are functionally independent; sequence 2 → 3 → 4 in the linear stack.
- PR 5 (mutation collapse) after PR 2 (uses `FeatureTypedMutation` compat shim) and after PR 4 (similar generic-descriptor shape).
- PR 6 (per-entity packages) last, after all template-shrinking PRs (3, 4, 5).

Per-PR implementation plans are written via the `writing-plans` skill **immediately before starting each PR**, not all upfront. This avoids pre-designing PRs whose requirements will shift based on prior PR measurements.

## 11. Resuming in a new session

A fresh session has the auto-loaded MEMORY index pointing at this spec. The minimal startup sequence:

1. **Verify environment.** Confirm worktree is at `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake`, branch is `worktree-wiggly-singing-pancake`, `master` is at `7e9d99b1435d541286a773ca128be1a1931d6cc8` (origin/master). No upstream should be configured.
2. **Verify the epic is green.** Run `go test -count=1 ./entc/...` — every package should pass. Run `go test -count=1 -run 'TestBootstrap_Skip|TestFeatureNo|TestReadOnly|TestSnapshot' ./entc/... ./entc/internal/...` to confirm the headline regression tests from PRs 0-2 all pass.
3. **Decide the next move.** Either run the consumer-scale bench (`go run ./cmd/bench-codegen -schema /path/to/service/ent/schema -label service-baseline -out /tmp/...`) to see how the shipped PR 0-2 work has landed in real numbers, OR proceed to write the PR 3 (predicate collapse) implementation plan via the `writing-plans` skill.
4. **Stay local.** Per [[feedback-no-prs-until-end-of-epic]], no `git push`, no `gh pr create`, no `gs branch submit` / `gs stack submit` until every PR is built locally and bench numbers confirm DX + build-time wins.

### Known artefacts that are NOT issues

- Pre-existing uncommitted modifications in `entc/integration/privacy/ent/task/where.go`, `team/where.go`, `team_query.go`, `user/where.go`, `task_query.go`, `user_query.go`. These were already there when the worktree was created and are NOT from any epic PR. Ignore them or `git checkout --` them at session start if they create noise.

### Recurring failure mode (3 incidents during PRs 0-2)

Implementer subagents sometimes commit to `master` instead of `worktree-wiggly-singing-pancake` because they `cd` to the parent repo at `/var/home/smoothbrain/dev/matthewsreis/ent/` (which is on master). After every implementer subagent reports DONE, the controller has been verifying:

```bash
git rev-parse --abbrev-ref HEAD   # must be worktree-wiggly-singing-pancake
git log -1 --oneline              # the new commit
git rev-parse master              # must still be 7e9d99b1435d541286a773ca128be1a1931d6cc8
```

If the new commit landed on master, recover with:
```bash
git cherry-pick <wrong-master-sha>
git update-ref refs/heads/master origin/master
```

Future implementer prompts should include explicit `pwd && git rev-parse --abbrev-ref HEAD` re-verification immediately before every `git commit`. Three of ~20 subagent dispatches hit this; the recovery is mechanical but should be expected.
