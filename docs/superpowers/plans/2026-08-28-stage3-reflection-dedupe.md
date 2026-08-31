# Stage 3: Reflection Scan + Plumbing Dedupe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the per-entity sqlSave field/edge unrolls, the per-entity
`schemaGraph` literals, the `ScanValues`/`AssignValues`/`String` bodies, and
the per-entity delete builders with descriptor-driven runtime generics —
expected −350-450k generated LOC in gemini (1,395k → ~950k-1.05M).

**Architecture:** The `entbuilder.Descriptor` (already emitted per entity in
`internal/<entity>_mutation.go`) is extended with the SQL-layer statics that
today get unrolled three separate times per entity: field column names +
`field.Type` enums, edge `sqlgraph.EdgeSpec` data (Rel/Table/Columns/Bidi/
Inverse/target-ID), and struct-index/scan metadata. One set of runtime
functions consumes it: `ApplyUpdateSpec`/`ApplyCreateSpec` (guts both
sqlSave bodies), `BuildSchemaGraph` (one shared graph replaces 306 per-entity
literals with duplicated neighbor stubs), and `ScanTargets`/`AssignRow`/
`FormatEntity` (guts the model-file bodies). Generated code keeps only thin
shells, typed structs, and the descriptor literal itself.

**Tech Stack:** Go generics + reflect; `entgo.io/ent/dialect/sql/sqlgraph`;
`entgo.io/ent/schema/field`; text/template codegen in `entc/gen/template/**`.

**Spec:** `docs/superpowers/specs/2026-08-27-codegen-reduction-design.md`
(Stage 3 section, lines 117-129 — binding authority). Spec items already
done before this plan (do not redo): per-entity mutations are already
map-backed `entbuilder.Mutation[T, I]` aliases; per-edge `loadX` funcs are
already gone (root-facade eager loaders); hooks already use
`m.SetField`/`m.OldField`. Spec item deliberately deferred: typed
hook-accessor sugar (`escrow.Name.Get(m)`) — gemini's hooks compile and pass
on the generic API today, so this is YAGNI until a caller hurts (record in
results doc as a deviation).

## Global Constraints

- Branch: `feat/stage3-reflection-dedupe`, stacked on `feat/stage2b-assignments`
  (base `3828f9803`). PR base = that branch.
- Restore rule (hard): if regen touches `examples/` or
  `entc/integration/codegen_isolation/`, restore with
  `git checkout -- examples/ entc/integration/codegen_isolation/`.
- Pre-existing-broken, do NOT chase: `entgo.io/ent/internal/bench`,
  `entc/integration/gremlin/ent/internal`, `entc/integration/multischema/ent/cleanuser`,
  `entc/integration/customid` (+`intsid`), `entc/integration/edgeschema`
  (test pkg; the generated tree builds), gremlin dialect generally.
- Multischema parity is a hard requirement (2a C1 lesson): every generated
  `edge.Schema = Config.SchemaConfig().<Key>` / `_spec.Node.Schema = ...`
  assignment in today's sqlSave must have a runtime equivalent, proven by
  `go test ./entc/integration/multischema/...` (the `ent` tree, not the
  broken `cleanuser` one) and by grep-diff of emitted SQL in its tests.
- Fork test gate per task: `go test ./runtime/... ./entc/gen/...` and, from
  `entc/integration`, `go test -run TestSQLite ./...` (SQLite only).
- Gemini work only in `/home/smoothbrain/dev/matthews/gemini/.worktrees/codegen-reduction`
  (never commit there); contrib clone read-only.
- Benchmarks (Task 7 only, same commands as stage 2b):
  `find gen -name '*.go' -print0 | xargs -0 cat | wc -l`;
  `MREIS_CODEGEN_ALLOW_WATCHER=1 task generate-go`;
  `go clean -cache && /usr/bin/time -v go build ./...` in `models/`.
  Baseline: LOC 1,395,043; gen 126.0s/3.2GB; build 72.9s/~3.0GB (quiet
  third-pass numbers from the 2b results section). Wait for load avg < 2
  before timing runs.
- Byte parity of generated output is NOT expected this stage (bodies are
  replaced wholesale); parity is semantic, proven by the integration suite.

---

### Task 1: SQL descriptor extension + generic update/create spec appliers

**Files:**
- Modify: `runtime/entbuilder/mutation.go` (FieldSpec, EdgeSpec, Descriptor)
- Create: `runtime/entbuilder/sqlspec.go`
- Test: `runtime/entbuilder/sqlspec_test.go`

**Interfaces:**
- Consumes: existing `Mutation[T, I]` internals (`m.fields`, `m.added`,
  `m.appended`, `m.cleared`, `m.edgeIDs`, `m.removedEdgeIDs`,
  `m.clearedEdges`, `m.desc`, `m.predicates`) — all package-local.
- Produces (Tasks 3 and 5 call these; Task 2 consumes the same spec fields):

```go
// FieldSpec additions (zero values = "not emitted", safe for old descriptors)
Column  string     // SQL column name
SQLType field.Type // schema/field type enum for _spec.SetField/AddField/ClearField

// EdgeSpec additions
Rel             sqlgraph.Rel // O2O/O2M/M2O/M2M
StorageTable    string       // sqlgraph table ("Table" name is taken by nothing; use StorageTable to avoid confusion with Descriptor.Table)
StorageColumns  []string     // 1 col for FK edges, 2 for M2M primary key
Bidi            bool
StorageInverse  bool         // sqlgraph Inverse flag (may differ from the graph-level Inverse already present; verify against update.tmpl emission and collapse to one field if identical — record the finding)
TargetIDColumn  string
TargetIDSQLType field.Type
SchemaKey       string       // multischema SchemaConfig struct-field name; "" = single-schema

// Descriptor additions
Table        string
TableColumns []string  // full column list including id
IDColumn     string
IDSQLType    field.Type
SchemaKey    string

// sqlspec.go
func ApplyUpdateSpec[T, I any](m *Mutation[T, I], spec *sqlgraph.UpdateSpec, schemaOf func(key string) string)
func ApplyCreateSpec[T, I any](m *Mutation[T, I], spec *sqlgraph.CreateSpec, schemaOf func(key string) string)
```

`schemaOf` is nil for single-schema apps; when non-nil it resolves an edge's
`SchemaKey` to the runtime schema name and the applier sets `edge.Schema`
(and `spec.Node.Schema` from `desc.SchemaKey`).

- [ ] **Step 1: Read the current unrolled bodies to fix the exact behavior being replaced.** Read `entc/integration/ent/user/update.go:220-460` (both sqlSave funcs), `entc/integration/ent/user/create.go` (createSpec/sqlSave), and `entc/integration/multischema/ent/user/update.go:190-330` (Schema assignments). List every distinct statement shape (SetField/AddField/ClearField; Edges.Clear/Add/Remove append with `!EdgeCleared` guard on Remove; create's `_spec.SetField` + edge Add). The appliers must reproduce each exactly, including the Remove-suppressed-when-Cleared guard.

- [ ] **Step 2: Write the failing unit test** (`runtime/entbuilder/sqlspec_test.go`). Build a hand-rolled descriptor with two fields (one numeric, one nillable string) and two edges (one O2O FK, one M2M), drive a mutation through SetField/AddField/ClearField/SetEdgeID/RemoveEdgeIDs/ClearEdge, call `ApplyUpdateSpec`, and assert on the resulting `sqlgraph.UpdateSpec`:

```go
func TestApplyUpdateSpec(t *testing.T) {
	desc := &Descriptor{
		Name: "Thing", IDType: reflect.TypeFor[int](),
		Table: "things", IDColumn: "id", IDSQLType: field.TypeInt,
		Fields: map[string]FieldSpec{
			"count": {Type: reflect.TypeFor[int](), GoName: "Count", Numeric: true, Column: "count", SQLType: field.TypeInt},
			"note":  {Type: reflect.TypeFor[string](), GoName: "Note", Nillable: true, Column: "note_col", SQLType: field.TypeString},
		},
		Edges: map[string]EdgeSpec{
			"owner": {Cardinality: O2OUnique, Target: "User", TargetIDType: reflect.TypeFor[int](),
				Rel: sqlgraph.M2O, StorageTable: "things", StorageColumns: []string{"thing_owner"},
				TargetIDColumn: "id", TargetIDSQLType: field.TypeInt},
			"tags": {Cardinality: M2M, Target: "Tag", TargetIDType: reflect.TypeFor[int](),
				Rel: sqlgraph.M2M, StorageTable: "thing_tags", StorageColumns: []string{"thing_id", "tag_id"},
				TargetIDColumn: "id", TargetIDSQLType: field.TypeInt},
		},
	}
	m := NewMutation[thingEnt, int](nil, ent.OpUpdateOne, desc)
	require.NoError(t, m.SetField("count", 3))
	require.NoError(t, m.AddField("count", 2))
	require.NoError(t, m.ClearField("note"))
	require.NoError(t, m.SetEdgeID("owner", 7))
	require.NoError(t, m.RemoveEdgeIDs("tags", 1, 2))
	spec := sqlgraph.NewUpdateSpec("things", []string{"id", "count", "note_col"},
		sqlgraph.NewFieldSpec("id", field.TypeInt))
	ApplyUpdateSpec(m, spec, nil)
	// fields
	require.Len(t, spec.Fields.Set, 1)    // count=3
	require.Len(t, spec.Fields.Add, 1)    // count+=2
	require.Len(t, spec.Fields.Clear, 1)  // note_col cleared
	require.Equal(t, "count", spec.Fields.Set[0].Column)
	// edges: owner Set => Clear+Add pair (matches generated shape), tags Remove
	require.Len(t, spec.Edges.Add, 1)
	require.Len(t, spec.Edges.Clear, 2) // owner clear-before-set + nothing else? adjust to the shape found in Step 1
}
```
(The exact Set-edge shape — whether generated code emits a Clear before Add
for a replaced unique edge — comes from Step 1; adjust assertions to match
the generated behavior verbatim, and comment each assertion with the
update.go line it mirrors. `spec.Fields`/`spec.Edges` shown here assume
sqlgraph exposes them; if the UpdateSpec fields differ, assert through
whatever sqlgraph really exposes — check `dialect/sql/sqlgraph/graph.go`.)

- [ ] **Step 3: Run to verify it fails** — `go test ./runtime/entbuilder/ -run TestApplyUpdateSpec` → compile error (fields/funcs undefined).

- [ ] **Step 4: Implement.** Add the spec fields (doc comments modeled on the existing ones), then `sqlspec.go`. Skeleton:

```go
func ApplyUpdateSpec[T, I any](m *Mutation[T, I], spec *sqlgraph.UpdateSpec, schemaOf func(string) string) {
	desc := m.desc
	if schemaOf != nil && desc.SchemaKey != "" {
		spec.Node.Schema = schemaOf(desc.SchemaKey)
	}
	if ps := m.MutationPredicates(); len(ps) > 0 {
		spec.Predicate = func(s *sql.Selector) { for i := range ps { ps[i](s) } }
	}
	for name, f := range desc.Fields {
		if v, ok := m.fields[name]; ok {
			spec.SetField(f.Column, f.SQLType, v)
		}
		if v, ok := m.added[name]; ok {
			spec.AddField(f.Column, f.SQLType, v)
		}
		if _, ok := m.cleared[name]; ok {
			spec.ClearField(f.Column, f.SQLType)
		}
	}
	for name, e := range desc.Edges {
		edgeSpec := func() *sqlgraph.EdgeSpec { /* build from e; set Schema via schemaOf(e.SchemaKey) */ }
		if m.EdgeCleared(name) { spec.Edges.Clear = append(spec.Edges.Clear, edgeSpec()...) }
		if ids := m.RemovedEdgeIDs(name); len(ids) > 0 && !m.EdgeCleared(name) { /* Remove */ }
		if ids := m.EdgeIDs(name); len(ids) > 0 { /* Add */ }
	}
}
```
Map iteration order is nondeterministic — acceptable because each field/edge
writes independent spec entries (verify no generated ordering dependency in
Step 1; if UpdateSpec preserves ordering semantics anywhere, sort names
first and say so in a comment). `ApplyCreateSpec` is the same loop against
`sqlgraph.CreateSpec` (fields append; edges only Add). Appended-JSON fields:
check how the generated sqlSave handles `m.appended` (there is a
`_spec.AddField`-vs-append distinction for JSON `Append`) and reproduce it.

- [ ] **Step 5: Run tests until green**, including the whole package: `go test ./runtime/entbuilder/`.

- [ ] **Step 6: Commit** — `feat(entbuilder): SQL spec fields on Descriptor + generic update/create appliers`.

---

### Task 2: One shared schema graph per generated package tree

**Files:**
- Create: `runtime/entbuilder/schemagraph.go`
- Test: `runtime/entbuilder/schemagraph_test.go`
- Modify: `entc/gen/template/internal_mutation.tmpl` (or the internal-package template that owns package-wide vars — locate the one that emits into `internal/`), `entc/gen/template/shared.tmpl`

**Interfaces:**
- Consumes: Task 1's spec fields on `Descriptor`/`FieldSpec`/`EdgeSpec`.
- Produces:

```go
// runtime/entbuilder/schemagraph.go
// BuildSchemaGraph assembles one sqlgraph.Schema from every entity's
// descriptor. Node order = descs order; edges added via MustAddE using
// Descriptor.Name / EdgeSpec.Target for from/to.
func BuildSchemaGraph(descs []*Descriptor) *sqlgraph.Schema
```

Generated (in `internal/`): `var SchemaGraph = entbuilder.BuildSchemaGraph([]*entbuilder.Descriptor{userDesc, cardDesc, ...})` plus `func NodeIndex(typ string) int`. Per-entity `shared.go`: `var schemaGraph = internal.SchemaGraph` replacing the whole literal (delete the stub-node machinery).

- [ ] **Step 1: Find how entql consumes schemaGraph.** `grep -rn "schemaGraph" entc/gen/template/ entc/integration/ent/user/entql.go`. Record whether node access is positional (`Nodes[0]`) — if so the entql template must switch to `schemaGraph.Nodes[internal.NodeIndex("User")]` (or an emitted per-package `nodeIndex` const). Write the finding into the task report.
- [ ] **Step 2: Failing unit test**: build 2 descriptors (with an edge between them), `BuildSchemaGraph`, assert node tables/columns/field specs and that `MustAddE` registered the edge (query via the schema's node lookup — mirror what `entql`/`sqlgraph.Neighbors` needs; the test should at minimum exercise `graph.EvalP` or whatever entql's Filter path calls, found in Step 1).
- [ ] **Step 3: Implement `BuildSchemaGraph`** — a direct transcription of today's per-entity literal builder, minus stubs (all nodes are real now). Green.
- [ ] **Step 4: Template changes.** Emit the descriptor list + `SchemaGraph` + `NodeIndex` in the internal package; shrink `shared.tmpl`'s schemaGraph block to the alias; adjust the entql template per Step 1. Regenerate (`go generate ./...` from `entc/integration`), apply the restore rule, run the fork gate suites. The entql integration tests (`entc/integration/ent` has `entql_test.go`-adjacent coverage — find via `grep -rln "Filter()" entc/integration --include='*_test.go'`) must pass.
- [ ] **Step 5: Commit** — template change + regen as two commits (`feat(entc/gen): shared schema graph via BuildSchemaGraph`, `chore(entc/integration): regenerate`).

---

### Task 3: sqlSave bodies via appliers; delete update-builder edge mutators

**Files:**
- Modify: `entc/gen/template/dialect/sql/update.tmpl`, `entc/gen/template/dialect/sql/create.tmpl`, `entc/gen/template/builder/update.tmpl`, `entc/gen/template/internal_mutation.tmpl` (descriptor literal gains the Task-1 fields), plus wherever `SchemaOf` must be generated for multischema (internal package template).
- Test: existing integration suite; new multischema regression assertions only if a gap is found.

**Interfaces:**
- Consumes: `ApplyUpdateSpec`/`ApplyCreateSpec` (Task 1), descriptor emission.
- Produces: sqlSave bodies of the shape:

```go
func (_u *UserUpdate) sqlSave(ctx context.Context) (_node int, err error) {
	if err := _u.check(); err != nil {
		return _node, err
	}
	_spec := sqlgraph.NewUpdateSpec(Table, Columns, sqlgraph.NewFieldSpec(FieldID, field.TypeInt))
	entbuilder.ApplyUpdateSpec(_u.mutation, _spec, nil) // multischema: internal.SchemaOf(_u.Config)
	if _u.modifiers != nil { _spec.Modifiers = _u.modifiers }
	if _node, err = sqlgraph.UpdateNodes(ctx, _u.Drv, _spec); err != nil { /* unchanged error translation */ }
	...
}
```
and a multischema-only generated helper:

```go
// internal/config.go (generated, multischema feature only)
func SchemaOf(c Config) func(string) string {
	sc := c.SchemaConfig()
	rv := reflect.ValueOf(sc)
	return func(key string) string { return rv.FieldByName(key).String() }
}
```

- [ ] **Step 1: Emit the new descriptor fields** in `internal_mutation.tmpl` (Column/SQLType per field; Rel/StorageTable/StorageColumns/Bidi/TargetIDColumn/TargetIDSQLType/SchemaKey per edge; Table/TableColumns/IDColumn/IDSQLType/SchemaKey on the descriptor). The template has access to the same `$e`/`$f` data update.tmpl uses today — copy the exact expressions from `dialect/sql/update.tmpl`'s current unroll so the values are identical. SchemaKey comes from the same pipeline that emits `Config.SchemaConfig().<Key>` today (find it: `grep -n "SchemaConfig" entc/gen/template/dialect/sql/*.tmpl`).
- [ ] **Step 2: Gut `dialect/sql/update.tmpl`'s per-field/per-edge ranges** down to the applier call (keep: spec construction, UpdateOne's returning-row Select/fields handling and node scan, error translation, affected-rows). Same for `create.tmpl`'s createSpec (keep: id handling, conflict/upsert wiring, bulk loop — which after 2b's C2 fix checks `builder.err`).
- [ ] **Step 3: Delete `Clear<E>`/`Remove<E>IDs` from `builder/update.tmpl`** (the mutable-edge block at ~lines 219-238). The rewriter's `-chains` decomposition already emits readings for these (final-review M4 confirmed both forms route to `ClearEdge`/`RemoveEdgeIDs`); verify `tools/handlerewrite/testdata/chainsmod` has a fixture exercising a `Clear<E>()`/`Remove<E>IDs(...)` chain link — add one if missing (copy the shape of the existing deleted-setter fixture).
- [ ] **Step 4: Regenerate, restore rule, migrate the fork's own test call sites** of the deleted edge mutators: build the rewriter (`go build -o /tmp/hr ./tools/handlerewrite`) and run `-chains` over `entc/integration/...` non-generated files as 2b did; hand-fix the known miss classes (conditional reassignment, shadows) — expect far fewer sites than 2b (only Clear/Remove edge calls).
- [ ] **Step 5: Full fork gate** + `go test ./entc/integration/multischema/...` (broken `cleanuser` excluded) — the multischema tests are the schemaconfig proof.
- [ ] **Step 6: Commit** in three commits: descriptor emission, template gutting + rewriter fixture, regen + call-site migration.

---

### Task 4: Reflection scanner — ScanValues/AssignValues/String via descriptor

**Files:**
- Modify: `runtime/entbuilder/mutation.go` (FieldSpec scan additions), the model template (find it: `grep -rln "AssignValues" entc/gen/template/` — the file emitting `internal/<entity>_model.go`)
- Create: `runtime/entbuilder/scan.go`
- Test: `runtime/entbuilder/scan_test.go`

**Interfaces:**
- Consumes: Task-1/3 descriptor fields (Column, SQLType, Type reflect.Type, Nillable, GoName).
- Produces:

```go
// FieldSpec additions
StructIndex int              // index of the field in the entity struct; -1 = not represented
ScanValue   func() any       // ValueScanner fields only; nil otherwise
FromValue   func(any) (any, error) // ValueScanner fields only

// Descriptor additions
FKColumns []string           // unexposed FK columns scanned when withFKs (assign target found in Step 1)

// scan.go
func ScanTargets(desc *Descriptor, columns []string) ([]any, error)
func AssignRow(desc *Descriptor, entity any, columns []string, values []any) error
func FormatEntity(desc *Descriptor, entity any) string
```
Generated model methods become three-liners delegating to these (keep the
methods — `sqlgraph.QuerySpec.ScanValues/Assign` and other callers use them
by name).

- [ ] **Step 1: Survey the real conversion matrix.** Read `entc/integration/ent/internal/fieldtype_model.go`'s `ScanValues` + `AssignValues` end to end (the torture entity: enums, bytes, net.IP, sql.Null*, ValueScanner, JSON, custom GoTypes) plus `user_model.go` (FK columns — find where `user_spouse`-style FK values land: unexported struct fields? a map? this fork may differ from upstream). Produce the complete rule table (SQLType × Nillable × GoType-kind → scan cell type + assignment conversion) in the task report BEFORE writing code. Any case the table can't express generically (e.g. a `field.TypeOther` special) stays generated — the template keeps a per-field override hook for listed exceptions.
- [ ] **Step 2: Failing unit tests** in `scan_test.go`, one subtest per rule-table row, asserting `ScanTargets` cell types and `AssignRow` results against a hand-built struct + descriptor (mirror at minimum: plain string/int/bool/time, nillable of each, enum, []byte, GoType-over-[]byte, JSON struct, sql.Scanner GoType, ValueScanner round-trip, unknown column → `Other`-style passthrough matching current behavior).
- [ ] **Step 3: Implement `scan.go`** against the table. `FormatEntity` reproduces the current `String()` format exactly (`User(id=1, age=30, ...)` — copy the format from a generated `String()` and assert byte-equality in a test against a fixture struct).
- [ ] **Step 4: Template change**: model bodies → delegation; emit `StructIndex` (the template ranges fields in struct order — the index is the range position offset by the leading `ID`/`Config` fields; read the struct emission in the same template to compute it correctly) and `ScanValue`/`FromValue` for ValueScanner fields.
- [ ] **Step 5: Regenerate, restore rule, full fork gate.** `entc/integration/ent`'s `TestSQLite` + `type_test.go` (fieldtype round-trips, including the I1 Bool/Bytes assertions from 2b) are the proof. Watch `withFKs` paths: `Fetch` appends `ForeignKeys...` columns — the FK rule from Step 1 must hold or eager-loading tests fail.
- [ ] **Step 6: Commit** (runtime, template+regen).

---

### Task 5: Generic delete builder

**Files:**
- Create: `runtime/entbuilder/delete.go`, `runtime/entbuilder/delete_test.go`
- Modify: `entc/gen/template/builder/delete.tmpl`, `entc/gen/template/dialect/sql/delete.tmpl`

**Interfaces:**
- Consumes: descriptor SQL fields (Task 1).
- Produces, following stage 1's upsert-alias precedent exactly:

```go
// runtime/entbuilder/delete.go
type Delete[T any, I any] struct { /* Config-opaque; mutation *Mutation[T, I]; hooks */ }
func (d *Delete[T, I]) Where(ps ...func(*sql.Selector)) *Delete[T, I]
func (d *Delete[T, I]) Exec(ctx context.Context) (int, error)
func (d *Delete[T, I]) ExecX(ctx context.Context) int
type DeleteOne[T any, I any] struct { ... } // Where/Exec/ExecX
```
Generated: `type UserDelete = entbuilder.Delete[internal.User, int]` + a
`NewUserDelete(c Config, ...)`-style constructor shim in client.go — mirror
how `XUpsertOne = entbuilder.UpsertOne[ID]` + `OnConflict` shims were done
(`grep -n "UpsertOne\[" entc/integration/ent/user/create.go` for the shape).
Cross-entity type collapse (all int-ID deletes share one Go type) is the
same recorded deviation as stage 1's upsert aliases — carry it forward in
the results doc, don't re-debate it.

- [ ] **Step 1: Read the current generated delete** (`entc/integration/ent/user/delete.go`, 110 lines) and its sqlExec; list what's entity-specific (table name, ID spec, predicate type — all in the descriptor now; hooks path via `WithHooks`).
- [ ] **Step 2: Failing unit test** — descriptor-driven `Delete` against an in-memory SQLite via `sqlgraph.DeleteNodes` (or, if a driver-free assertion is feasible, assert the built `sqlgraph.DeleteSpec`; pick whichever the upsert tests did — mirror them).
- [ ] **Step 3: Implement, green, template swap, regen, restore rule, fork gate.** The predicate type on `Where` must remain the per-entity alias (`predicate.User` is `func(*sql.Selector)` — aliases make this assignable; confirm with a compile check in the integration tree).
- [ ] **Step 4: Commit** (runtime; template+regen).

---

### Task 6: entql decision — shared graph vs. feature drop (investigation, gemini-side ruling)

**Files:**
- Read-only investigation + a written ruling in the task report; any gemini
  edits happen in Task 7 based on it.

Gemini generates 55,394 LOC of entql (`entql*` files) to serve exactly two
hand-written helpers (`models/schema/utils.go:32-58`,
`IsUserOnEntityDealTeamPredicate` / `IsUserOnDealTeamPredicate`) that build
dynamic-by-entity-name privacy predicates via `entql.HasEdgeWith` +
`sqlgraph.WrapFunc`.

- [ ] **Step 1: Trace both helpers' full call graph** in gemini (who calls them, with which entity strings, through which privacy/scopes machinery — `grep -rn "IsUserOnEntityDealTeamPredicate\|IsUserOnDealTeamPredicate" models/ api/ workers/`).
- [ ] **Step 2: Determine whether `internal.SchemaGraph` (Task 2) + `sqlgraph.HasNeighborsWith`-style calls can replace them** without entql: the wrapped closure already works on `*sql.Selector`; what entql contributes is only edge-step resolution by (type, edge-name) string — which the shared graph can serve directly (that is exactly what `entql` does internally with the same graph).
- [ ] **Step 3: Write the ruling with a prototype snippet** (a compilable ~40-line replacement of `utils.go`'s two funcs against the shared graph, or a precise statement of why it can't work). If GO: Task 7 drops `"entql"` from `entc.FeatureNames` in gemini's `models/entc.go:129` and swaps `utils.go` (−55k). If NO-GO: gemini keeps entql; the shared-graph dedupe from Task 2 still applies to its `shared.go`.

---

### Task 7: Gemini migration + benchmarks + results

**Files:**
- Gemini worktree regen + migration (uncommitted); fork:
  `CODEGEN_REDUCTION_RESULTS.md` (Stage 3 section, committed).

- [ ] **Step 1: Regen** (`MREIS_CODEGEN_ALLOW_WATCHER=1 task generate-go` from `api/`). Extension templates that reference deleted surface (update-builder `Clear<E>`/`Remove<E>IDs`; anything touching `ScanValues` internals or per-entity `schemaGraph`) get the same treatment as 2b's template fixes — sweep `models/templates/` + `models/extensions/*/templates/` for `Clear[A-Z]\w*()\b` on update builders and `schemaGraph`.
- [ ] **Step 2: Rewriter pass** for deleted edge mutators (same `-chains` invocation per module as 2b; expect a small worklist — only Clear/Remove edge calls on update builders). Hand-fix the two known miss classes; iterate `go vet ./...` to clean in models/api/workers.
- [ ] **Step 3: entql ruling execution** (from Task 6): if GO, drop the feature flag, replace `utils.go` helpers, regen again, and run the privacy/scopes test packages that exercise deal-team scoping (find them: `grep -rln "IsUserOnDealTeam" api/integration/`).
- [ ] **Step 4: Tests**: models suite; workers with `-p 2 -parallel 4`; `-run OnConflict`; `ent_resolvers`; `transaction` + `contact` (both write-heavy, both previously exercised); one eager-loading-heavy package (scanner proof — `chatter` or `property`).
- [ ] **Step 5: Benchmarks AFTER tests, on a settled box** (load < 2; three-pass protocol from 2b if the first run looks confounded).
- [ ] **Step 6: Append the Stage 3 section** to `CODEGEN_REDUCTION_RESULTS.md`: numbers vs the 1,395,043/126.0s/72.9s baseline; deviations carried forward (delete-builder type collapse; hook-accessor sugar deferred; entql ruling either way; scanner conversion-rule exceptions if any); commit ONLY that doc in the fork.

---

## Self-Review

- **Spec coverage:** scan/assign/String → Task 4; generic mutation → already
  done pre-plan (recorded); loadX → already done pre-plan (recorded);
  shared/internal dedupe → Tasks 2+3 (schemaGraph + descriptor unification);
  delete.go shells → Task 5; client.go shrink → examined and skipped
  (already ~90 LOC/entity in gemini; no whale left — YAGNI, recorded);
  hooks/extensions migration → Task 7 rewriter pass.
- **Placeholder scan:** the deliberately-open items are investigation steps
  whose output feeds later steps in the same task (Step-1 reads), not
  implementation gaps; every code step has concrete signatures or snippets.
- **Type consistency:** `ApplyUpdateSpec`/`ApplyCreateSpec`/`BuildSchemaGraph`/
  `ScanTargets`/`AssignRow`/`FormatEntity`/`Delete[T, I]` names match across
  Tasks 1-5; descriptor field names (`Column`, `SQLType`, `StorageTable`,
  `StorageColumns`, `SchemaKey`, `StructIndex`) are used identically in
  Tasks 1, 2, 3, and 4.
