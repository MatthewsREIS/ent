# Drop-in LOC and Compile-Time Reduction for ent Codegen

Design spec — Approach 1 (lean generics + 1-line generated shims).

- Date: 2026-04-23
- Branch: `i-h8-ent`
- Status: draft, pending user approval
- Prior related work: compact helpers in `runtime/entbuilder/` (commit `c93835e32` and ancestors)

## Problem

ent generates an extreme volume of code per schema. In the downstream consumer
`service-api-go/api-graphql/src/ent/gen` (111 schemas), the output is 2,343 Go
files / 1,624,012 lines / 62 MB. `go vet ./...` on that package crashes on
machines with over 100 GB of memory. IDE indexing is punishing. Cold builds are
slow. None of this is acceptable as the schema count grows.

The consumer cannot migrate off ent today. The fix must be a drop-in
replacement for the generated code: every exported symbol in `ent/gen/` keeps
its signature; consumer call sites do not change.

## Non-goals

- Changing schema definition ergonomics for authors.
- Runtime throughput or latency improvements. Single-digit-nanosecond
  regressions per hot-path operation are acceptable if they buy compile-time
  wins.
- Fixing `gql_*.go` bloat (entgql is a separate plugin; separate effort).
- Upstream-mergeability with `github.com/ent/ent`. This fork is already
  diverged and will remain so.

## Constraint: correctness first

The user has stated correctness > runtime nanoseconds explicitly. Design
decisions that trade correctness for reduction are rejected. Where an approach
introduces a new failure mode (e.g., descriptor/struct drift), the design must
show how that failure mode is detected at codegen or test time, not at query
time in production.

## Approach

**Lean generics with 1-line generated shims.**

- The generated typed shell is preserved: every `func`, method, constant, and
  type that exists in `ent/gen/` today exists with the same signature after
  the refactor.
- The *bodies* of those functions and methods collapse to single-line calls
  into generic helpers living in `runtime/entbuilder/`.
- Type parameters carry the type information; generated code carries naming,
  signatures, and schema-specific constants.

### Invariant

If a consumer's source compiles against today's `ent/gen/`, it compiles
against the refactored `ent/gen/` with no diff.

### Why Approach 1

Evaluated three candidates during brainstorming:

1. **Lean generics + 1-line shims** (this spec). Preserves full compile-time
   type safety. Estimated 60–70% source reduction on the targeted surface;
   compile-time win is the open question the POC must answer.
2. **Descriptor-driven runtime** (reflection + metadata). Bigger source and
   binary reduction (80–90%) but adds runtime failure modes (descriptor/struct
   drift, scan-path reflection). Held in reserve.
3. **Hybrid** (generics for predicates/setters, descriptor for mutation
   state). Targets the biggest-body category (`mutation.go`) with reflection
   while keeping the rest compile-safe. Held in reserve.

Rationale for starting with #1: smallest correctness-risk surface, fastest
path to a quantitative answer on the compile-time question. If #1's numbers
disappoint, we already have #2 and #3 designed as fallbacks.

## Target files

Ranked by LOC impact in `service-api-go/api-graphql/src/ent/gen` (111
schemas). Numbers are from the current state of the consumer repo.

| Category | LOC today | Expected LOC | Technique |
|---|---|---|---|
| `<schema>/create.go` | 248,905 | ~75,000 | 1-line setters delegating to generic mutation helpers |
| `<schema>/update.go` | 209,024 | ~65,000 | Same pattern + generic `AddX` / `Clear` helpers |
| `<schema>/where.go` | 155,706 | ~50,000 | Predicates collapse from 3 lines to 1 |
| `internal/<schema>_mutation.go` | ~150,000 | ~45,000 | Field accessors wrap generic helpers; struct fields stay typed |
| `<schema>_query.go` (top-level) | 123,338 | ~40,000 | Edge-loader helpers centralized; typed wrappers become 1-line |
| `<schema>_client.go` | 27,970 | ~10,000 | CRUD entrypoints delegate to generic client helpers |
| `<schema>_entql.go` | 26,337 | ~10,000 | Node predicate wiring through generic helper |
| `<schema>/delete.go` | 10,767 | ~4,000 | Already small; becomes trivial |

Approximate total: 952k → 300k. ~65–70% reduction across the targeted surface.
Secondary savings expected in `entql.go`, `indexing.go`, `fake.go`.

Out of this pass: `gql_*.go` (entgql plugin output), `schema/*` (user schema
source), runtime execution code.

## Architecture

Only two areas change.

### `runtime/entbuilder/` — additions

New files:

- `predicate.go` — generic predicate builders. Give codegen a single-call
  entrypoint per operator rather than `return predicate.X(sql.FieldEQ(...))`
  repeated per operator per schema.
- `mutation.go` — generic helpers for `SetField` / `OldField` / `ResetField`
  patterns. Take typed pointers to the mutation's state fields plus pointers
  to shared bookkeeping maps (cleared fields, added fields). Struct layout
  stays typed.
- `create_setters.go`, `update_setters.go` — helpers for the extremely
  repetitive builder-return pattern:
  `func (b *B) SetX(v T) *B { b.mutation.SetX(v); return b }`. Collapses the
  builder's thousands of 3-line methods.
- `edge.go` — generic edge operations. M2M add/clear, O2M add/clear, O2O set
  patterns share structure and can be shared; 3–4 templates cover the cases.

Extend `helpers.go` only if additional field-descriptor shapes surface during
implementation.

### `entc/gen/template/` — rewrites

Rewrite the templates for: `where`, `create`, `update`, `delete`, `mutation`,
`client`, `entql`, `query`. All named methods and functions stay. Bodies
shrink to single-line calls into `runtime/entbuilder/`.

Nothing else changes: the schema AST, codegen pipeline, runtime query
execution, dialects, and graph analysis are not touched by this refactor.

## Correctness safeguards

Highest-priority section given the correctness constraint.

### Frozen baseline for measurement

Before any template change: regenerate each `entc/integration/*` scenario and
commit the generated tree in a dedicated "baseline" commit. This is not a
correctness gate — diffs are expected after the refactor. The baseline exists
to produce a reproducible before/after comparison for LOC, compile time, and
`go vet` memory usage.

Correctness gating happens through the integration test suite and the SQL
snapshot regression tests, both described below.

### Generated witness tests per schema

Codegen emits a `<schema>_witness_test.go` per schema (build tag gated so
consumer repos can opt out of shipping them). Each witness test:

- Asserts each generated `Field*` constant names a real struct tag on the
  entity type.
- Round-trips a zero value through each mutation field: set → read → reset,
  verifying the `changedField` state transitions.
- Exercises every generated predicate on a synthetic in-memory table to
  confirm it emits non-empty SQL.

These tests catch descriptor/struct drift at `go test` time, before the code
reaches a query.

### Direct unit tests for `runtime/entbuilder`

The package currently has no tests. Prerequisite to expansion: every existing
helper gets table-driven unit tests covering each supported `field.Type`.
Every new helper added in this effort ships with its tests in the same MR as
the helper.

### Consumer-repo CI as gate

Every template-change MR runs `service-api-go`'s test suite against the
regenerated code. Red = no merge. This is the highest-signal regression net
because it exercises 111 real schemas across the feature matrix (hooks,
privacy, multischema, soft-delete, UUIDs, JSON fields, edges).

### SQL regression tests

Snapshot SQL emitted for one representative query per category
(select/filter/join/insert/update/delete/bulk). Template changes that alter
emitted SQL by even whitespace trip the snapshot; updates must be
deliberate.

## Phase plan

Steps 1 (understand current patterns) and 2 (evaluate approaches) of the
user's 6-step plan were completed during the brainstorming that produced this
spec. The phases below implement steps 3 through 6, plus a final measurement
step.

### Phase 1 — Coverage audit (1–2 days) — maps to user step 3

Produce a coverage matrix: `{file category} × {scenario}` where scenarios
are {edge types, soft-delete, multischema, hooks, privacy, JSON, custom IDs,
bulk, pagination, entql predicates}. Mark each cell as covered, partial, or
gap. Output: a table in this spec's follow-up implementation plan.

### Phase 2 — Shore up tests (3–5 days) — maps to user step 4

- Write `runtime/entbuilder/*_test.go` covering every existing helper.
  Prerequisite for Phase 3.
- Add the witness-test generator to the codegen pipeline (plumbing only; not
  emitting for any schema yet).
- Add targeted tests to fill gaps from Phase 1 — especially edge cases
  around hooks, multischema qualifier injection, and bulk operations.
- Add SQL snapshot regression infrastructure.

### Phase 3 — POC: `where.go` for one schema (1–2 days) — maps to user step 5

Target: `where.go` for one integration scenario (e.g., `edgeschema/`) and
one consumer-repo schema (`user`, 6,442 LOC of `where.go` today).

Measure:

- Source LOC before and after.
- `go build -o /dev/null ./...` wall time and peak RSS.
- `go vet ./...` wall time and peak RSS.
- Binary size of the output.

**Go/no-go gate.** If compile RSS on the consumer repo hasn't dropped
materially (target: ≥30%) or generic instantiation blows up the type graph,
pause and re-evaluate against Approach 2 or 3. Do not proceed to Phase 4.

### Phase 4 — Full refactor, category by category — maps to user step 6

Order chosen to ascend in risk, so confidence accumulates:

1. `where.go` — smallest bodies, most symmetric, predicates already one-line.
2. `delete.go` — trivially small.
3. `create.go` — setters + edge adders.
4. `update.go` — setters + `AddX` + `Clear` + edge mutations.
5. `internal/<schema>_mutation.go` — state management; most complex. Witness
   tests must be in place before starting.
6. `client.go`, `entql.go`, `query.go` — top-level entrypoints.

Each category proceeds as: runtime-helper MR → template MR → regen MR →
integration green + consumer CI green. No new category starts until the
prior is merged and stable for at least one cycle of consumer CI.

### Phase 5 — Measure and publish

Produce a results document matching the format of
`COMPACT_HELPERS_RESULTS.md`: LOC before/after, compile time before/after,
`go vet` memory before/after, integration test pass rate, consumer test pass
rate.

## Success criteria

All must hold for the refactor to be declared successful.

| Metric | Target |
|---|---|
| `service-api-go/ent/gen` total LOC | ≥ 60% reduction |
| `go vet ./...` on `service-api-go` | completes on a 32 GB machine |
| Cold `go build ./...` on `service-api-go` | ≥ 25% faster |
| `entc/integration/*` all scenarios | 100% green, tests unchanged |
| `service-api-go` test suite | 100% green, tests unchanged |
| Exported API surface of `ent/gen/` | zero diff in signatures |

If compile-time targets fail while LOC and tests pass, Approach 1 has
produced a source-level improvement but has not solved the stated pain.
Escalate to Approach 2 or 3 via a new spec; do not declare victory.

## Risks

### Generic instantiation blowup

`predEQ[User]`, `predEQ[Escrow]`, …, one generic instantiation per schema
per operator, could produce a type graph that cancels out the source-level
win. Mitigation: prefer helpers that take `any` / `driver.Value` internally
and only use type parameters at the call-site boundary; measure in Phase 3
before committing to full rollout.

### Mutation state uniformity

Mutations today carry per-field typed pointers (`name *string`, `age *int`).
Go generics don't have variadic type-parameter lists, so a single generic
`SetField` across heterogeneous fields isn't expressible directly. We'll
either use one generic helper per field type (acceptable) or codegen a small
adapter per field type (also acceptable). Neither changes the external API.

### Edge operations

M2M, O2O, and O2M edge setters have more varied shapes than scalar fields.
Likely needs 3–4 generic templates (M2M add, M2M clear, O2M add, O2O set)
rather than one universal helper. Still a big reduction but not as clean as
scalars.

### Consumer-repo drift during refactor

Bugs may land in `service-api-go` between our phases. Each phase branches
from a pinned consumer-repo SHA to keep the baseline stable, regenerating
from that SHA for each CI run.

## Out of scope

- `gql_*.go` bloat from entgql. Clarified as a follow-up.
- Gremlin dialect fidelity in the POC: only scenarios exercised by the
  target consumer are in scope for POC. Full rollout must still support the
  full integration matrix.
- Any rename or removal of generated API. If escalation to Approach 2 or 3
  requires API breakage, that triggers a new spec.

## Open questions

None required to start Phase 1. Phase-2-and-later details will be refined
in the implementation plan.
