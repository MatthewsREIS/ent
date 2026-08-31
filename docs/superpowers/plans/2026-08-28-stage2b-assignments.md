# Stage 2b: Assignments (Builder Setters → Handle Assignments) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete every per-field/per-edge setter method from the generated create/update builders (`SetX`, `SetNillableX`, `AddX`, `AppendX`, `ClearX`, `SetXID`, `SetNillableXID`, `AddXIDs`, `RemoveXIDs`, `ClearX`-edge — ~440k lines across gemini's `create.go`+`update.go`) in favor of `builder.With(escrow.F.Name.Set("x"), escrow.E.Parcels.AddIDs(id1, id2))`.

**Architecture:** Stage 2a's `F`/`E` handles gain assignment-producing methods. An `entfield.Assignment` is `func(m Mutable) error` where `Mutable` is the small interface already satisfied by every mutation (all mutations are aliases of `entbuilder.Mutation[T, I]`, which exposes `SetField/AddField/AppendField/ClearField/ResetField/SetEdgeID/AddEdgeIDs/RemoveEdgeIDs/ClearEdge` generically — verified). Builders keep only `With(assignments ...entfield.Assignment)` (a 5-line generated method) plus their existing shells. JSON fields (skipped by 2a's `F`) join `F` via a new setter-only `JSON[T]` handle; edge-fields (fields with an `EdgeName`) get an edge-backed assignment variant; `Edge` gains an ID type parameter for typed edge assignments. Migration uses a types-aware rewriter mode (chain folding needs type information syntax alone can't provide).

**Tech Stack:** Go 1.24 generics; `entbuilder.Mutation[T, I]` generic API; `golang.org/x/tools/go/packages` for the types-aware rewriter mode (NEW dependency, tools-scoped — sanctioned below).

**Spec:** `docs/superpowers/specs/2026-08-27-codegen-reduction-design.md` (stage 2, setter/assignment half — completes stage 2)

## Global Constraints

- Branch `feat/stage2b-assignments`, stacked on `feat/stage2-field-handles` (PR base = that branch). All fork work in `/var/home/smoothbrain/dev/matthews/ent/.claude/worktrees/say-less`; never run git against `/var/home/smoothbrain/dev/matthews/ent`. Never `git stash`.
- Dependency exception (controller-sanctioned): `golang.org/x/tools/go/packages` may be added for `tools/handlerewrite` only. No other new deps.
- Verified facts the design leans on: mutations are type ALIASES of `entbuilder.Mutation[T, I]` (`models/gen/internal/*_mutation.go`); its generic API covers all setter needs incl. `RemoveEdgeIDs` (M2M-only, errors on unique edges) and `SetEdgeID` (unique-only); generated setters are one-line delegations to that API, with two behavioral wrinkles to preserve: (a) numeric `SetX` on UPDATE builders calls `ResetField` first (Set overrides prior Add), (b) edge-fields (`$f.EdgeName`) route `SetX` through `SetEdgeID(edgeName, v)` and `ClearX` through `ClearEdge(edgeName)`, not `SetField`; (c) `SetNillableX` exists on create for Optional/Default fields and on update for non-UpdateDefault fields — the generic `SetNillable` subsumes both. This fork has NO entity-object edge setters (`AddParcels(...*Parcel)` etc. were already removed) — only ID forms exist.
- entgql mutation-inputs ALREADY call the generic `m.SetField(...)` API (contrib fork, verified in `models/gen/mutationinputs/*.go`) — contrib is expected to need no template changes for 2b; Task 4 verifies rather than assumes.
- Old ClearX existed only for Optional fields and AddX only for numerics/AppendX only for JSON-appendable: generic handles expose `Clear` on all handles and rely on mutation/DB-level errors for misuse (same ponytail class as 2a's IsNil-everywhere; comment it). `Add` stays Number-only, `Append` stays JSON-only (compile-enforced by handle kind).
- Restore-after-regen rule (standing): `examples/` and `entc/integration/codegen_isolation/` restored to the branch base after any `go generate ./...`.
- Pre-existing-broken on master/base (do not chase, do not worsen): gremlin, multischema/cleanuser build, customid/edgeschema TEST-pkg builds, internal/bench, one entc bootstrap test.
- Gemini work: in `/home/smoothbrain/dev/matthews/gemini/.worktrees/codegen-reduction` ONLY (never `.worktrees/main`), everything left uncommitted; contrib clone `/home/smoothbrain/dev/matthews/contrib` read-mostly (commit on its branch only if Task 4 finds real emissions). Benchmarks appended to `CODEGEN_REDUCTION_RESULTS.md` as "Stage 2b" (baseline = stage-2a quiet-machine numbers: LOC 1,577,213; gen 112.4s/3.2GB; build 63.8s/3.17GB — same worktree this time, so no environment confound; say so).
- Rewriter manifest schema extension (contract between Tasks 2 and 3 — codegen emits, rewriter consumes): each package entry gains
  `"setters": {"<StructField>": {"kind": "field|edgefield|edge", "nillable": bool, "canAdd": bool, "canAppend": bool, "unique": bool}}` — fields keyed by the SAME PascalCase names as `fields`/`edges`; `edge` entries describe E-handle edges (unique → SetID/SetNillableID/Clear; non-unique → AddIDs/RemoveIDs/Clear).

---

### Task 1: `entfield` assignments

**Files:**
- Modify: `runtime/entfield/entfield.go`, `runtime/entfield/edge.go`
- Create: `runtime/entfield/assign.go` (Mutable, Assignment, shared helpers)
- Test: `runtime/entfield/assign_test.go`

**Interfaces (Tasks 2-5 rely on these exact names):**

```go
// assign.go
type Mutable interface {
	SetField(name string, value ent.Value) error
	AddField(name string, value ent.Value) error
	AppendField(name string, value ent.Value) error
	ClearField(name string) error
	ResetField(name string) error
	SetEdgeID(edge string, id any) error
	AddEdgeIDs(edge string, ids ...any) error
	RemoveEdgeIDs(edge string, ids ...any) error
	ClearEdge(edge string) error
}
type Assignment = func(m Mutable) error
// Apply runs assignments in order, returning the first error.
func Apply(m Mutable, as ...Assignment) error
```

Handle additions (every existing field handle — String/Number/Bool/Time/Enum/Value/Bytes and their Scan variants):
- `Set(v T) Assignment` — `m.SetField(col, <same conversion the handle's EQ uses: string(v) for String/Enum, raw for others>)`. **Number.Set** first calls `m.ResetField(col)` (ignore its error — field may be unset), then SetField (preserves update-builder Set-overrides-Add; harmless no-op on create).
- `SetNillable(v *T) Assignment` — no-op assignment when nil, else Set.
- `Clear() Assignment` — `m.ClearField(col)`. `// ponytail:` exposed on all handles; non-optional misuse surfaces at save/DB.
- `Add(v T) Assignment` — Number only — `m.AddField(col, v)`.

New handle kinds:
```go
// setter-only JSON handle; joins F for JSON fields (no predicates — unchanged from 2a).
type JSON[T any] struct{ col string }
func NewJSON[T any](col string) JSON[T]
func (f JSON[T]) Set(v T) Assignment
func (f JSON[T]) SetNillable(v *T) Assignment  // pointer-to-T; template emits only for !Type.Nillable fields, same rule as others
func (f JSON[T]) Append(v T) Assignment        // m.AppendField
func (f JSON[T]) Clear() Assignment

// edge-field wrapper: predicates delegate to an embedded Value/String/... handle's
// column; assignments route through the EDGE per old setter semantics.
type EdgeField[T any] struct {
	Value[T]          // predicate/order surface on the column (matches 2a behavior)
	edge string
}
func NewEdgeField[T any](col, edge string) EdgeField[T]
func (f EdgeField[T]) Set(v T) Assignment          // m.SetEdgeID(f.edge, v)
func (f EdgeField[T]) SetNillable(v *T) Assignment
func (f EdgeField[T]) Clear() Assignment           // m.ClearEdge(f.edge)
```

`Edge` gains an ID type parameter and assignment methods (2a emission is regenerated in Task 3, and `NewEdge`/`NewEdgeSteps`/tests updated in the same commit):
```go
type Edge[TP ~func(*sql.Selector), ID any] struct{ ... } // existing fields unchanged
func (e Edge[TP, ID]) SetID(v ID) Assignment            // unique edges
func (e Edge[TP, ID]) SetNillableID(v *ID) Assignment
func (e Edge[TP, ID]) AddIDs(vs ...ID) Assignment       // non-unique
func (e Edge[TP, ID]) RemoveIDs(vs ...ID) Assignment
func (e Edge[TP, ID]) Clear() Assignment                // m.ClearEdge(name) — Edge needs its name: add `name string` to the struct + constructors (2a's constructors currently take only step+filters; extend signatures: NewEdge[TP, ID](name string, step func() *sqlgraph.Step, neighborFilters ...func(*sql.Selector)); same for NewEdgeSteps)
```
Edge ID values pass through `entbuilder.ToAny`-style boxing for AddEdgeIDs/RemoveEdgeIDs (`[]any`). Misuse (SetID on non-unique etc.) surfaces as the mutation's existing errors — `Apply` propagates them; the generated `With` records the first error on the builder (Task 3 wires it like existing builder-err patterns).

- [ ] **Step 1: Write failing tests** — use a recording fake implementing `Mutable` (log of (op, name, value) tuples) + error-injection fake:

```go
func TestFieldAssignments(t *testing.T) {
	rec := &recorder{}
	name := entfield.NewString[string]("name")
	require.NoError(t, entfield.Apply(rec, name.Set("a"), name.Clear(), name.SetNillable(nil), name.SetNillable(ptr("b"))))
	require.Equal(t, []op{{"SetField", "name", "a"}, {"ClearField", "name", nil}, {"SetField", "name", "b"}}, rec.ops)
}
func TestNumberSetResetsAdd(t *testing.T)   // Number.Set → ResetField then SetField; Add → AddField
func TestEnumSetConvertsToString(t *testing.T)
func TestJSONAppendAndSet(t *testing.T)
func TestEdgeFieldRoutesThroughEdge(t *testing.T) // Set → SetEdgeID("owner", v); Clear → ClearEdge("owner"); predicates still hit the column (render one)
func TestEdgeAssignments(t *testing.T)      // SetID/AddIDs/RemoveIDs/Clear with recorded []any boxing
func TestApplyStopsOnFirstError(t *testing.T)
func TestScanHandleSetUsesRawValue(t *testing.T) // Set on a Scan handle: decide + pin — old SetX passed the RAW Go value to mutation (scanner applies at spec-build time), so Set must NOT scan; assert raw pass-through
```

- [ ] **Step 2: verify fail** → **Step 3: implement** → **Step 4: `go test ./runtime/entfield/ -v` PASS, existing 2a tests still green (Edge signature changes ripple into edge_test.go — update those in this commit), vet clean** → **Step 5: commit** `feat(entfield): add assignment support to field and edge handles`.

---

### Task 2: `tools/handlerewrite` v2 — types-aware setter-chain mode + block-scoped shadowing

**Files:**
- Modify: `tools/handlerewrite/main.go`, `rewrite.go`; Create: `tools/handlerewrite/chains.go`, `chains_test.go`
- Modify: root `go.mod` (add `golang.org/x/tools`)

**Interfaces:** new CLI mode `-chains` (implies types-aware loading): `handlerewrite -manifest M -pkgprefix P -chains ./...` — loads packages via `go/packages` (NeedTypes|NeedSyntax|...), so receiver types are KNOWN, eliminating both the shadowing heuristic and root-detection guesswork in this mode.

**Rewrite rules (types-aware):** for any method call whose receiver type is a generated builder (`*<X>Create` or `*<X>Update`/`*<X>UpdateOne` in a manifest package, identified by type name + package path prefix) and whose method decomposes per the extended manifest `setters` table:

| Old | New |
|---|---|
| `.Set<F>(v)` (kind=field) | `.With(<pkg>.F.<F>.Set(v))` |
| `.SetNillable<F>(p)` | `.With(<pkg>.F.<F>.SetNillable(p))` |
| `.Add<F>(v)` (canAdd) | `.With(<pkg>.F.<F>.Add(v))` |
| `.Append<F>(v)` (canAppend) | `.With(<pkg>.F.<F>.Append(v))` |
| `.Clear<F>()` | `.With(<pkg>.F.<F>.Clear())` |
| `.Set<F>(v)` (kind=edgefield) | same shape — handle routes it |
| `.Set<E>ID(v)` / `.SetNillable<E>ID(p)` (edge, unique) | `.With(<pkg>.E.<E>.SetID(v))` / `...SetNillableID(p)` |
| `.Add<E>IDs(vs...)` / `.Remove<E>IDs(vs...)` (edge, non-unique) | `.With(<pkg>.E.<E>.AddIDs(vs...))` / `...RemoveIDs(vs...)` |
| `.Clear<E>()` (edge) | `.With(<pkg>.E.<E>.Clear())` |

**Chain folding:** consecutive rewritten calls in one chain merge into a single `With(a1, a2, …)`; non-setter calls (`.Where(...)`, `.Save(ctx)`, `.OnConflict(...)`) break the fold. `<pkg>` = the entity package, added to the file's imports if absent (derive path from the builder type's package + `/`+lowercase-entity — no: derive from the manifest key's actual import path, which the types info provides via the F var's package… simplest correct: record each manifest package's full import path in the manifest: extend schema with `"importPath"`; Task 3's emitter writes it).
Ambiguity rule carried from 2a: any name decomposing two ways against the setters table → refuse-and-report (types mode makes this near-impossible, keep the guard).
Blast-radius guard: a receiver that is a builder type NOT under `-pkgprefix` is untouched.

- [ ] **Step 1: failing tests** — golden-file style with a small self-contained test module (testdata module with fake generated builders implementing the method names + F/E vars, loaded via go/packages in tests): single setter; long chain folds to one With; chain interrupted by Where keeps two Withs; SetNillable; edge forms incl. unique/non-unique; import insertion; non-manifest builder untouched; shadowed-variable case now handled CORRECTLY (a local named like the package doesn't matter — types mode); ambiguity refusal.
- [ ] **Steps 2-4: fail → implement → PASS; `go vet ./tools/...`; keep v1 (syntax mode) behavior untouched and its tests green.**
- [ ] **Step 5: commit** `feat(tools/handlerewrite): types-aware setter-chain rewrite mode`.

---

### Task 3: Templates + manifest v2 + fork regen/migration

**Files:**
- Modify: `entc/gen/template/builder/setter.tmpl` (delete all per-field/per-edge output; emit `With` + keep `Mutation()`), `entc/gen/template/where.tmpl` (F gains JSON fields via `NewJSON`, edge-fields switch to `NewEdgeField`, E entries updated for `Edge[TP, ID]` + name arg), the manifest emitter in `entc/gen` (add `importPath` + `setters` per the schema contract), plus any create/update template references to deleted setters (e.g. docs comments, `SetNillableXID` emission sites).
- Regenerate `entc/integration/**` (restore rule applies); migrate fork call sites with the v2 rewriter (`-chains`); hand-fix leftovers (report classes).

**Produces:** per entity: `func (c *XCreate) With(as ...entfield.Assignment) *XCreate` (applies via `entfield.Apply(c.mutation, as...)`, recording the first error the way the builder's existing validation-error path does — find the existing err-carrying pattern on builders and reuse it so `Save` surfaces it); same on `XUpdate`/`XUpdateOne`. All per-field setter methods gone. `defaults()`/`check()`/upsert shims untouched.

- [ ] Steps: template edits → regen (background) → restore rule → build rewriter → migrate fork tests (`-chains`) → iterate builds → suites (`runtime/... entc/gen/... -count=1`; integration `TestSQLite`) → logical commits.
- ⚠ Watch: `SetNillableXID` (unique optional edges) and `SetXID` emissions live in `setter.tmpl`'s EdgesWithID block — both must map to E-handle forms; the upsert template's doc-comment examples reference `SetX` — update comments to `With(F.X.Set(v))`.

---

### Task 4: Contrib verification (expected no-op)

- [ ] Grep ALL of `entgql/template/**` + `entgql/*.go` in `/home/smoothbrain/dev/matthews/contrib` for builder-setter emissions (`MutationSet`, `MutationAdd`, `MutationClear`, `Set{{`, `.Clear{{`, `Append`) EXCLUDING mutation-input templates (already generic `m.SetField`) — mutation-input's `m.Clear/Set/Append` calls target the MUTATION's generic API, not builders; confirm none target builders.
- [ ] If genuinely zero: report the inventory as evidence; no contrib commit. If any found: apply the Task-2-style swap on the contrib branch (`feat/stage2a-field-handles`), commit, don't push.
- [ ] Validation either way happens via Task 5's gemini regen.

---

### Task 5: Gemini migration + benchmarks + results

Same protocol as 2a Task 6, in `/home/smoothbrain/dev/matthews/gemini/.worktrees/codegen-reduction` (expected dirty state = the reviewed 2a output; anything else → BLOCKED):
- [ ] BEFORE = stage-2a quiet-machine numbers (same worktree — state that the environment confound from 2a does NOT apply this round).
- [ ] Regenerate; fix gemini extension templates emitting builder setters (~11 known: enthistory history_schema, enthubspot inbound/outbound workers + tests, entmerge merge_go, entsearch reindex_helpers + resolvers, entcontext — plus sweep entfake and all others fresh; swap to `With(F.X.Set(...))` forms).
- [ ] Migrate call sites: v2 rewriter `-chains` over models/api/workers/shared non-generated Go; iterate builds; hand-fix classes recorded.
- [ ] Tests: models+workers suites; `-run OnConflict` (upsert paths touch create builders); `ent_resolvers`; one mutation-heavy integration package (e.g. `transaction` or `contact`) since 2b changes WRITE paths — record vs pre-existing.
- [ ] AFTER benchmarks (LOC, gen, clean build); append "Stage 2b" section with deviations: Clear-on-all-handles; numeric Set→Reset+Set parity note; Scan-handle Set passes raw value (pin whatever Task 1 decided); With error-deferral semantics; anything discovered.
- [ ] Commit ONLY the results doc in the fork worktree; reply with end-state `git status --porcelain` enumeration.

---

## Self-review notes

- Spec coverage: completes stage 2 (spec's "With(assignments…)" surface). Mutation typed accessors for hooks (`escrow.Name.Get(m)`) remain stage 3 — hooks/extensions calling MUTATION methods (`m.SetField` etc.) are untouched by 2b since those are already generic.
- Type consistency: `Assignment`/`Mutable`/`Apply` names used identically in Tasks 1/2/3/5; manifest v2 contract (`importPath`, `setters`) defined once in Global Constraints; `Edge[TP, ID]` signature change is contained to Task 1 + Task 3's regeneration (2a's NewEdge/NewEdgeSteps call sites are all generated or in entfield tests).
- Deliberate simplifications carried: Clear-on-all-handles (ponytail comment required); `With` defers assignment errors to Save (matches builder err conventions rather than panicking mid-chain).
