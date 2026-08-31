# Stage 2a: Field & Edge Handles (Predicates + Orders) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace every per-field/per-edge generated predicate and order function (`NameEQ`, `HasParcelsWith`, `ByStatus`, `ByParcelsCount`, …) with one-line-per-field generic handles, cutting ~219k lines of `where.go` plus ~all `ByX` order helpers from gemini's generated code.

**Architecture:** A new `runtime/entfield` package defines per-Go-kind handle types (`String[T]`, `Number[T]`, `Bool[T]`, `Time`, `Enum[T]`, `Value[T]`, `Bytes`) whose methods build predicates via the existing generic `dialect/sql.Field*` helpers and orders via `sql.OrderByField`/`sqlgraph.OrderByNeighbors*`. Codegen emits, per entity, two struct vars — `F` (field handles, incl. ID) and `E` (edge handles) — whose struct-field names are the PascalCase field/edge names; nesting inside `F`/`E` makes collisions with package identifiers impossible by construction. Old per-field funcs are deleted. A manifest-driven go/ast rewriter migrates call sites mechanically. The entgql fork (codelite7/contrib) templates swap per-field func references (`escrow.NameEQ`) for handle method values (`escrow.F.Name.EQ`) — same signatures, so `wherehelpers.AppendPtr` call shapes are unchanged.

**Tech Stack:** Go 1.24 generics; `dialect/sql` Field/Order helpers (already column-string generic); `sqlgraph` neighbor steps (per-edge `newXStep()` constructors already generated); go/ast+go/format for the rewriter.

**Spec:** `docs/superpowers/specs/2026-08-27-codegen-reduction-design.md` (Stage 2 section; this plan covers its predicate/order half — assignments/setters are stage 2b)

## Global Constraints

- Go 1.24; no new dependencies (rewriter uses stdlib go/ast, go/parser, go/format, go/token only).
- Branch: `feat/stage2-field-handles`, stacked on `worktree-say-less` (PR base = `worktree-say-less`). All fork work in the say-less worktree at `/var/home/smoothbrain/dev/matthews/ent/.claude/worktrees/say-less`. Never run git against `/var/home/smoothbrain/dev/matthews/ent` (the shared checkout).
- Key facts the design leans on (verified in-tree): `predicate.X` types are ALIASES of `func(*sql.Selector)` (`entc/gen/template/predicate.tmpl`); per-entity `OrderOption` is currently a DEFINED type (`meta.tmpl` ~line 159) and must become an alias of `entfield.Order` so handle-produced orders are assignable; predicate bodies are thin `sql.Field<Op>(column, args…)` calls; edge predicates/orders route through generated `newXStep()` constructors; setters/mutation methods are OUT of scope (stage 2b).
- `examples/` and `entc/integration/codegen_isolation/` stay at their checked-in state (dormant view-schema split bug; same ruling as stage 1). After any `go generate ./...`, restore both from git before committing.
- Per-field ops emitted today (source of truth: `entc/gen/template/where.tmpl` + `gen.Field.Ops`): comparable fields get bare equality + EQ/NEQ/In/NotIn/GT/GTE/LT/LTE; strings add Contains/HasPrefix/HasSuffix/EqualFold/ContainsFold; optional fields add IsNil/NotNil; enums get EQ/NEQ/In/NotIn only; JSON fields get NO predicate funcs (constants only) — handles must not invent ops for them.
- ValueScanner fields exist in fork integration schemas (zero in gemini): handles support them via a scanner-aware constructor; predicates wrap errors with `AddError` exactly like `predicate.XOrErr` does today.
- Deliberate simplifications (`ponytail:` comments required in code): (a) every handle exposes `IsNil/NotNil` even on non-optional fields (querying NULL on a NOT NULL column just returns no rows) — avoids a Nillable wrapper type; (b) every String handle exposes the full string op set even for fields whose schema restricted ops — DB/query semantics unchanged, compile-surface slightly wider.
- Gemini work mirrors stage 1: changes left UNCOMMITTED in `/home/smoothbrain/dev/matthews/gemini/.worktrees/main`; benchmarks recorded in `CODEGEN_REDUCTION_RESULTS.md` (new "Stage 2a" section).
- The contrib fork (codelite7/contrib) has no local clone yet: clone with `gh repo clone codelite7/contrib /home/smoothbrain/dev/matthews/contrib` (skip if the directory already exists). Its changes are committed on a branch there but NOT pushed without user say-so.

---

### Task 1: `runtime/entfield` — field handles

**Files:**
- Create: `runtime/entfield/entfield.go`
- Test: `runtime/entfield/entfield_test.go`

**Interfaces (Tasks 2-6 and templates rely on these exact names):**

```go
package entfield

type P = func(*sql.Selector)     // identical to every generated predicate.X alias
type Order = func(*sql.Selector) // identical to entity OrderOption once aliased

// constructors (column string is the entity's Field* constant):
func NewString[T ~string](col string) String[T]
func NewNumber[T Numeric](col string) Number[T]
func NewBool[T ~bool](col string) Bool[T]
func NewTime(col string) Time
func NewEnum[T ~string](col string) Enum[T]
func NewValue[T any](col string) Value[T]     // uuid, custom Valuer types, etc.
func NewBytes(col string) Bytes
// scanner-aware variants (ValueScanner fields):
func NewStringScan[T ~string](col string, scan func(T) (driver.Value, error)) String[T]
func NewValueScan[T any](col string, scan func(T) (driver.Value, error)) Value[T]

type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
	~float32 | ~float64
}
```

Method sets (all methods are on the value receiver; every handle also gets `Order(opts ...sql.OrderTermOption) Order`, `Asc() Order`, `Desc() Order`, `IsNil() P`, `NotNil() P`, and a `Column() string` accessor):

| Handle | Predicate methods |
|---|---|
| `String[T]` | `EQ(T)`, `NEQ(T)`, `In(...T)`, `NotIn(...T)`, `GT(T)`, `GTE(T)`, `LT(T)`, `LTE(T)`, `Contains(T)`, `HasPrefix(T)`, `HasSuffix(T)`, `EqualFold(T)`, `ContainsFold(T)` |
| `Number[T]` | `EQ`, `NEQ`, `In`, `NotIn`, `GT`, `GTE`, `LT`, `LTE` |
| `Bool[T]` | `EQ`, `NEQ` |
| `Time` | `EQ`, `NEQ`, `In`, `NotIn`, `GT`, `GTE`, `LT`, `LTE` (arg `time.Time`) |
| `Enum[T]` | `EQ`, `NEQ`, `In(...T)`, `NotIn(...T)` |
| `Value[T]` | `EQ`, `NEQ`, `In`, `NotIn`, `GT`, `GTE`, `LT`, `LTE` |
| `Bytes` | `EQ`, `NEQ`, `In`, `NotIn`, `GT`, `GTE`, `LT`, `LTE` (arg `[]byte`) |

Implementation notes (bodies are one-liners; write them all):
- Predicates delegate to `sql.FieldEQ(f.col, v)`, `sql.FieldNEQ`, `sql.FieldIn(f.col, anySlice(vs)...)`, `sql.FieldNotIn`, `sql.FieldGT/GTE/LT/LTE`, `sql.FieldContains/FieldHasPrefix/FieldHasSuffix/FieldEqualFold/FieldContainsFold` (string ops take `string(v)`), `sql.FieldIsNull/FieldNotNull`. Variadic ops convert with a package-local `func anySlice[T any](vs []T) []any`.
- Orders: `Order(opts...)` returns `sql.OrderByField(f.col, opts...).ToFunc()`; `Asc()` = `Order()`; `Desc()` = `Order(sql.OrderDesc())`.
- Scanner-aware handles: when `scan != nil`, each predicate converts the value first and on error returns `func(s *sql.Selector) { s.AddError(err) }` (mirror of generated `predicate.XOrErr`); variadic ops convert element-wise, first error wins.
- Enum values pass `string(v)` to the sql helpers (matches today's `$f.BasicType` conversion for custom Go types); `Value[T]` passes `v` through untouched (driver.Valuer handles it, matching today's behavior for uuid fields).
- `// ponytail:` comments for the two sanctioned simplifications (IsNil on all handles; full string ops).

- [ ] **Step 1: Write the failing tests**

Test predicates by applying them to a real Postgres-dialect selector and asserting rendered SQL (same approach as `runtime/entbuilder/upsert_test.go`):

```go
package entfield_test

import (
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entfield"
	"github.com/stretchr/testify/require"
)

func render(t *testing.T, p func(*sql.Selector)) (string, []any) {
	t.Helper()
	s := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("users"))
	p(s)
	q, args := s.Query()
	require.NoError(t, s.Err())
	return q, args
}

func renderErr(t *testing.T, p func(*sql.Selector)) error {
	t.Helper()
	s := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("users"))
	p(s)
	s.Query()
	return s.Err()
}

func TestStringOps(t *testing.T) {
	name := entfield.NewString[string]("name")
	q, args := render(t, name.EQ("a"))
	require.Contains(t, q, `"name" = $1`)
	require.Equal(t, []any{"a"}, args)
	q, _ = render(t, name.In("a", "b"))
	require.Contains(t, q, `"name" IN ($1, $2)`)
	q, _ = render(t, name.ContainsFold("x"))
	require.Contains(t, q, "name")
	q, _ = render(t, name.IsNil())
	require.Contains(t, q, `"name" IS NULL`)
}

func TestCustomStringType(t *testing.T) {
	type Status string
	status := entfield.NewEnum[Status]("status")
	q, args := render(t, status.EQ(Status("active")))
	require.Contains(t, q, `"status" = $1`)
	require.Equal(t, []any{"active"}, args)
	q, _ = render(t, status.In(Status("a"), Status("b")))
	require.Contains(t, q, "IN")
}

func TestNumberOps(t *testing.T) {
	age := entfield.NewNumber[int]("age")
	q, args := render(t, age.GT(3))
	require.Contains(t, q, `"age" > $1`)
	require.Equal(t, []any{3}, args)
}

func TestTimeAndBoolAndValue(t *testing.T) {
	at := entfield.NewTime("created_at")
	q, _ := render(t, at.LTE(time.Unix(0, 0)))
	require.Contains(t, q, `"created_at" <= $1`)
	ok := entfield.NewBool[bool]("ok")
	q, _ = render(t, ok.EQ(true))
	require.Contains(t, q, `"ok" = $1`)
	id := entfield.NewValue[[16]byte]("id")
	q, _ = render(t, id.NEQ([16]byte{}))
	require.Contains(t, q, `"id" <> $1`)
}

func TestOrders(t *testing.T) {
	name := entfield.NewString[string]("name")
	s := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("users"))
	name.Desc()(s)
	q, _ := s.Query()
	require.Contains(t, q, "ORDER BY")
	require.Contains(t, q, "DESC")
}

func TestScannerConversionAndError(t *testing.T) {
	good := entfield.NewStringScan[string]("slug", func(v string) (driver.Value, error) { return "s:" + v, nil })
	_, args := render(t, good.EQ("x"))
	require.Equal(t, []any{"s:x"}, args)
	bad := entfield.NewStringScan[string]("slug", func(v string) (driver.Value, error) { return nil, fmt.Errorf("boom") })
	require.ErrorContains(t, renderErr(t, bad.EQ("x")), "boom")
	require.ErrorContains(t, renderErr(t, bad.In("a", "b")), "boom")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./runtime/entfield/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `runtime/entfield/entfield.go`** per the interface table above. Every method body is a one-line delegation; no reflection, no maps.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./runtime/entfield/ -v` — PASS; `go vet ./runtime/entfield/` clean.

- [ ] **Step 5: Commit**

```bash
git add runtime/entfield && git commit -m "feat(entfield): add generic field handles for predicates and orders"
```

---

### Task 2: `runtime/entfield` — edge handles

**Files:**
- Modify: `runtime/entfield/entfield.go` (append) — or a sibling `edge.go` in the same package
- Test: `runtime/entfield/edge_test.go`

**Interfaces:**

```go
// Edge is the generated per-edge handle. Step returns a fresh neighbor step
// (the generated newXStep() constructors have exactly this signature).
type Edge[TP any] struct { // TP: the neighbor entity's predicate type (an alias of P, kept for doc value)
	step func() *sqlgraph.Step
}
func NewEdge[TP ~func(*sql.Selector)](step func() *sqlgraph.Step) Edge[TP]

func (e Edge[TP]) Has() P
func (e Edge[TP]) HasWith(preds ...TP) P
func (e Edge[TP]) OrderByCount(opts ...sql.OrderTermOption) Order
func (e Edge[TP]) OrderBy(term sql.OrderTerm, terms ...sql.OrderTerm) Order
```

Bodies mirror the code generated today (verbatim semantics):
- `Has`: `func(s *sql.Selector) { sqlgraph.HasNeighbors(s, e.step()) }`
- `HasWith`: `func(s *sql.Selector) { sqlgraph.HasNeighborsWith(s, e.step(), func(s *sql.Selector) { for _, p := range preds { p(s) } }) }` — copy the exact current template body from `entc/gen/template/dialect/sql/predicate.tmpl` (`dialect/sql/predicate/edge/haswith` define) including any wrapping it does; if the generated `HasParcelsWith` in `entc/integration/ent/user/where.go`-equivalent files does something extra (e.g. soft-delete extension hooks via `matchTemplate "dialect/sql/predicate/edge/has/*"`), see the ⚠ note below.
- `OrderByCount`: `func(s *sql.Selector) { sqlgraph.OrderByNeighborsCount(s, e.step(), opts...) }`
- `OrderBy`: `func(s *sql.Selector) { sqlgraph.OrderByNeighborTerms(s, e.step(), append([]sql.OrderTerm{term}, terms...)...) }`

**Extension injection (verified, must support):** gemini's soft-delete extension injects an extra neighbor predicate INSIDE the `HasNeighborsWith` closure (verified in gemini `models/gen/escrow/where.go`, `HasOwnerWith`: after applying `preds`, it appends `sql.FieldIsNull("deleted_at")(s)`); plain `Has()` gets no injection. Therefore `NewEdge` accepts optional generated neighbor filters:

```go
func NewEdge[TP ~func(*sql.Selector)](step func() *sqlgraph.Step, neighborFilters ...func(*sql.Selector)) Edge[TP]
```

`HasWith` applies `preds` then every `neighborFilter` inside the closure (exact order as today: preds first, filters after); `Has()` ignores them. Task 4's template emits the filters by preserving whatever the existing `haswith` extension hooks (`matchTemplate "dialect/sql/predicate/edge/has/*"` and the soft-delete override) inject today — the injected code moves into the generated `NewEdge(...)` call's filter argument as `func(s *sql.Selector) { <injected body> }`. Grep the fork + gemini extension templates for how the injection is wired (a `where/additional` or `haswith` override) and mirror it; cover with a Task-2 unit test (`HasWith` with one filter → rendered subquery contains both the inner predicate and the filter clause).

- [ ] **Step 1: Write failing tests** — build a step for a fake M2O edge via `sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("pets", "id"), sqlgraph.Edge(sqlgraph.O2M, false, "pets", "owner_id"))`, then render `Has()`, `HasWith(inner)`, `OrderByCount()`, `OrderBy(sql.OrderByField("name"))` through the Task-1 `render` helper and assert the SQL contains the join/EXISTS/ORDER fragments (print once, then pin distinct assertions — upstream rendering is the oracle, same escape hatch as stage 1).
- [ ] **Step 2: Verify they fail** (`undefined: entfield.NewEdge`).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: `go test ./runtime/entfield/ -v` PASS; vet clean.**
- [ ] **Step 5: Commit** — `feat(entfield): add generic edge handles`.

---

### Task 3: `tools/handlerewrite` — manifest-driven call-site rewriter

**Files:**
- Create: `tools/handlerewrite/main.go`, `tools/handlerewrite/rewrite.go`
- Test: `tools/handlerewrite/rewrite_test.go`

**Interfaces:**
- CLI: `go run ./tools/handlerewrite -manifest <manifest.json> <pkg-dir>...` — rewrites .go files in place (skips `// Code generated` files).
- Manifest (produced by codegen in Task 4): JSON `{"<entity-pkg-name>": {"fields": {"Name": true, ...}, "edges": {"Parcels": true, ...}}}` keyed by the entity package's import name (e.g. `escrow`, `user`).

**Rewrite rules** (applied to selector calls `pkg.Fn(args)` where `pkg` resolves to a manifest key by import name/alias):

| Old call | New call |
|---|---|
| `pkg.<Field><Op>(args)` for Op in EQ,NEQ,In,NotIn,GT,GTE,LT,LTE,Contains,HasPrefix,HasSuffix,EqualFold,ContainsFold,IsNil,NotNil | `pkg.F.<Field>.<Op>(args)` |
| `pkg.<Field>(v)` (bare equality; Field in manifest fields) | `pkg.F.<Field>.EQ(v)` |
| `pkg.ID(v)` / `pkg.ID<Op>(args)` | `pkg.F.ID.EQ(v)` / `pkg.F.ID.<Op>(args)` |
| `pkg.Has<Edge>()` | `pkg.E.<Edge>.Has()` |
| `pkg.Has<Edge>With(preds...)` | `pkg.E.<Edge>.HasWith(preds...)` |
| `pkg.By<Field>(opts...)` | `pkg.F.<Field>.Order(opts...)` |
| `pkg.By<Edge>Count(opts...)` | `pkg.E.<Edge>.OrderByCount(opts...)` |
| `pkg.By<Edge>(term, terms...)` (Edge in manifest edges) | `pkg.E.<Edge>.OrderBy(term, terms...)` |
| bare func-value references (no call): `pkg.<Field><Op>` (e.g. passed to `wherehelpers.AppendPtr`) | `pkg.F.<Field>.<Op>` (method value) |

Matching is longest-suffix-first (so `NameNEQ` → field `Name` + op `NEQ`, not field `NameN` + `EQ`); a selector that matches no manifest field/edge+op decomposition is left untouched. `And/Or/Not` untouched.

- [ ] **Step 1: Write failing tests** — table-driven: input source string + manifest → expected output source. Cover: each op suffix; bare equality; suffix ambiguity (field `Count` with op `EQ` vs edge `X`+`Count`: manifest membership decides — test both directions); method-value (non-call) references; aliased imports (`esc "…/gen/escrow"`); non-manifest packages untouched; generated files skipped.
- [ ] **Step 2: Verify fail.** 
- [ ] **Step 3: Implement** with `go/parser` + `astutil`-free manual AST walk (stdlib only): resolve each file's imports to manifest keys by package base name (and alias), walk `ast.SelectorExpr` nodes (both in `CallExpr.Fun` position and value position), rewrite `pkg.X` selectors to nested `pkg.F.X.Op` / `pkg.E.X.Op` selector chains, print with `go/format`. Decompose names by trying (in order): known op suffixes longest-first against manifest fields; `Has`/`By` edge prefixes against manifest edges; bare field names.
- [ ] **Step 4: Tests PASS; vet clean.**
- [ ] **Step 5: Commit** — `feat(tools): add handlerewrite call-site migrator`.

---

### Task 4: Template rewrite + manifest emission + fork regen/migration

**Files:**
- Modify: `entc/gen/template/where.tmpl` (gut per-field/per-edge/ID funcs; emit `F`/`E` vars), `entc/gen/template/meta.tmpl` (delete `ByX` helpers; `OrderOption` becomes `= entfield.Order` alias; keep `newXStep` constructors — `F`/`E` and edge handles need them), import template wiring for `entfield`
- Modify: `entc/gen/*.go` — add manifest emission (a `handle_manifest.json` written next to the generated root, listing fields/edges per entity package; find the right hook by mirroring how existing auxiliary files are emitted, e.g. the internal/ or facade assets)
- Regenerate: `entc/integration/**` (NOT examples/, NOT codegen_isolation/ — restore both after regen)
- Migrate: fork test files via the Task-3 rewriter + hand-fixes

**Produces (the generated shape Tasks 5-6 depend on):** per entity package:

```go
// in where.go, replacing ~everything:
var F = struct {
	ID     entfield.Value[uuid.UUID]         // per ID type; Number for ints
	Name   entfield.String[string]
	Status entfield.Enum[Status]
	// … one struct field per entity field, PascalCase, in schema order
}{
	ID:     entfield.NewValue[uuid.UUID](FieldID),
	Name:   entfield.NewString[string](FieldName),
	Status: entfield.NewEnum[Status](FieldStatus),
}

var E = struct {
	Parcels entfield.Edge[predicate.Parcel]
	// … one per edge
}{
	Parcels: entfield.NewEdge[predicate.Parcel](newParcelsStep),
}

// And/Or/Not stay exactly as today. predicate.X aliases stay.
// JSON fields appear in F only if a handle kind exists for them — they do
// not (no ops today), so SKIP JSON fields in F entirely.
```

Handle-kind selection per field mirrors `where.tmpl`'s current op logic: enum → `Enum[<EnumType>]`; string-ish basic/custom → `String[T]`; numeric → `Number[T]`; bool → `Bool[T]`; time.Time → `Time`; `[]byte` → `Bytes`; everything else comparable (uuid, Valuer GoTypes) → `Value[T]`; ValueScanner fields → the `New<Kind>Scan` constructor wired to `ValueScanner.<X>.Value`; JSON → skipped. For fields whose predicates convert via `$f.BasicType` today, the `~string`/`Numeric` type params already perform the same conversion.

- [ ] **Step 1:** Rewrite `where.tmpl`: keep header/imports/And/Or/Not/`where/additional` hooks; delete the ID-func block, both per-field ranges, and the per-edge range; add the `F` and `E` var emission per the shape above. Move nothing else. `HasCompositeID` entities (no single ID) simply omit `F.ID`.
- [ ] **Step 2:** `meta.tmpl`: `type OrderOption = entfield.Order`; delete the `ByX`/`ByXCount`/`ByX(terms)` range blocks; KEEP `newXStep` constructors and everything else. Wire the `entfield` import in the import template the same way `entbuilder` was wired (look at stage 1's `CreateUsesEntbuilder` flag for the pattern).
- [ ] **Step 3:** Manifest emission: after generation, write `<target>/handle_manifest.json` with fields/edges per entity package (PascalCase struct-field names as the rewriter expects). Simplest correct hook: a small addition to `entc/gen` code that iterates the graph's types — mirror where `gen.Config` writes other post-gen assets; if no clean hook exists, a standalone `{{define}}` template emitting a Go file is NOT acceptable (manifest must be JSON on disk for the rewriter) — instead add a `graph.Gen`-adjacent Go function. Keep it under ~60 lines.
- [ ] **Step 4:** `go generate ./...`; then restore `examples/` and `entc/integration/codegen_isolation/` to their checked-in state (checkout + rm newly created files, exactly as stage 1 did).
- [ ] **Step 5:** Migrate fork call sites: run the rewriter with the emitted manifests over `entc/integration` test files; iterate `go build ./...` in `entc/integration` and hand-fix what the rewriter missed (record misses — they are rewriter bugs or new patterns; fix the rewriter for mechanical classes, hand-fix one-offs). Pre-existing master failures (gremlin, multischema, customid/edgeschema test-pkg QueryX gaps) stay as-is; `codegen_isolation` must still build.
- [ ] **Step 6:** Tests: `go test ./runtime/... ./entc/gen/... -count=1`; `cd entc/integration && go test . -run TestSQLite -count=1` green.
- [ ] **Step 7:** Commit(s): template+manifest change; regen; test migration.

---

### Task 5: entgql fork (codelite7/contrib) — swap per-field func references to handle method values

**Files:**
- Clone (if absent): `gh repo clone codelite7/contrib /home/smoothbrain/dev/matthews/contrib`; branch `feat/stage2a-field-handles`.
- Modify: every entgql template that references generated per-field predicate or order funcs. Known: the where-input body template (emits `wherehelpers.AppendPtr(predicates, i.X, <entity>.XEQ)` etc. — swap final arg to `<entity>.F.X.EQ` method values; bare equality/`IsNil` forms analogous; `HasXWith` → `E.X.HasWith`), and the pagination/order template if it references `ByX`/`OrderOption` helpers (search for `By{{`, `OrderOption`, `%sEQ`, `HasPrefix` etc. across `entgql/template/**`).
- gemini's own `entgql_templates/where_input_entity.tmpl` is a thin re-export shim (verified) — no change needed there.

**Interfaces:** produced generated code must reference only: `F`/`E` handle method values, `wherehelpers.*` (unchanged), `And/Or/Not`, and `predicate.X` — nothing deleted by Task 4.

- [ ] **Step 1:** Grep contrib's `entgql/template/` for every reference pattern to per-field funcs (`EQ`, `NEQ`, `In(`, `HasPrefix`, `IsNil`, `Has%sWith`, `By%s`, `OrderOption`) and list them in your report before editing.
- [ ] **Step 2:** Apply the method-value swaps. Signatures are identical (e.g. `func(string) P`), so surrounding `AppendPtr/AppendSlice/AppendNiladic` calls are untouched.
- [ ] **Step 3:** Validation is Task 6's gemini regen (contrib has no self-contained integration harness for this fork's split mode — state this in the report if confirmed; if `entgql/internal/todo*` regen targets DO build under the fork, regenerate and build them too).
- [ ] **Step 4:** Commit on the contrib branch. Do NOT push without user approval.

---

### Task 6: Gemini regen + migration + benchmarks

**Files:** gemini working tree (UNCOMMITTED, as stage 1); `CODEGEN_REDUCTION_RESULTS.md` "Stage 2a" section committed in the ent worktree.

- [ ] **Step 1:** `git -C /home/smoothbrain/dev/matthews/gemini/.worktrees/main status --porcelain` — record. Expected pre-existing uncommitted state: stage-1 regen + go.mod replaces + 1 migrated worker file. That is fine to build on (it is the reviewed stage-1 state); anything ELSE dirty → BLOCKED.
- [ ] **Step 2:** Capture BEFORE numbers (same commands as stage 1: LOC via `find gen -name '*.go' | xargs cat | wc -l`, `task api:generate-go` profile output, clean-build profile via `go clean -cache && /usr/bin/time -v go build ./...` in models). "Before" = current stage-1 state.
- [ ] **Step 3:** Point gemini at the stage-2a code: `models/api/workers` go.mod `replace entgo.io/ent => /var/home/smoothbrain/dev/matthews/ent/.claude/worktrees/say-less` (local, temporary — noted for user) AND `replace entgo.io/contrib => /home/smoothbrain/dev/matthews/contrib` (adjust if contrib fork lives elsewhere; run `go mod download` not `go mod tidy` — tidy wanders into private-module resolution). Regenerate.
- [ ] **Step 4:** Migrate gemini: run the rewriter (manifests from `models/gen/handle_manifest.json`) over all non-generated Go in models/api/workers + `models/schema/`; then iterate builds. Expect extension templates inside gemini (`models/extensions/**`: entfake, enthistory, entsearch, entsoftdelete, entmerge…) to emit now-deleted funcs — fix those templates the same way as contrib's (method-value swaps), regenerate, repeat. This is the widest, least predictable part: record every template touched and every hand-fix class.
- [ ] **Step 5:** Tests: models/workers unit suites; `task api:test-integration -- -count=1` for at least the OnConflict + a where-input-heavy package (resolvers integration); record pass/fail vs pre-existing.
- [ ] **Step 6:** AFTER numbers; append "Stage 2a" section to `CODEGEN_REDUCTION_RESULTS.md` (before/after table, deltas, deviations: IsNil-everywhere, full-string-ops, OrderOption now an alias, JSON fields lost their (nonexistent) predicate surface — n/a, plus anything discovered); commit that file only, in the ent worktree.
- [ ] **Step 7:** Reply with the short status contract; leave gemini + contrib uncommitted/unpushed for user review.

---

## Self-review notes

- Spec coverage: stage-2 predicate/order half fully covered; assignments/setters explicitly deferred to a stage-2b plan; spec's "deterministic suffix on collision" rule superseded by the strictly better `F`/`E` nesting (no collisions possible) — deviation to note in results doc.
- Type consistency: `entfield.P` = `func(*sql.Selector)` matches the alias form of `predicate.X` (verified in `predicate.tmpl`); `OrderOption = entfield.Order` alias makes handle orders assignable everywhere `OrderOption` appears (query `Order(...)` signatures unchanged).
- The rewriter is manifest-driven precisely because bare-equality funcs (`escrow.Name(v)`) are indistinguishable from arbitrary functions without knowing the field list.
- Known risk carried forward: extension hook `dialect/sql/predicate/edge/has/*` (Task 2 ⚠) — if gemini's soft-delete injects into edge predicates, the edge-handle emission must wrap the step closure; Task 2 investigates before Task 4 locks the template shape.
