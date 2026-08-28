# Codegen Footprint Reduction: Field Handles + Reflection Runtime

**Date**: 2026-08-27
**Status**: Approved design
**Repos**: this ent fork (MatthewsREIS/ent), the entgql fork (codelite7/contrib), gemini (`models/`) as consumer/benchmark.

## Problem

Gemini's 306-entity schema generates **2,035,883 lines** of Go across ~4,700 files.
Compile time, compiler/gopls memory, generation time, and CI cost all scale with
that volume. The prior compact-helpers pass (`runtime/entbuilder`, −99k lines)
slimmed builder internals; the remaining bulk is the per-field **public API
surface** — thousands of near-identical typed methods.

Measured breakdown (gemini `models/gen`):

| Category | LOC | Dominant content |
|---|---|---|
| create.go | 356k | ~75% upsert: 3 types × Set/Update/Clear/Add per field |
| update.go | 299k | 2 builders × per-field setters + edge ops |
| where.go | 219k | 10–15 predicate funcs per field |
| shared.go | 158k | per-entity copies of near-generic plumbing |
| internal/ | 154k | isolation copies of model+mutation |
| model.go | 118k | structs + scanValues/assignValues/String |
| whereinputs/ | 118k | entgql WhereInput structs + Filter/P bodies |
| query.go | 116k | per-entity Query builder, per-edge loaders |
| gql_pagination | 94k | Connection types + per-field orderings |

## Constraints (user decisions)

- **API breakage: anything goes.** Gemini is the only consumer; call sites are
  rewritten mechanically per stage.
- **Runtime perf: reflection acceptable** (~2× row-scan cost; dwarfed by DB latency).
- **Optimize**: build time, gopls memory, generation time, CI — all roughly equally.
- **No go/no-go gate between stages**; benchmarks are recorded for steering only.
- App-side custom extensions (fake, history, facades, search, indexing, ~50k LOC
  total) are **out of scope**.

## Core architecture

Go forbids type parameters on methods, so `builder.Set[T](f, v)` is impossible.
Instead **field handles produce typed values; builders consume them as varargs**:

```go
// generated, one line per field, in the existing {entity} package:
var (
    Name    = entfield.String[predicate.Escrow](FieldName)
    Amount  = entfield.Number[predicate.Escrow, float64](FieldAmount)
    Parcels = entfield.Edge[predicate.Escrow, predicate.Parcel]("parcels")
)

// call sites — compile-time type-checked:
client.Escrow.Create().With(escrow.Name.Set("x"), escrow.Amount.Set(9.5)).Save(ctx)
client.Escrow.Update().Where(escrow.Name.EQ("x")).With(escrow.Amount.Add(1)).Save(ctx)
q.Order(escrow.Name.Asc())
```

- `runtime/entfield` (new, this fork): handle kinds written once — `String`,
  `Number[N]`, `Bool`, `Time`, `Enum[T]`, `UUID`, `Bytes`, `JSON[T]`, `Other`,
  each exposing only the ops valid for that kind; `.Nillable()` adds
  `IsNil/NotNil/SetNillable`. Edge handles: `Has`, `HasWith`, `Add/Remove/Clear/Set`
  assignments, `CountAsc/CountDesc` orders.
- Handles emit `Predicate` (the entity's existing `predicate.X` type — kept),
  `Assignment[E]`, `Order[E]`. Generic builders accept them as varargs.
- A per-entity **descriptor** (one generated table: fields → types, nillable,
  defaults, validators, immutable; edges → table/columns/M2M) drives all
  runtime behavior: scan/assign, validation at Save, upsert defaults, edge loading.
- Old per-field funcs are **deleted, not deprecated**. Each stage ships a
  mechanical migration (sed/gopatch or a go/ast rewriter in `tools/`).
- Identifier collisions (field handle vs existing package identifier like
  `Table`) get a deterministic `Field` suffix, flagged in gen output.

### Why generics don't re-add the compile cost

Go stencils generics by GC shape; all entity pointers share one shape, so
`entfield.String[*A]` and `String[*B]` compile to a single dictionary-passing
instantiation. Type params are entity pointers plus a fixed handful of scalar
kinds — the runtime compiles ~once, not 306×. The entbuilder experiment
(heavy generics added, build time flat) is evidence for this.

## Stages

Each stage: runtime pkg + template change in the fork(s), fork integration
tests adapted, migration script, gemini regenerated/rewritten/built/tested,
benchmarks recorded.

### Stage 1 — generic upsert (~−270k)

Replace per-entity `XUpsert`/`XUpsertOne`/`XUpsertBulk` with
`entbuilder.Upsert[E]`/`UpsertOne[E]`/`UpsertBulk[E]`:

```go
func (u *Upsert[E]) Set(field string, v any) *Upsert[E]   // also Add, Clear
func (u *Upsert[E]) Update(fields ...string) *Upsert[E]   // take excluded value
// UpdateNewValues / Ignore / DoNothing / Exec / ID — descriptor-driven
// (immutable columns skipped, ID presence for edge schemas)
```

Generated per entity: only `OnConflict`/`OnConflictColumns` shims on
`XCreate`/`XCreateBulk` returning the generic types. Stage-1 setters are
untyped (runtime-validated against the descriptor); stage 2 adds a typed
`With(assignments…)` path, untyped `Set` remains the low-level API.
Behavior parity with ent's upsert semantics per dialect; existing upsert
integration tests ported and passing. Gemini migration: 30 files.

### Stage 2 — field & edge handles (~−550-600k, +~15k handle vars)

- Delete `where.go` entirely; delete all `SetX/SetNillableX/AddX/ClearX` and
  edge mutators from create/update; delete `ByX` order funcs.
- `update.go` shrinks to the thin builder shell. Per-field validation moves to
  descriptor-driven checks at `Save`.
- Predicate renames are regex-able (`NameEQ(v)` → `Name.EQ(v)`); setter chains
  change shape (`.SetA(a).SetB(b)` → `.With(A.Set(a), B.Set(b))`) — a go/ast
  rewriter in `tools/` handles gemini + schema files mechanically. Widest
  call-site blast radius of any stage.

### Stage 3 — reflection scan, generic mutation, plumbing dedupe (~−350-400k)

- Structs and typed `Edges` structs stay; `scanValues`/`assignValues`/`String`
  bodies deleted. Runtime scanner builds a column→struct-field-index table once
  per entity at init; per-row work is indexed `reflect` sets.
- Per-entity mutations become map-backed `entbuilder.Mutation[E]` + a ~20-line
  generated alias/constructor. Typed access in hooks via handles:
  `escrow.Name.Get(m)`, `.SetM(m, v)`, `.Old(ctx, m)`. All `SetX/OldX/ClearX/
  ResetX` mutation methods deleted; hooks/extensions migrated by the rewriter.
- Per-edge `loadX` funcs replaced by one descriptor-driven generic loader.
- `shared.go` / `internal/` / `edges/` (113k) per-entity plumbing and constants
  dedupe into the runtime package and descriptors; `delete.go`/`client.go`
  shells shrink onto the generic builders.

### Stage 4 — entgql fork (~−130-150k)

- WhereInput structs stay (gqlgen unmarshals into them; they define the GraphQL
  schema). `Filter`/`P()` bodies replaced by one reflection walker mapping
  non-nil op-fields (naming convention + struct tags) to handle predicates.
  Gemini's `where_input_entity.tmpl` override rebased onto this.
- Per-entity `Paginate` bodies → generic `Paginate[T]` (cursor codec, ordering,
  windowing) driven by descriptor + order handles; per entity remains a small
  OrderField→handle table.
- Mutation inputs (`CreateXInput.Mutate`) and node_implementors: same
  reflection-walker treatment.

## Expected impact (end state ≈ 2.0M → ~450-650k LOC, −70-80%)

Stage estimates sum to ~1.3–1.5M removed. The floor is set by what must stay
typed: model/Edges structs, WhereInput structs, per-field handle vars,
descriptors, and out-of-scope app extensions. Deeper cuts than estimated are
likely in shared/internal/edges dedupe; per-stage benchmarks recalibrate.

- `go build` models/gen: −60–80% time and peak memory (per-function compiler
  work removed; generic instantiation shared via GC-shape stenciling).
- gopls: −80%+ memory on gen packages; per-entity packages drop ~30k → 2–4k lines.
- `go generate`: several-× faster (output printing/formatting dominates; −85% output).
- Runtime: ~2× row-scan cost via reflection; validation moves from compile-time
  setters to Save-time descriptor checks.

These are scaling arguments, not measurements; per-stage benchmarks recalibrate.

## Testing & rollout

- TDD at the runtime-pkg level: unit tests for entfield/entbuilder/scanner
  written before templates are wired to them.
- Fork integration suite (`entc/integration`) adapted per stage and passing.
- End-to-end per stage: bump fork pseudo-version in gemini, `go generate`,
  run rewriter, `go build`, run gemini's test suite.
- Benchmarks per stage in `CODEGEN_REDUCTION_RESULTS.md`: gen wall time + peak
  RSS, clean build time + RSS, gen LOC, gopls memory on a fixed workload.
- One branch per stage per fork, PR'd sequentially; migration scripts committed
  in `tools/` with the stage that needs them.
