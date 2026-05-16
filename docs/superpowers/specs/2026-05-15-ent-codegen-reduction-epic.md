# ent codegen reduction — epic design

**Status:** Draft for review
**Date:** 2026-05-15
**Worktree:** `.claude/worktrees/wiggly-singing-pancake`
**Stack tool:** git-spice

## 1. Problem

The matthewsreis fork of `entgo.io/ent` generates volumes of Go code that bring CI to its knees at production scale.

**Bloat shape** (consumer: `service-api-go/api-graphql`, 134 entities):

- 1.69M LOC across 2,501 generated files, 65 MB on disk
- ≈ 12,600 LOC per entity
- Dominant per-entity contributors:
  - `where.go` ≈ 1K LOC of per-(field × op) predicate factories
  - `create.go` / `update.go` per-field setter explosions
  - `internal/<entity>_mutation.go` ≈ one struct field per column/edge per entity (355K LOC aggregate across all entities)
- Single biggest hotspot: `internal/schema.go` is a 2.3 MB / 72K-LOC serialized Go constant
- 1,286 GraphQL-binding files at the root of `ent/gen`

**CI cost** (PR MatthewsREIS/gemini#3844 — an 11-line `go.mod` bump triggers a 28-min CI):

- `generate and build` job: 13 min wall
- `task generate`: 5m52s wall, ~46 min CPU, 7 GB peak RSS
- `go build` of main binary: 3 min, **12.8 GB peak RSS**, 9.88M disk writes
- Workflow pins `GOMEMLIMIT: 28GiB` and has a built-in stale-cache fallback (`"Warm generation failed — clearing stale cache and retrying cold"`)

**Architectural force.** ent's design motto is *"100% statically typed and explicit API."* Each entity gets a sealed per-entity predicate type (`predicate.User` vs `predicate.Post`) and a sealed `OrderField` constructor. That guarantee is what forces every method signature to be entity-concrete (`UserQuery.Where(predicate.User)`) instead of generic — and that's what blows up the LOC.

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

Eight PRs in a **git-spice stack**, branch prefix `codegen-epic/`. Each attacks a different layer and ships with its own measurement gate. Three layers of attack, applied in order:

1. **Eliminate work outright** (PR 1, 3): remove artifacts from the compile graph or let consumers opt out of categories.
2. **Shrink what each entity emits** (PRs 2, 4, 5, 6): generics, helpers, embedded data, and reflection/unsafe push per-entity boilerplate into shared runtime.
3. **Shard the compile model** (PR 7): split the giant root `ent` package into per-entity sub-packages so the Go compiler can parallelize and sees smaller per-package working sets.

Order rationale: cheap structural changes first to validate the bench methodology, API-visible changes in the middle, the biggest structural refactor (per-package split) last so it doesn't confound earlier measurements.

### Bloat layer → PR mapping

| Layer | LOC contribution (134-entity baseline, est.) | Attacked by PR |
|---|---|---|
| `internal/schema.go` Go constant | 72K (one file) | 1 |
| Repeated bodies in `where` / `create` / `update` templates | ~100K | 2 |
| Selective generation knobs | varies (consumer choice) | 3 |
| `where.go` predicate factories | ~134K (~1K × 134 entities) | 4 |
| `_query.go` / `_update.go` / `_delete.go` builders | ~150–200K | 5 |
| `internal/<entity>_mutation.go` | 355K | 6 |
| Type-checker working set (per-package symbol table size) | structural | 7 |

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

### PR 1 — `codegen-epic/01-embed-schema`

**Scope.** Replace the 2.3 MB `internal/schema.go` Go constant with `//go:embed schema.json` + a tiny init decoder.

**Changes:**
- New template emits `schema.json` alongside generated Go
- New template emits `internal/schema.go` as a ~20-line decoder using `//go:embed` + `encoding/json`
- Runtime parses JSON once at init, populates the existing schema variable
- Pre-check (grep) that no consumer of the schema constant requires it to be a compile-time-evaluable expression

**Acceptance:**
- All existing tests pass
- Bench shows measurable reduction in build RSS attributable to removing one giant constant
- LOC delta in generated output: −72K at consumer scale (confirm by re-running on consumer)

**Risk:** Low. Hypothesis: nothing does compile-time reflection on the constant. Mitigation: grep first, fail the PR review if a use is found.

**Consumer impact:** Transparent. Regenerate, output is functionally identical.

---

### PR 2 — `codegen-epic/02-helper-dedup`

**Scope.** Extract repeated template bodies in `where.go`, `create.go`, `update.go` into per-type private helpers in the generated file.

**Changes:**
- In `entc/gen/template/dialect/sql/where.tmpl`: predicate bodies that are emitted identically 8× per field collapse to 1-line calls into per-type helpers (`eqString`, `eqInt`, `eqTime`, …) emitted once at the bottom of the same generated file.
- Same pattern in `create.tmpl` and `update.tmpl` for repeated setter/getter bodies.
- No public API change.

**Acceptance:**
- All existing tests pass
- Regression fixtures regenerated within this PR
- LOC delta target: 30–50% reduction inside `where.go` / `create.go` / `update.go` at consumer scale

**Risk:** Low. Pure template refactor; behavior unchanged.

**Consumer impact:** Transparent.

---

### PR 3 — `codegen-epic/03-feature-flags`

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

### PR 4 — `codegen-epic/04-predicate-collapse`

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

### PR 5 — `codegen-epic/05-generic-builders`

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
- **Build-time RSS measurement at consumer scale is the critical test:** does generics-based reduction move build time at 134-entity scale even when it didn't at small fixture scale? Record carefully; results inform whether PR 7's package-split is the only build-time lever or one of several.

**Risk:** Medium. Already-proven pattern (`SimpleField` etc. shipped via COMPACT_HELPERS), but new ground for query semantics. Likely escape hatches: per-entity overrides for `GroupBy`/`Select` typing with arbitrary `any` results.

**Consumer impact:** Mostly transparent — the exported type `*ent.UserQuery` resolves through the type alias. Edge case: code doing `reflect.TypeOf((*ent.UserQuery)(nil))` may see the underlying generic type name. Document.

---

### PR 6 — `codegen-epic/06-mutation-collapse`

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
- Migration shim: behind opt-in `FeatureTypedMutation` (from PR 3), codegen synthesizes the old typed methods (`SetName`, `Name`, `ClearName`) as 1-line wrappers around the generic API. Shipped for one release, removed in the next.

**Acceptance:**
- All existing tests pass; hook integration tests carefully reviewed
- LOC delta: 300K+ reduction in `/internal/` at consumer scale
- Build-time RSS target: 30%+ reduction (largest single contributor to type-checker symbol-table size)
- Hook benchmark: mutation field access ≤ 100 ns mean (acceptance gate for choosing between A and B)

**Risk:** High. Hook surface change. Some downstream consumers may have hand-written code referencing typed mutation fields directly. Mitigations: `FeatureTypedMutation` opt-in shim, thorough `MIGRATION.md`, ahead-of-release communication.

**Consumer impact:** Hook authors update from `m.SetName(v)` / `m.Name()` to `m.SetField(user.FieldName, v)` / `m.Field(user.FieldName)`, or opt into `FeatureTypedMutation` for one release while migrating.

---

### PR 7 — `codegen-epic/07-per-entity-packages`

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
2. **Consumer scale** — `service-api-go/api-graphql` (134 entities, the real workload). Run manually before merging high-impact PRs (1, 4, 5, 6, 7). Tracked in `BENCH_RESULTS.md` per PR.

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

Many `entc/gen` regression tests compare against snapshot output of generated code. PRs that change template output (1, 2, 4, 5, 6, 7) regenerate fixtures as part of the PR. PR 0 adds a `make regen-fixtures` target so this is mechanical. Reviewers diff template changes alongside fixture changes; the bench tool includes a fixture-diff summarizer to keep review tractable.

### 5.3 Deprecation policy

PRs 4 and 6 ship deprecation shims for **one release**. Shim files are clearly marked `// Deprecated: ...`. Removed in the release after. PR 6's shim is itself gated by an opt-in feature flag (`FeatureTypedMutation`) so consumers who don't need it pay no cost.

### 5.4 Consumer re-baseline (recommended, not blocking)

Before PR 0 lands, run codegen against `service-api-go/api-graphql` with the current matthewsreis fork (which already includes COMPACT_HELPERS). The 99K-LOC reduction from `SimpleField` may not yet have been absorbed by the consumer's regenerated output. If a significant chunk of the 1.69M baseline vanishes from a fresh regen, the per-PR headroom estimates in §4 are revised downward. Optional but informative.

### 5.5 PR ordering interactions

- PR 5 depends on PR 2 for clean template diffs (not functionally — re-orderable if needed).
- PR 6 depends on PR 3 for the `FeatureTypedMutation` compat shim.
- PR 7 must come **after** all template-shrinking PRs (4, 5, 6). Splitting before shrinking shards the bloat instead of removing it.

## 6. Decisions deferred

- **PR 6: `unsafe.Offsetof` vs cached `reflect`** for mutation field access. Prototype both within PR 6; choose based on bench numbers and consumer concerns about shipping `unsafe` in a generated-code path.
- **`FeatureTypedMutation` lifetime past one release.** Default: remove after one release. Revisit based on downstream feedback during PR 6's release cycle.
- **Per-entity sub-package naming.** ent currently uses singular (`ent/user/where.go`). Default: stay singular (`ent/user/query.go`). Bikeshed welcome.

## 7. Risks

| Risk | PR(s) | Mitigation |
|---|---|---|
| LOC reduction does not translate to build-time wins at scale (COMPACT_HELPERS was neutral at fixture scale) | 4, 5, 6 | Measure at consumer scale before merging. If 4/5 land neutral, accelerate PR 7 (which moves build time via package parallelism regardless of LOC). |
| `unsafe.Offsetof` mutation table introduces subtle bugs on struct layout changes | 6 | Fall back to cached-reflect; add init-time validation that descriptor matches struct |
| Per-entity package split creates cyclic imports (M2M edges referencing each other) | 7 | Leaf `ent/<entity>/types` sub-packages for entity + predicate types; builders depend on leaves without depending on each other |
| Downstream consumers have hand-written code against deprecated APIs | 4, 6 | One-release deprecation shims, `MIGRATION.md` entries, ahead-of-release communication |
| `entc/gen` regression fixtures churn enormously per PR, making review hard | All template PRs | Bench tool includes fixture-diff summarizer; reviewers focus on template + bench, fixtures verified mechanically |
| Generic instantiation cost (post-Go-1.18 GC-shape stenciling) erodes the LOC win at scale | 5, 6 | Measure; cap generic depth in templates; some methods stay non-generic if benchmarks demand |

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

If COMPACT_HELPERS-style reduction continues to be neutral on build time at scale (i.e., generic instantiation cost cancels LOC savings), success becomes anchored on PR 7's package-parallelism win. If both kinds of reduction compound as we expect, the targets above are conservative.

## 9. Out of scope

- Reducing the 1,286 GraphQL-binding files (`ent/gen/gql_*.go`) — separate epic
- Tree-shaking / dead-code-elimination based on consumer usage analysis — v2
- Switching to a different ORM or abandoning codegen wholesale
- Struct composition with embedded bases as a top-level pattern — generics subsume it
- An interface-based polymorphism layer — generics subsume it

## 10. Implementation cadence

- PR 0 first; unblocks measurement.
- PRs 1, 2, 3 are independent in function; for git-spice stack purposes, sequence 1 → 2 → 3 to keep the stack linear.
- PR 4 after 3.
- PR 5 after 2.
- PR 6 after 3 and 5.
- PR 7 after all of 4, 5, 6.

Per-PR implementation plans are written via the `writing-plans` skill **immediately before starting each PR**, not all upfront. This avoids pre-designing PRs whose requirements will shift based on prior PR measurements.
