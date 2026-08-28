package entfield_test

import (
	"testing"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/runtime/entfield"
	"github.com/stretchr/testify/require"
)

// petsStep mirrors a generated newPetsStep() constructor for a users -> pets O2M edge.
func petsStep() *sqlgraph.Step {
	return sqlgraph.NewStep(
		sqlgraph.From("users", "id"),
		sqlgraph.To("pets", "id"),
		sqlgraph.Edge(sqlgraph.O2M, false, "pets", "owner_id"),
	)
}

func TestEdgeHas(t *testing.T) {
	e := entfield.NewEdge[entfield.P](petsStep)
	q, _ := render(t, e.Has())
	require.Contains(t, q, `EXISTS (SELECT "pets"."owner_id" FROM "pets" WHERE "users"."id" = "pets"."owner_id")`)
}

func TestEdgeHasWith(t *testing.T) {
	e := entfield.NewEdge[entfield.P](petsStep)
	inner := sql.FieldEQ("name", "fido")
	q, args := render(t, e.HasWith(inner))
	require.Contains(t, q, `EXISTS (SELECT "pets"."owner_id" FROM "pets" WHERE "users"."id" = "pets"."owner_id" AND "pets"."name" = $1)`)
	require.Equal(t, []any{"fido"}, args)
}

func TestEdgeHasWithNeighborFilter(t *testing.T) {
	filter := sql.FieldIsNull("deleted_at")
	e := entfield.NewEdge[entfield.P](petsStep, filter)
	inner := sql.FieldEQ("name", "fido")
	q, args := render(t, e.HasWith(inner))
	require.Contains(t, q, `"pets"."name" = $1`)
	require.Contains(t, q, `"pets"."deleted_at" IS NULL`)
	require.Equal(t, []any{"fido"}, args)
}

func TestEdgeStepModsHas(t *testing.T) {
	mod := func(_ *sql.Selector, step *sqlgraph.Step) {
		step.To.Schema = "alt_schema"
		step.Edge.Schema = "alt_schema"
	}
	e := entfield.NewEdgeSteps[entfield.P](petsStep, []func(*sql.Selector, *sqlgraph.Step){mod})
	q, _ := render(t, e.Has())
	require.Contains(t, q, `"alt_schema"."pets"`)
}

func TestEdgeStepModsHasWith(t *testing.T) {
	mod := func(_ *sql.Selector, step *sqlgraph.Step) {
		step.To.Schema = "alt_schema"
		step.Edge.Schema = "alt_schema"
	}
	e := entfield.NewEdgeSteps[entfield.P](petsStep, []func(*sql.Selector, *sqlgraph.Step){mod})
	inner := sql.FieldEQ("name", "fido")
	q, _ := render(t, e.HasWith(inner))
	require.Contains(t, q, `"alt_schema"."pets"`)
}

func TestEdgeOrderByCount(t *testing.T) {
	e := entfield.NewEdge[entfield.P](petsStep)
	q, _ := render(t, e.OrderByCount())
	require.Contains(t, q, `COUNT(*) AS "count_pets"`)
	require.Contains(t, q, `ORDER BY "t1"."count_pets"`)
}

func TestEdgeOrderBy(t *testing.T) {
	e := entfield.NewEdge[entfield.P](petsStep)
	q, _ := render(t, e.OrderBy(sql.OrderByField("name")))
	require.Contains(t, q, `SELECT "pets"."owner_id", "pets"."name" FROM "pets"`)
	require.Contains(t, q, `ORDER BY "t1"."name"`)
}
