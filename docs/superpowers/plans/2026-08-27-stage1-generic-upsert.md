# Stage 1: Generic Upsert Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three per-entity generated upsert types (`XUpsert`, `XUpsertOne`, `XUpsertBulk` and their ~24 per-field methods each) with three generic types in `runtime/entbuilder`, cutting ~270k generated lines from gemini.

**Architecture:** The generated create builders keep their `conflict []sql.ConflictOption` field and spec plumbing (unchanged). The per-entity upsert types become type aliases of generic `entbuilder.Upsert` / `UpsertOne[ID]` / `UpsertBulk[ID]`, constructed by thin generated `OnConflict`/`OnConflictColumns` shims that pass closures + a small per-entity `UpsertMeta` var. Per-field setters become untyped column-based calls (`u.Set(escrow.FieldName, v)`); invalid columns/ops surface as database errors at exec time.

**Tech Stack:** Go 1.24 generics, `entgo.io/ent/dialect/sql` (`UpdateSet`, `ConflictOption` — all already column-based and generic underneath), text/template codegen in `entc/gen/template/dialect/sql/feature/upsert.tmpl`.

**Spec:** `docs/superpowers/specs/2026-08-27-codegen-reduction-design.md`

## Global Constraints

- Go 1.24; no new dependencies.
- All work in this repo happens in the current worktree (`.claude/worktrees/say-less`, branch `worktree-say-less`).
- Behavior parity with upstream ent upsert semantics: `UpdateNewValues` skips user-defined IDs and set immutable fields via `sql.SetIgnore`; `ID()` errors on MySQL for non-numeric user-defined IDs; error message formats match upstream (`"<pkg>: missing options for <Builder>.OnConflict"` etc.).
- Generated code must remain `gofmt`-clean (codegen pipeline formats output; don't fight it).
- Templates are embedded via `//go:embed template/*` in `entc/gen/template.go` — editing `.tmpl` files is enough; no extra embed step.
- Deliberate simplification (`ponytail:`): no runtime column/op validation table. Unknown columns, `Clear` on non-nullable, `Add` on non-numeric surface as database errors at exec. Upgrade path: per-entity field-metadata map if DB errors prove too opaque.

---

### Task 1: `entbuilder.Upsert` setter + `UpsertMeta`

**Files:**
- Create: `runtime/entbuilder/upsert.go`
- Test: `runtime/entbuilder/upsert_test.go`

**Interfaces:**
- Consumes: `dialect/sql.UpdateSet` (methods `Set`, `Add`, `SetNull`, `SetExcluded`, `SetIgnore` — see `dialect/sql/builder.go:398-422`), `field` not needed.
- Produces (Task 2 and the template rely on these exact names):
  - `type UpsertMeta struct { Pkg, Builder, IDColumn string; UserDefinedID, NumericID bool; Immutable []string }`
  - `type Upsert struct { *sql.UpdateSet }` with `Set(column string, v any) *Upsert`, `Add(column string, v any) *Upsert`, `Clear(column string) *Upsert`, `Update(columns ...string) *Upsert`.

- [ ] **Step 1: Write the failing test**

The `sql.UpdateSet` cannot be constructed directly; exercise it through a real Postgres-dialect INSERT builder and assert on the rendered SQL:

```go
// runtime/entbuilder/upsert_test.go
package entbuilder_test

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

func renderConflict(t *testing.T, opts ...sql.ConflictOption) string {
	t.Helper()
	query, _ := sql.Dialect(dialect.Postgres).
		Insert("users").Columns("name", "age").Values("a", 1).
		OnConflict(append([]sql.ConflictOption{sql.ConflictColumns("name")}, opts...)...).
		Query()
	return query
}

func TestUpsertSet(t *testing.T) {
	q := renderConflict(t, sql.ResolveWith(func(s *sql.UpdateSet) {
		u := &entbuilder.Upsert{UpdateSet: s}
		u.Set("name", "b")
	}))
	require.Contains(t, q, `"name" = `)
}

func TestUpsertUpdateTakesExcluded(t *testing.T) {
	q := renderConflict(t, sql.ResolveWith(func(s *sql.UpdateSet) {
		u := &entbuilder.Upsert{UpdateSet: s}
		u.Update("name")
	}))
	require.Contains(t, q, `"name" = excluded."name"`)
}

func TestUpsertClearSetsNull(t *testing.T) {
	q := renderConflict(t, sql.ResolveWith(func(s *sql.UpdateSet) {
		u := &entbuilder.Upsert{UpdateSet: s}
		u.Clear("name")
	}))
	require.Contains(t, q, `"name" = NULL`)
}

func TestUpsertAdd(t *testing.T) {
	q := renderConflict(t, sql.ResolveWith(func(s *sql.UpdateSet) {
		u := &entbuilder.Upsert{UpdateSet: s}
		u.Add("age", 2)
	}))
	require.Contains(t, q, "+")
}
```

If a rendered-SQL assertion doesn't match the actual `dialect/sql` output format, print the query and adjust the *assertion* to the real rendering (the rendering is upstream behavior, not code under test) — but each op must render a distinct, correct clause.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./runtime/entbuilder/ -run TestUpsert -v`
Expected: FAIL — `undefined: entbuilder.Upsert`

- [ ] **Step 3: Write the implementation**

```go
// runtime/entbuilder/upsert.go
package entbuilder

import (
	"entgo.io/ent/dialect/sql"
)

// UpsertMeta is the per-entity metadata codegen emits once per entity to
// drive the generic upsert builders.
type UpsertMeta struct {
	// Pkg and Builder are used in error messages, e.g. "gen", "EscrowCreate".
	Pkg, Builder string
	// IDColumn is the entity's ID column ("" when the entity has no single ID field).
	IDColumn string
	// UserDefinedID reports whether the ID is set by the user (schema-defined default/no autoincrement).
	UserDefinedID bool
	// NumericID reports whether the ID type is numeric (MySQL can return those from upserts).
	NumericID bool
	// Immutable lists immutable field columns skipped by UpdateNewValues when set on create.
	Immutable []string
}

// Upsert is the generic "ON CONFLICT" setter passed to Update callbacks.
// Columns are addressed by their field constants (e.g. escrow.FieldName).
// ponytail: no column/op validation — unknown columns and invalid ops
// (Clear on NOT NULL, Add on non-numeric) surface as database errors.
type Upsert struct {
	*sql.UpdateSet
}

// Set sets the column to the given value.
func (u *Upsert) Set(column string, v any) *Upsert {
	u.UpdateSet.Set(column, v)
	return u
}

// Add adds v to the column's current value.
func (u *Upsert) Add(column string, v any) *Upsert {
	u.UpdateSet.Add(column, v)
	return u
}

// Clear sets the column to NULL.
func (u *Upsert) Clear(column string) *Upsert {
	u.UpdateSet.SetNull(column)
	return u
}

// Update sets each column to the value proposed for insertion ("excluded").
func (u *Upsert) Update(columns ...string) *Upsert {
	for _, c := range columns {
		u.UpdateSet.SetExcluded(c)
	}
	return u
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./runtime/entbuilder/ -run TestUpsert -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/upsert.go runtime/entbuilder/upsert_test.go
git commit -m "feat(entbuilder): add generic Upsert conflict setter"
```

---

### Task 2: `UpsertOne[ID]` / `UpsertBulk[ID]` generic builders

**Files:**
- Modify: `runtime/entbuilder/upsert.go` (append)
- Test: `runtime/entbuilder/upsert_test.go` (append)

**Interfaces:**
- Consumes: Task 1's `Upsert`, `UpsertMeta`; `entgo.io/ent` (`ent.Value` — already imported by this package).
- Produces (the template in Task 3 relies on these exact names):

```go
type FieldReader interface{ Field(name string) (ent.Value, bool) }

type UpsertConfig[ID any] struct {
	Meta          *UpsertMeta
	Conflict      *[]sql.ConflictOption            // points at the create builder's conflict slice
	Err           func() error                     // optional: bulk builder's stored error
	ChildConflict func() int                       // optional: index of child builder with conflict set, -1 otherwise
	Exec          func(context.Context) error      // the create builder's Exec
	SaveID        func(context.Context) (ID, error) // nil → ID() unsupported (no single ID field)
	Mutations     func() []FieldReader             // the create builder mutation(s)
	IDsSet        func() []bool                    // aligned with Mutations; nil when ID isn't user-defined
	Dialect       func() string
}

func NewUpsertOne[ID any](cfg UpsertConfig[ID]) *UpsertOne[ID]
func NewUpsertBulk[ID any](cfg UpsertConfig[ID]) *UpsertBulk[ID]
```

- `UpsertOne[ID]` methods: `UpdateNewValues`, `Ignore`, `DoNothing`, `Update(set func(*Upsert))`, `Set(column string, v any)`, `Add(column string, v any)`, `Clear(column string)`, `UpdateFields(columns ...string)` (all returning `*UpsertOne[ID]`), `Exec(ctx) error`, `ExecX(ctx)`, `ID(ctx) (ID, error)`, `IDX(ctx) ID`.
- `UpsertBulk[ID]` methods: same minus `ID`/`IDX`.

- [ ] **Step 1: Write the failing tests**

```go
// append to runtime/entbuilder/upsert_test.go
import (
	"context"
	"errors"
	// ... existing imports

	"entgo.io/ent"
)

type fakeFieldReader map[string]any

func (f fakeFieldReader) Field(name string) (ent.Value, bool) { v, ok := f[name]; return v, ok }

func oneCfg(meta *entbuilder.UpsertMeta, conflict *[]sql.ConflictOption) entbuilder.UpsertConfig[int] {
	return entbuilder.UpsertConfig[int]{
		Meta:      meta,
		Conflict:  conflict,
		Exec:      func(context.Context) error { return nil },
		SaveID:    func(context.Context) (int, error) { return 7, nil },
		Mutations: func() []entbuilder.FieldReader { return []entbuilder.FieldReader{fakeFieldReader{"created_at": "x"}} },
		Dialect:   func() string { return dialect.Postgres },
	}
}

func TestUpsertOneExecMissingOptions(t *testing.T) {
	var conflict []sql.ConflictOption
	meta := &entbuilder.UpsertMeta{Pkg: "gen", Builder: "EscrowCreate"}
	u := entbuilder.NewUpsertOne(oneCfg(meta, &conflict))
	err := u.Exec(context.Background())
	require.EqualError(t, err, "gen: missing options for EscrowCreate.OnConflict")
}

func TestUpsertOneUpdateNewValuesIgnoresImmutable(t *testing.T) {
	var conflict []sql.ConflictOption
	meta := &entbuilder.UpsertMeta{
		Pkg: "gen", Builder: "EscrowCreate",
		IDColumn: "id", UserDefinedID: true,
		Immutable: []string{"created_at"},
	}
	cfg := oneCfg(meta, &conflict)
	cfg.IDsSet = func() []bool { return []bool{true} }
	entbuilder.NewUpsertOne(cfg).UpdateNewValues()
	q := renderConflict(t, conflict...)
	// user-defined ID that was set, and set immutable fields, resolve to themselves
	require.Contains(t, q, `"id" = "users"."id"`)
	require.Contains(t, q, `"created_at" = "users"."created_at"`)
}

func TestUpsertOneUpdateNewValuesSkipsUnsetImmutable(t *testing.T) {
	var conflict []sql.ConflictOption
	meta := &entbuilder.UpsertMeta{Pkg: "gen", Builder: "EscrowCreate", Immutable: []string{"created_at"}}
	cfg := oneCfg(meta, &conflict)
	cfg.Mutations = func() []entbuilder.FieldReader { return []entbuilder.FieldReader{fakeFieldReader{}} }
	entbuilder.NewUpsertOne(cfg).UpdateNewValues()
	q := renderConflict(t, conflict...)
	require.NotContains(t, q, `"created_at" = "users"."created_at"`)
}

func TestUpsertOneIDUnsupported(t *testing.T) {
	var conflict []sql.ConflictOption
	meta := &entbuilder.UpsertMeta{Pkg: "gen", Builder: "WordCreate"}
	cfg := entbuilder.UpsertConfig[struct{}]{
		Meta: meta, Conflict: &conflict,
		Exec:      func(context.Context) error { return nil },
		Mutations: func() []entbuilder.FieldReader { return nil },
		Dialect:   func() string { return dialect.Postgres },
	}
	_, err := entbuilder.NewUpsertOne(cfg).ID(context.Background())
	require.ErrorContains(t, err, "not supported")
}

func TestUpsertOneIDMySQLNonNumeric(t *testing.T) {
	var conflict []sql.ConflictOption
	meta := &entbuilder.UpsertMeta{Pkg: "gen", Builder: "EscrowCreate", UserDefinedID: true, NumericID: false}
	cfg := oneCfg(meta, &conflict)
	cfg.Dialect = func() string { return dialect.MySQL }
	_, err := entbuilder.NewUpsertOne(cfg).ID(context.Background())
	require.ErrorContains(t, err, "MySQL")
}

func TestUpsertOneIDHappyPath(t *testing.T) {
	conflict := []sql.ConflictOption{sql.ConflictColumns("name")}
	meta := &entbuilder.UpsertMeta{Pkg: "gen", Builder: "EscrowCreate"}
	id, err := entbuilder.NewUpsertOne(oneCfg(meta, &conflict)).ID(context.Background())
	require.NoError(t, err)
	require.Equal(t, 7, id)
}

func TestUpsertBulkChildConflict(t *testing.T) {
	conflict := []sql.ConflictOption{sql.ConflictColumns("name")}
	meta := &entbuilder.UpsertMeta{Pkg: "gen", Builder: "EscrowCreateBulk"}
	cfg := entbuilder.UpsertConfig[int]{
		Meta: meta, Conflict: &conflict,
		Exec:          func(context.Context) error { return nil },
		Mutations:     func() []entbuilder.FieldReader { return nil },
		ChildConflict: func() int { return 1 },
		Dialect:       func() string { return dialect.Postgres },
	}
	err := entbuilder.NewUpsertBulk(cfg).Exec(context.Background())
	require.EqualError(t, err, "gen: OnConflict was set for builder 1. Set it on the EscrowCreateBulk instead")
}

func TestUpsertBulkErrPropagates(t *testing.T) {
	conflict := []sql.ConflictOption{sql.ConflictColumns("name")}
	meta := &entbuilder.UpsertMeta{Pkg: "gen", Builder: "EscrowCreateBulk"}
	boom := errors.New("boom")
	cfg := entbuilder.UpsertConfig[int]{
		Meta: meta, Conflict: &conflict,
		Err:       func() error { return boom },
		Exec:      func(context.Context) error { return nil },
		Mutations: func() []entbuilder.FieldReader { return nil },
		Dialect:   func() string { return dialect.Postgres },
	}
	require.ErrorIs(t, entbuilder.NewUpsertBulk(cfg).Exec(context.Background()), boom)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./runtime/entbuilder/ -run "TestUpsertOne|TestUpsertBulk" -v`
Expected: FAIL — `undefined: entbuilder.UpsertConfig`

- [ ] **Step 3: Write the implementation**

Append to `runtime/entbuilder/upsert.go`:

```go
import (
	"context"
	"errors"
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
)

// FieldReader is the subset of ent.Mutation the upsert builders need.
type FieldReader interface {
	Field(name string) (ent.Value, bool)
}

// UpsertConfig wires a generated create builder into the generic upsert builders.
type UpsertConfig[ID any] struct {
	Meta          *UpsertMeta
	Conflict      *[]sql.ConflictOption
	Err           func() error
	ChildConflict func() int
	Exec          func(context.Context) error
	SaveID        func(context.Context) (ID, error)
	Mutations     func() []FieldReader
	IDsSet        func() []bool
	Dialect       func() string
}

// UpsertOne is the generic builder for upserting one entity.
type UpsertOne[ID any] struct {
	cfg UpsertConfig[ID]
}

// NewUpsertOne returns a generic single-row upsert builder.
func NewUpsertOne[ID any](cfg UpsertConfig[ID]) *UpsertOne[ID] {
	return &UpsertOne[ID]{cfg: cfg}
}

// UpsertBulk is the generic builder for upserting many entities.
type UpsertBulk[ID any] struct {
	cfg UpsertConfig[ID]
}

// NewUpsertBulk returns a generic bulk upsert builder.
func NewUpsertBulk[ID any](cfg UpsertConfig[ID]) *UpsertBulk[ID] {
	return &UpsertBulk[ID]{cfg: cfg}
}

// updateNewValues appends ResolveWithNewValues plus the ignore-resolver for
// user-defined IDs and immutable fields that were explicitly set on create.
func updateNewValues[ID any](cfg *UpsertConfig[ID]) {
	*cfg.Conflict = append(*cfg.Conflict, sql.ResolveWithNewValues())
	if !cfg.Meta.UserDefinedID && len(cfg.Meta.Immutable) == 0 {
		return
	}
	*cfg.Conflict = append(*cfg.Conflict, sql.ResolveWith(func(s *sql.UpdateSet) {
		var idsSet []bool
		if cfg.IDsSet != nil {
			idsSet = cfg.IDsSet()
		}
		for i, m := range cfg.Mutations() {
			if cfg.Meta.UserDefinedID && i < len(idsSet) && idsSet[i] {
				s.SetIgnore(cfg.Meta.IDColumn)
			}
			for _, col := range cfg.Meta.Immutable {
				if _, exists := m.Field(col); exists {
					s.SetIgnore(col)
				}
			}
		}
	}))
}

func resolveUpdate(conflict *[]sql.ConflictOption, set func(*Upsert)) {
	*conflict = append(*conflict, sql.ResolveWith(func(update *sql.UpdateSet) {
		set(&Upsert{UpdateSet: update})
	}))
}

func execUpsert[ID any](ctx context.Context, cfg *UpsertConfig[ID]) error {
	if cfg.Err != nil {
		if err := cfg.Err(); err != nil {
			return err
		}
	}
	if cfg.ChildConflict != nil {
		if i := cfg.ChildConflict(); i >= 0 {
			return fmt.Errorf("%s: OnConflict was set for builder %d. Set it on the %s instead", cfg.Meta.Pkg, i, cfg.Meta.Builder)
		}
	}
	if len(*cfg.Conflict) == 0 {
		return fmt.Errorf("%s: missing options for %s.OnConflict", cfg.Meta.Pkg, cfg.Meta.Builder)
	}
	return cfg.Exec(ctx)
}

// UpdateNewValues updates mutable fields using the values proposed for
// insertion, skipping user-defined IDs and immutable fields set on create.
func (u *UpsertOne[ID]) UpdateNewValues() *UpsertOne[ID] { updateNewValues(&u.cfg); return u }

// Ignore sets each column to itself in case of conflict.
func (u *UpsertOne[ID]) Ignore() *UpsertOne[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING` (SQLite/PostgreSQL only).
func (u *UpsertOne[ID]) DoNothing() *UpsertOne[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.DoNothing())
	return u
}

// Update allows setting the `UPDATE` clause columns via the generic Upsert setter.
func (u *UpsertOne[ID]) Update(set func(*Upsert)) *UpsertOne[ID] {
	resolveUpdate(u.cfg.Conflict, set)
	return u
}

// Set sets the column to v on conflict.
func (u *UpsertOne[ID]) Set(column string, v any) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Set(column, v) })
}

// Add adds v to the column on conflict.
func (u *UpsertOne[ID]) Add(column string, v any) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Add(column, v) })
}

// Clear sets the column to NULL on conflict.
func (u *UpsertOne[ID]) Clear(column string) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Clear(column) })
}

// UpdateFields sets each column to the value proposed for insertion.
func (u *UpsertOne[ID]) UpdateFields(columns ...string) *UpsertOne[ID] {
	return u.Update(func(s *Upsert) { s.Update(columns...) })
}

// Exec executes the upsert.
func (u *UpsertOne[ID]) Exec(ctx context.Context) error { return execUpsert(ctx, &u.cfg) }

// ExecX is like Exec, but panics on error.
func (u *UpsertOne[ID]) ExecX(ctx context.Context) {
	if err := u.Exec(ctx); err != nil {
		panic(err)
	}
}

// ID executes the upsert and returns the inserted/updated ID.
func (u *UpsertOne[ID]) ID(ctx context.Context) (id ID, err error) {
	if u.cfg.SaveID == nil {
		return id, fmt.Errorf("%s: %s.OnConflict ID is not supported on entities without a single ID field", u.cfg.Meta.Pkg, u.cfg.Meta.Builder)
	}
	if u.cfg.Meta.UserDefinedID && !u.cfg.Meta.NumericID && u.cfg.Dialect() == dialect.MySQL {
		// MySQL does not support RETURNING, so non-numeric IDs cannot be read back.
		return id, errors.New(u.cfg.Meta.Pkg + ": UpsertOne.ID is not supported by MySQL driver. Use UpsertOne.Exec instead")
	}
	if len(*u.cfg.Conflict) == 0 {
		return id, fmt.Errorf("%s: missing options for %s.OnConflict", u.cfg.Meta.Pkg, u.cfg.Meta.Builder)
	}
	return u.cfg.SaveID(ctx)
}

// IDX is like ID, but panics on error.
func (u *UpsertOne[ID]) IDX(ctx context.Context) ID {
	id, err := u.ID(ctx)
	if err != nil {
		panic(err)
	}
	return id
}

// UpdateNewValues updates mutable fields using the values proposed for
// insertion, skipping user-defined IDs and immutable fields set on create.
func (u *UpsertBulk[ID]) UpdateNewValues() *UpsertBulk[ID] { updateNewValues(&u.cfg); return u }

// Ignore sets each column to itself in case of conflict.
func (u *UpsertBulk[ID]) Ignore() *UpsertBulk[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.ResolveWithIgnore())
	return u
}

// DoNothing configures the conflict_action to `DO NOTHING` (SQLite/PostgreSQL only).
func (u *UpsertBulk[ID]) DoNothing() *UpsertBulk[ID] {
	*u.cfg.Conflict = append(*u.cfg.Conflict, sql.DoNothing())
	return u
}

// Update allows setting the `UPDATE` clause columns via the generic Upsert setter.
func (u *UpsertBulk[ID]) Update(set func(*Upsert)) *UpsertBulk[ID] {
	resolveUpdate(u.cfg.Conflict, set)
	return u
}

// Set sets the column to v on conflict.
func (u *UpsertBulk[ID]) Set(column string, v any) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Set(column, v) })
}

// Add adds v to the column on conflict.
func (u *UpsertBulk[ID]) Add(column string, v any) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Add(column, v) })
}

// Clear sets the column to NULL on conflict.
func (u *UpsertBulk[ID]) Clear(column string) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Clear(column) })
}

// UpdateFields sets each column to the value proposed for insertion.
func (u *UpsertBulk[ID]) UpdateFields(columns ...string) *UpsertBulk[ID] {
	return u.Update(func(s *Upsert) { s.Update(columns...) })
}

// Exec executes the upsert.
func (u *UpsertBulk[ID]) Exec(ctx context.Context) error { return execUpsert(ctx, &u.cfg) }

// ExecX is like Exec, but panics on error.
func (u *UpsertBulk[ID]) ExecX(ctx context.Context) {
	if err := u.Exec(ctx); err != nil {
		panic(err)
	}
}
```

Note: `ID()` intentionally repeats the missing-options check because it routes through `SaveID` (create's `Save`), not `Exec`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./runtime/entbuilder/ -v -run "TestUpsert"`
Expected: PASS (all Task 1 + Task 2 tests)

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/upsert.go runtime/entbuilder/upsert_test.go
git commit -m "feat(entbuilder): add generic UpsertOne/UpsertBulk builders"
```

---

### Task 3: Rewrite the upsert template, regenerate, migrate integration tests

**Files:**
- Modify: `entc/gen/template/dialect/sql/feature/upsert.tmpl` (full rewrite of `helper/upsertone`, `helper/upsertbulk`; delete `helper/upsert/fields`; keep the four small `fields/additional` + `spec` defines unchanged)
- Regenerated: `entc/integration/**` generated packages
- Modify: integration test call sites that use per-field upsert methods (`entc/integration/integration_test.go`, `entc/integration/customid/customid_test.go`, `entc/integration/edgeschema/edgeschema_test.go`)

**Interfaces:**
- Consumes: Task 1+2 exact names (`entbuilder.Upsert`, `UpsertOne[ID]`, `UpsertBulk[ID]`, `NewUpsertOne`, `NewUpsertBulk`, `UpsertConfig[ID]`, `UpsertMeta`, `FieldReader`).
- Produces: per entity, generated create.go now contains only — type aliases `XUpsert = entbuilder.Upsert`, `XUpsertOne = entbuilder.UpsertOne[<idType>]`, `XUpsertBulk = entbuilder.UpsertBulk[<idType>]` (`struct{}` when no single ID); one `var xUpsertMeta entbuilder.UpsertMeta`; `OnConflict`/`OnConflictColumns` on `XCreate` and `XCreateBulk`.

- [ ] **Step 1: Rewrite `helper/upsertone` in the template**

Replace everything from `{{ define "helper/upsertone" }}` through its `{{ end }}` with:

```
{{ define "helper/upsertone" }}
{{ $pkg := base $.Config.Package }}
{{ $builder := pascal $.Scope.Builder }}
{{ $receiver := $.Scope.Receiver }}
{{ $idType := "struct{}" }}{{ if $.HasOneFieldID }}{{ $idType = print $.ID.Type }}{{ end }}
{{ $meta := print (camel $.Name) "UpsertMeta" }}

type (
	// {{ $.Name }}Upsert is the "OnConflict" setter; columns are addressed
	// by their field constants (e.g. {{ $.Package }}.FieldName).
	{{ $.Name }}Upsert = entbuilder.Upsert

	// {{ $.Name }}UpsertOne is the builder for "upsert"-ing one {{ $.Name }} node.
	{{ $.Name }}UpsertOne = entbuilder.UpsertOne[{{ $idType }}]

	// {{ $.Name }}UpsertBulk is the builder for "upsert"-ing many {{ $.Name }} nodes.
	{{ $.Name }}UpsertBulk = entbuilder.UpsertBulk[{{ $idType }}]
)

var {{ $meta }} = entbuilder.UpsertMeta{
	Pkg: "{{ $pkg }}",
	Builder: "{{ $builder }}",
	{{- if $.HasOneFieldID }}
	IDColumn: {{ $.ID.Constant }},
	UserDefinedID: {{ $.ID.UserDefined }},
	NumericID: {{ $.ID.Type.Numeric }},
	{{- end }}
	{{- with $.ImmutableFields }}
	Immutable: []string{ {{ range . }}{{ .Constant }}, {{ end }} },
	{{- end }}
}

func ({{ $receiver }} *{{ $builder }}) upsertConfig() entbuilder.UpsertConfig[{{ $idType }}] {
	return entbuilder.UpsertConfig[{{ $idType }}]{
		Meta: &{{ $meta }},
		Conflict: &{{ $receiver }}.conflict,
		Exec: {{ $receiver }}.Exec,
		{{- if $.HasOneFieldID }}
		SaveID: func(ctx context.Context) ({{ $idType }}, error) {
			node, err := {{ $receiver }}.Save(ctx)
			if err != nil {
				var zero {{ $idType }}
				return zero, err
			}
			return node.ID, nil
		},
		{{- end }}
		{{- if and $.HasOneFieldID $.ID.UserDefined }}
		IDsSet: func() []bool {
			_, ok := {{ $receiver }}.mutation.ID()
			return []bool{ok}
		},
		{{- end }}
		Mutations: func() []entbuilder.FieldReader {
			return []entbuilder.FieldReader{ {{ $receiver }}.mutation }
		},
		Dialect: {{ $receiver }}.Drv.Dialect,
	}
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement. For example:
//
//	client.{{ $.Name }}.Create().
//		OnConflict(sql.ResolveWithNewValues()).
//		Update(func(u *{{ $pkg }}.{{ $.Name }}Upsert) {
//			u.Set({{ $.Package }}.FieldX, v)
//		}).
//		Exec(ctx)
func ({{ $receiver }} *{{ $builder }}) OnConflict(opts ...sql.ConflictOption) *{{ $.Name }}UpsertOne {
	{{ $receiver }}.conflict = opts
	return entbuilder.NewUpsertOne({{ $receiver }}.upsertConfig())
}

// OnConflictColumns calls `OnConflict` and configures the columns as conflict target.
func ({{ $receiver }} *{{ $builder }}) OnConflictColumns(columns ...string) *{{ $.Name }}UpsertOne {
	{{ $receiver }}.conflict = append({{ $receiver }}.conflict, sql.ConflictColumns(columns...))
	return entbuilder.NewUpsertOne({{ $receiver }}.upsertConfig())
}
{{ end }}
```

- [ ] **Step 2: Rewrite `helper/upsertbulk`**

Replace everything from `{{ define "helper/upsertbulk" }}` through its `{{ end }}` with:

```
{{ define "helper/upsertbulk" }}
{{ $builder := pascal $.Scope.Builder }}
{{ $receiver := $.Scope.Receiver }}
{{ $idType := "struct{}" }}{{ if $.HasOneFieldID }}{{ $idType = print $.ID.Type }}{{ end }}
{{ $meta := print (camel $.Name) "UpsertMeta" }}

func ({{ $receiver }} *{{ $builder }}) upsertBulkConfig() entbuilder.UpsertConfig[{{ $idType }}] {
	return entbuilder.UpsertConfig[{{ $idType }}]{
		Meta: &{{ $meta }},
		Conflict: &{{ $receiver }}.conflict,
		Err: func() error { return {{ $receiver }}.err },
		ChildConflict: func() int {
			for i, b := range {{ $receiver }}.builders {
				if len(b.conflict) != 0 {
					return i
				}
			}
			return -1
		},
		Exec: {{ $receiver }}.Exec,
		{{- if and $.HasOneFieldID $.ID.UserDefined }}
		IDsSet: func() []bool {
			set := make([]bool, len({{ $receiver }}.builders))
			for i, b := range {{ $receiver }}.builders {
				_, set[i] = b.mutation.ID()
			}
			return set
		},
		{{- end }}
		Mutations: func() []entbuilder.FieldReader {
			ms := make([]entbuilder.FieldReader, len({{ $receiver }}.builders))
			for i, b := range {{ $receiver }}.builders {
				ms[i] = b.mutation
			}
			return ms
		},
		Dialect: {{ $receiver }}.Drv.Dialect,
	}
}

// OnConflict allows configuring the `ON CONFLICT` / `ON DUPLICATE KEY` clause
// of the `INSERT` statement (see {{ $.CreateName }}.OnConflict).
func ({{ $receiver }} *{{ $builder }}) OnConflict(opts ...sql.ConflictOption) *{{ $.Name }}UpsertBulk {
	{{ $receiver }}.conflict = opts
	return entbuilder.NewUpsertBulk({{ $receiver }}.upsertBulkConfig())
}

// OnConflictColumns calls `OnConflict` and configures the columns as conflict target.
func ({{ $receiver }} *{{ $builder }}) OnConflictColumns(columns ...string) *{{ $.Name }}UpsertBulk {
	{{ $receiver }}.conflict = append({{ $receiver }}.conflict, sql.ConflictColumns(columns...))
	return entbuilder.NewUpsertBulk({{ $receiver }}.upsertBulkConfig())
}
{{ end }}
```

Also delete the entire `{{ define "helper/upsert/fields" }}...{{ end }}` block, and keep the first five defines in the file (`dialect/sql/create/fields/additional/upsert`, `dialect/sql/create_bulk/fields/additional/upsert`, `dialect/sql/create/spec/upsert`, `dialect/sql/create_bulk/spec/upsert`, `dialect/sql/create/additional/upsert`, `dialect/sql/create_bulk/additional/upsert`) exactly as they are.

Template-var caveats for the implementer: `helper/upsertone`/`helper/upsertbulk` are invoked with `extend $ "Receiver" ... "Builder" ...` scope from the create template — mirror the old file's use of `$.Scope.Receiver`/`$.Scope.Builder`; `$.Package` is the entity package identifier for constants; if `context` or `entbuilder` are unused in some entity's create.go the import block handles pruning automatically (it's generated by the shared import template). If `{{ $.ID.Type }}` prints an unqualified/unimported type in some edge case, check how the deleted ID() code printed it (it used `{{ $.ID.Type }}` too — same behavior).

- [ ] **Step 3: Regenerate the fork's own generated packages**

Run: `go generate ./...` (from repo root; regenerates `entc/integration/**` and `examples/**`)
Expected: success. Then `go build ./...` — expect FAILures only in *test files and examples* that call removed per-field upsert methods.

- [ ] **Step 4: Migrate integration/example call sites**

Mechanical mapping (find each compile error from Step 3 and apply):

| Old | New |
|---|---|
| `u.SetName(v)` (on `*XUpsert` in Update callback) | `u.Set(x.FieldName, v)` |
| `u.UpdateName()` (callback) | `u.Update(x.FieldName)` |
| `u.AddAge(v)` (callback) | `u.Add(x.FieldAge, v)` |
| `u.ClearName()` (callback) | `u.Clear(x.FieldName)` |
| `.SetName(v)` (chained on UpsertOne/Bulk) | `.Set(x.FieldName, v)` |
| `.UpdateName()` (chained) | `.UpdateFields(x.FieldName)` |
| `.AddAge(v)` (chained) | `.Add(x.FieldAge, v)` |
| `.ClearName()` (chained) | `.Clear(x.FieldName)` |

(`x` = the entity's package, e.g. `user.FieldName`. `OnConflict`, `OnConflictColumns`, `UpdateNewValues`, `Ignore`, `DoNothing`, `Update(func...)`, `Exec`, `ExecX`, `ID`, `IDX` and the `XUpsert*` type names are unchanged — aliases cover them.)

Repeat `go build ./...` until clean, then `go vet ./...`.

- [ ] **Step 5: Run the test suite**

Run:
```bash
go test ./runtime/entbuilder/ -count=1
go test ./entc/integration/ -run TestSQLite -count=1
go test ./entc/integration/customid/ -count=1
go test ./entc/integration/edgeschema/ -count=1
```
Expected: PASS (DB-dependent subtests for MySQL/Postgres skip when no server is available — SQLite subtests must pass; they cover `OnConflict` paths).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(entc): generate upsert via generic entbuilder builders"
```

---

### Task 4: Gemini migration, benchmarks, results doc

**Files:**
- Modify: `/home/smoothbrain/dev/matthews/gemini/.worktrees/main/models/go.mod` (fork pseudo-version bump), regenerated `models/gen/**`, ~30 app files using upsert
- Create: `CODEGEN_REDUCTION_RESULTS.md` (this repo, root — alongside `COMPACT_HELPERS_RESULTS.md`)

**Interfaces:**
- Consumes: everything above, via a pushed fork commit (gemini's go.mod replaces `entgo.io/ent` with `github.com/MatthewsREIS/ent@<pseudo-version>`; for local iteration use a `replace` to the local path first, but the committed go.mod must point at a pushed SHA).
- Produces: benchmark numbers (before/after) and a migrated, building, tested gemini.

- [ ] **Step 1: Capture the BEFORE baseline in gemini** (before touching go.mod)

```bash
cd /home/smoothbrain/dev/matthews/gemini/.worktrees/main/models
find gen -name '*.go' -print0 | xargs -0 cat | wc -l                    # LOC
/usr/bin/time -v go generate . 2>&1 | tail -25                          # gen wall time + max RSS
go clean -cache && /usr/bin/time -v go build ./... 2>&1 | tail -25      # clean build time + max RSS
```
Record all numbers. (Check `models/Taskfile.yaml` first — if generation is `task generate` rather than `go generate .`, use that command for both baseline and after, identically.)

- [ ] **Step 2: Point gemini at the new fork commit and regenerate**

Add `replace entgo.io/ent => /var/home/smoothbrain/dev/matthews/ent/.claude/worktrees/say-less` to `models/go.mod` for local testing, `go mod tidy`, regenerate with the same command as Step 1.

- [ ] **Step 3: Migrate the ~30 upsert call-site files**

`go build ./...` and fix every error using the Task 4 mapping table (same as Task 3 Step 4; the entity package for field constants is e.g. `models/gen/escrow`). Grep to find them up front:
```bash
grep -rln "Upsert\|OnConflict" --include='*.go' . | grep -v '/gen/'
```

- [ ] **Step 4: Run gemini's test suite**

Use gemini's own test entrypoint (check `Taskfile.yaml`; default `go test ./...` from the module roots that changed). Expected: same pass/fail set as before the migration (pre-existing failures are not this change's problem — record them if any).

- [ ] **Step 5: Capture the AFTER numbers**

Re-run the exact Step 1 commands. Record LOC, gen time/RSS, build time/RSS.

- [ ] **Step 6: Write `CODEGEN_REDUCTION_RESULTS.md`** (in this ent repo, root)

Structure: Stage 1 section with a before/after table (gen LOC, generation time, generation peak RSS, clean build time, build peak RSS), the delta percentages, and one-line notes on any semantic deviations (e.g. untyped setters, DB-level validation). Later stages append sections.

- [ ] **Step 7: Commit (both repos) and swap gemini back to a pushed pseudo-version**

```bash
# this repo
git add CODEGEN_REDUCTION_RESULTS.md && git commit -m "docs: stage 1 upsert reduction results"
```
Gemini's go.mod local `replace` must be reverted and pointed at the pushed fork SHA before its branch is pushed (coordinate with the user on pushing the fork branch first).

---

## Self-review notes

- Spec coverage: stage-1 spec section fully covered (generic types, untyped Set, shims, aliases, parity, tests, migration, benchmarks). The spec's "values validated at runtime against the descriptor" is deliberately narrowed to DB-level errors — recorded as a `ponytail:` constraint above and to be noted in the results doc.
- The `Upsert`/`UpsertOne`/`UpsertBulk` alias names preserve type-name compatibility for variable declarations and function signatures in app code; only per-field method calls change.
- Type consistency: template emits exactly the Task 2 names; `UpdateFields` (not `Update(cols...)`) on One/Bulk because `Update(func(*Upsert))` occupies that name there.
