package entbuilder_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent"
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
	require.Contains(t, q, `"name" = "excluded"."name"`)
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
