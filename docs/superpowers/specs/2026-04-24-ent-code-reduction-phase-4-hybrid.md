# ent Code Reduction — Phase 4 (Hybrid)

- Date: 2026-04-24
- Branch: `i-h8-ent` (current) — Phase 4 work may branch from here
- Status: draft, pending user approval
- Supersedes: none. Extends `docs/superpowers/specs/2026-04-23-ent-code-reduction-design.md` at the approach-selection level.

## Prior context

- **Original spec** (2026-04-23): selected Approach 1 (lean generics + 1-line shims) as primary, with Approach 2 (descriptor-driven) and Approach 3 (hybrid) held in reserve.
- **POC results** (`docs/superpowers/results/2026-04-23-poc-where-measurements.md`): applying Approach 1 to `where.go` (9.6% of generated LOC in the 111-schema consumer) produced a 1.8% source reduction, a 10.4% cold-build wall-time improvement, and a 3.8% peak-RSS improvement. Drop-in API compatibility confirmed (zero diff across 127 packages).
- **Decision** (Task 17): proceed to Phase 4 with **hybrid Approach 3** instead of pure Approach 1. Rationale:
  - Approach 1 validated the *mechanism* — generic instantiation does not blow up the type graph.
  - Per-LOC efficiency is real but the remaining categories are not uniform: `mutation.go`, `client.go`, and `entql.go` carry the highest type-graph weight (111 distinct named types × dozens of methods each). Approach 1 shortens their bodies but doesn't collapse the type graph.
  - Descriptor-driven generics (Approach 2 applied selectively) collapse 111 distinct types into one shared generic type with a tiny per-schema façade, cutting the type graph by a much larger factor.

## Goal

Reduce compile peak memory, cold-build wall time, and source LOC of the generated ent code on the 111-schema consumer so that:

- Cold `go build ./ent/gen/...` on the consumer peaks below ~5 GB RSS.
- Cold `go build ./...` (full repo, no `-N -l` flags) peaks below ~5.5 GB.
- Total generated ent LOC drops by ≥40% on the consumer.
- IDE indexing (gopls) on the consumer is responsive at desktop-scale memory (<8 GB).

All without breaking drop-in API compatibility, and with zero regression on the `entc/integration/*` suite or the consumer's test suite.

## Non-goals

- Changing schema authoring ergonomics.
- Improving runtime throughput/latency (single-digit nanosecond regressions per hot-path operation remain acceptable).
- Fixing `gql_*.go` bloat (entgql plugin) — still a separate effort.
- Upstream merge compatibility — fork continues to diverge.

## Constraint carried forward: correctness first

Restated from the prior spec. Where an approach introduces a new runtime-failure mode (descriptor/struct drift, reflection-path type mismatch, scan-path panics), the design must show that failure mode is caught at codegen time or at the first `go test` run, never at query time in production.

This constraint rules out the GORM-style "infer from struct tags at first use" model. All descriptor metadata is generated from the same AST as the struct it describes, in the same codegen pass.

## The hybrid split

Each generated file category is assigned one of two strategies.

| Category | Strategy | LOC share (consumer) | Complexity share | Rationale |
|---|---|---|---|---|
| `<entity>/where.go` | **Lean generics** (Approach 1) | ~10% | Low | Already done in Phase 3. Trivial body shape. |
| `<entity>/create.go` | **Lean generics** | ~15% | Low | Setters are 1-line delegates to the mutation. |
| `<entity>/update.go` | **Lean generics** | ~13% | Medium | Same shape as create, plus `AddX`/`ClearX`. |
| `<entity>/delete.go` | **Lean generics** | ~1% | Very low | Already tiny. |
| `internal/<entity>_mutation.go` | **Descriptor-driven** (Approach 2) | ~9% | **Highest** | 40+ methods per schema × 111 schemas; single biggest type-graph lever. |
| `<entity>_client.go` | **Descriptor-driven** | ~2% | High | 111 per-entity client types collapse to one generic. |
| `<entity>_entql.go` | **Descriptor-driven** | ~2% | High | Same shape as client — per-entity typed filters collapse to one generic. |
| `<entity>_query.go` (top-level) | **Lean generics** | ~8% | Medium | Builder pattern. Possible candidate for escalation to descriptor-driven if the lean-generics win is small. |

Out of this pass: `gql_*.go` files (entgql plugin). Still separate.

## Mechanism details

### Lean generics (recap)

Generated code keeps its exported types and signatures. Bodies collapse to 1-line calls into `runtime/entbuilder/*.go` helpers instantiated on the per-schema named type at the call site. Verified in Phase 3 for `where.go`:

```go
func NameEQ(v string) predicate.User { return entbuilder.FieldEQ[predicate.User](FieldName, v) }
```

### Descriptor-driven (new)

The generated code splits into two layers:

**Layer 1 — per-schema metadata (small, data-only):** one file per schema containing a
compile-time-constructed `SchemaDescriptor` referencing the schema's fields, edges,
indexes, hooks, and type witnesses.

```go
// generated: internal/user_descriptor.go
var userDescriptor = &entbuilder.Schema[User]{
    Name: "users",
    IDField: entbuilder.Field{Name: "id", Type: reflect.TypeOf(uuid.UUID{}), ...},
    Fields: []entbuilder.Field{
        {Name: "name", Column: "name", Type: reflect.TypeOf("")},
        {Name: "email", Column: "email", Type: reflect.TypeOf(""), Nullable: true},
        // ...
    },
    Edges: []entbuilder.Edge{ ... },
}
```

**Layer 2 — per-schema typed façade (thin):** the exported `UserMutation`,
`UserClient`, and entity filter types remain as named types. Their methods are
1-line delegates into a shared generic `Mutation[T]`, `Client[T]`, `Filter[T]`
that reads the descriptor at runtime.

```go
// generated: internal/user_mutation.go
type UserMutation = entbuilder.Mutation[User] // type alias, one line

// Typed helpers are still generated so callers can do m.Name() :
func (m *UserMutation) Name() (string, bool) {
    return entbuilder.GetField[string](m, "name")
}
```

The generic `entbuilder.Mutation[T]` in the runtime owns all of: `oldFields` map,
`changedFields` set, `clearedFields` set, `addedFields` map, field-access dispatch
through the schema descriptor, and `OldX`-style fetch via the schema descriptor's
fetcher closure.

Per-schema codegen size drops from ~1,500 lines of mutation code to ~300 lines of
typed façade + ~200 lines of descriptor. For 111 schemas: ~150k → ~55k lines on
this category alone, with the type graph collapsing from 111 distinct
heavyweight types to one generic + 111 thin aliases.

### The type alias nuance

`type UserMutation = entbuilder.Mutation[User]` (alias, with `=`) exposes the
same type identity to consumers. `type UserMutation entbuilder.Mutation[User]`
(new named type) would break drop-in compat for code that performs type
assertions. The generator must use the alias form for drop-in preservation.

If we encounter call sites that conflict with an alias (e.g., defining new
methods on the alias is not allowed in Go), fall back to `type UserMutation
*entbuilder.Mutation[User]` with a typed wrapper. This choice is deferred to
implementation.

## Correctness safeguards

Additions beyond what Phase 3 had in place.

### Witness tests per schema (now mandatory)

Codegen emits `internal/<entity>_witness_test.go` per schema (build-tag gated so
consumers can opt out of shipping them). Each witness test:

1. **Descriptor/struct agreement:** every `Field*` constant names a real struct
   field on the entity type, with the expected Go type. Caught at `go test` time.
2. **Mutation round-trip:** for each field, set → get → reset → assert cleared,
   through the generic Mutation[T].
3. **Descriptor self-check:** `entbuilder.ValidateSchema(userDescriptor, User{})`
   asserts the descriptor's field list exactly matches reflection on the
   entity's struct.
4. **Nil-safety:** calling every mutation accessor on a zero-value mutation
   must not panic; missing-field reads return `(_, false)`.

### Runtime pre-flight validation

On `ent.NewClient`, each schema's descriptor is validated against its struct
type via reflection. A mismatch produces a panic at client construction time,
with a clear error message naming the offending field. This catches the case
where a consumer regenerates against one version of the schema and builds
against a skewed version of the runtime.

### Scan-path fuzzing

The scan path — `rows.Scan` → `reflect.Value.Set` on the struct — is the
single place where runtime-only bugs can surface. Add a property-based test
that generates random field values, writes them through the mutation, reads
them through a synthetic row scan, and compares. Runs in CI.

### Consumer-CI gating continues

Every Phase 4 template-change MR runs the consumer's test suite against the
regenerated code. Red = no merge.

### SQL snapshot regression continues

The Phase 3 snapshot harness stays in place. Phase 4 adds snapshots for
mutation-heavy operations (OldX fetch in hook, bulk create, cascading
delete through edge). See the plan.

## Phase plan (high-level)

Details live in the implementation plan document. High-level ordering:

1. **Spike on mutation descriptor for ONE schema** to validate the approach
   works end-to-end against a real integration test before template work
   starts. Success criterion: `entc/integration/hooks` passes with `Card`
   manually ported to the descriptor-driven form. If the spike fails,
   escalate to pure Approach 2 (including `create`/`update`/`delete`) or
   reconsider.
2. **Build the generic runtime** (`entbuilder.Mutation[T]`, `Client[T]`, `Filter[T]`,
   `ValidateSchema`) with unit tests before any template rewrite.
3. **Build the witness-test generator** in `entc/gen`. Plumbing only; emits
   no schemas yet.
4. **Template rewrite, category by category**, emitting witness tests as
   we go. Order:
   a. `create.go` (lean generics — easy warm-up).
   b. `update.go`.
   c. `delete.go`.
   d. `mutation.go` (descriptor-driven — the big one; witness tests mandatory).
   e. `client.go` (descriptor-driven).
   f. `entql.go` (descriptor-driven).
   g. `query.go` (lean generics first; escalate if numbers disappoint).
5. **Final measurement** on the consumer, captured in a new results doc.

Each category is its own PR cluster with the same structure as Phase 3:
runtime helpers → template change → regeneration → integration green → consumer
CI green. No next category starts until the prior is stable.

## Success criteria (revised from the original spec)

All must hold.

| Metric | Target | Source |
|---|---|---|
| Consumer `ent/gen` total LOC | ≥40% reduction | measured against 1,635,044 |
| Cold `go build ./ent/gen/...` peak RSS | ≤5.5 GB | baseline 9.6 GB |
| Cold `go build ./ent/gen/...` wall time | ≥40% faster | baseline 1:33.50 |
| Cold `go build ./...` peak RSS | ≤5.5 GB | baseline 9.3 GB (Phase 3 measurement) |
| `entc/integration/*` | 100% green, tests unchanged | |
| Consumer test suite | 100% green, tests unchanged | |
| Exported API surface of `ent/gen` | zero diff (type-alias form) | baseline /tmp/api-baseline |
| Client-init-time descriptor validation | panics on drift | new safety net |
| Witness tests emitted | one per schema | new safety net |

If consumer RSS lands between 5.5 and 7 GB: partial success; evaluate whether
the remaining gap justifies a Phase 5 with deeper descriptor-driven coverage or
a different tactic.

If consumer RSS stays above 7 GB: the type-graph collapse hypothesis was
optimistic. Escalate to pure Approach 2 across all categories or reconsider the
whole direction.

## Risks (new or revised from the original spec)

### Type-alias method restriction

Go forbids declaring new methods on a type declared via `type X = Y` when `Y`
is in another package. If the per-schema façade needs to add methods that
aren't in `Mutation[T]`, aliases won't work. Mitigation: make the wrapper a
named pointer type (`type UserMutation *entbuilder.Mutation[User]` with a
helper constructor), accepting one extra level of indirection. This does not
break drop-in callers for the common method-call paths.

### Descriptor-struct drift

Described above. Mitigation: witness tests per schema + client-init-time
validation. This was always a concern with Approach 2 and now has explicit
guards.

### Scan path reflection cost

`reflect.Value.Set` per field per row is slower than the current
direct-struct-field-assignment. User said single-digit-ns regressions are
fine. Order-of-magnitude check: typical 50-field entity, ~500 ns extra per
row for reflection. In a network-bound query this is lost in noise. Benchmark
during the spike to confirm.

### Consumer codegen migration

Consumers on the `service-api-go` branch regenerate on every `go generate`.
The first regeneration under new templates produces a massive diff. Land
category changes in separate merges to keep review surfaces tractable.

### Witness-test runtime

Running witness tests per schema adds CI time. At 111 schemas × 50
round-trips each, the overall cost is small (milliseconds per test). Guard
with a build tag the consumer can opt out of if needed.

## Out of scope

- `gql_*.go` bloat (entgql). Still deferred.
- Gremlin dialect fidelity in the spike (can be included in full rollout).
- Any rename or removal of generated API beyond what the drop-in compatibility
  invariant already allows.
- Approach-2-across-all-categories, unless Phase 4 fails the gate (then
  trigger a new spec).

## Open questions

- **Spike schema choice.** Which `entc/integration/*` scenario is the best
  testbed for the descriptor-driven Mutation? `hooks` (covers OldX, ResetX,
  AddedFields, hook.HasFields — all the state machinery), or `edgeschema` (covers
  M2M/through-table edge mutations — different stress)? Recommend `hooks`
  for the spike, with a secondary check against `edgeschema` before
  committing.
- **Gate on spike success.** What's the measurable gate for "spike works"
  before we proceed? Candidates: all `hooks` tests pass on ported `Card`;
  cold-build RSS for `hooks/` scenario drops by ≥15%; emitted SQL unchanged.
  Pick during plan drafting.
