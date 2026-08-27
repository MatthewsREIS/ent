package entbuilder_test

import (
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
