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
	e := entfield.NewEdge[entfield.P, int]("pets", petsStep)
	q, _ := render(t, e.Has())
	require.Contains(t, q, `EXISTS (SELECT "pets"."owner_id" FROM "pets" WHERE "users"."id" = "pets"."owner_id")`)
}

func TestEdgeHasWith(t *testing.T) {
	e := entfield.NewEdge[entfield.P, int]("pets", petsStep)
	inner := sql.FieldEQ("name", "fido")
	q, args := render(t, e.HasWith(inner))
	require.Contains(t, q, `EXISTS (SELECT "pets"."owner_id" FROM "pets" WHERE "users"."id" = "pets"."owner_id" AND "pets"."name" = $1)`)
	require.Equal(t, []any{"fido"}, args)
}

func TestEdgeHasWithNeighborFilter(t *testing.T) {
	filter := sql.FieldIsNull("deleted_at")
	e := entfield.NewEdge[entfield.P, int]("pets", petsStep, filter)
	inner := sql.FieldEQ("name", "fido")
	q, args := render(t, e.HasWith(inner))
	require.Contains(t, q, `"pets"."name" = $1`)
	require.Contains(t, q, `"pets"."deleted_at" IS NULL`)
	require.Equal(t, []any{"fido"}, args)
}

// groupsStep mirrors a generated M2M through-edge: users <-> groups joined by
// the user_groups table, which is itself an entity carrying deleted_at.
func groupsStep() *sqlgraph.Step {
	return sqlgraph.NewStep(
		sqlgraph.From("users", "id"),
		sqlgraph.To("groups", "id"),
		sqlgraph.Edge(sqlgraph.M2M, false, "user_groups", "user_id", "group_id"),
	)
}

func junctionNotDeleted() func(*sql.Selector, *sql.SelectTable) {
	return func(sel *sql.Selector, t *sql.SelectTable) { sel.Where(sql.IsNull(t.C("deleted_at"))) }
}

// A soft-deleted junction row means the edge is gone even though both endpoints
// are live, so Has() must not match on it.
//
// FALSIFY: drop the WithJunction call — the emitted SQL loses the
// "user_groups"."deleted_at" IS NULL term and this fails.
func TestEdgeHasJunctionFilter(t *testing.T) {
	e := entfield.NewEdge[entfield.P, int]("groups", groupsStep).WithJunction(junctionNotDeleted())
	q, _ := render(t, e.Has())
	require.Contains(t, q, `"user_groups"."deleted_at" IS NULL`)
	require.Contains(t, q, `"users"."id" IN (SELECT "user_groups"."user_id" FROM "user_groups"`)
}

// The junction filter has to sit inside the same subquery as the neighbor
// predicate. Filtering outside would answer "some junction row survives"
// instead of "the row linking to THIS neighbor survives" — which differs
// exactly when one junction is deleted and a sibling is not.
//
// FALSIFY: move the junctionFilters loop above the join.FromSelect call in
// HasWith — FromSelect replaces the WHERE clause and the deleted_at term
// disappears from the rendered SQL.
func TestEdgeHasWithJunctionFilter(t *testing.T) {
	e := entfield.NewEdge[entfield.P, int]("groups", groupsStep).WithJunction(junctionNotDeleted())
	q, args := render(t, e.HasWith(sql.FieldEQ("name", "admins")))
	// The neighbor table is joined under an alias, so its predicate renders
	// against t1; what matters is that both terms land in the one subquery.
	require.Contains(t, q, `JOIN "groups" AS "t1" ON "user_groups"."group_id" = "t1"."id" WHERE "t1"."name" = $1 AND "user_groups"."deleted_at" IS NULL`)
	require.Equal(t, []any{"admins"}, args)
}

// Without WithJunction nothing changes: the ordinary sqlgraph path still runs,
// so non-through M2M edges and every O2M/M2O edge are untouched.
func TestEdgeNoJunctionFilterUnchanged(t *testing.T) {
	e := entfield.NewEdge[entfield.P, int]("groups", groupsStep)
	q, _ := render(t, e.Has())
	require.NotContains(t, q, "deleted_at")
}

func TestEdgeStepModsHas(t *testing.T) {
	mod := func(_ *sql.Selector, step *sqlgraph.Step) {
		step.To.Schema = "alt_schema"
		step.Edge.Schema = "alt_schema"
	}
	e := entfield.NewEdgeSteps[entfield.P, int]("pets", petsStep, []func(*sql.Selector, *sqlgraph.Step){mod})
	q, _ := render(t, e.Has())
	require.Contains(t, q, `"alt_schema"."pets"`)
}

func TestEdgeStepModsHasWith(t *testing.T) {
	mod := func(_ *sql.Selector, step *sqlgraph.Step) {
		step.To.Schema = "alt_schema"
		step.Edge.Schema = "alt_schema"
	}
	e := entfield.NewEdgeSteps[entfield.P, int]("pets", petsStep, []func(*sql.Selector, *sqlgraph.Step){mod})
	inner := sql.FieldEQ("name", "fido")
	q, _ := render(t, e.HasWith(inner))
	require.Contains(t, q, `"alt_schema"."pets"`)
}

func TestEdgeOrderByCount(t *testing.T) {
	e := entfield.NewEdge[entfield.P, int]("pets", petsStep)
	q, _ := render(t, e.OrderByCount())
	require.Contains(t, q, `COUNT(*) AS "count_pets"`)
	require.Contains(t, q, `ORDER BY "t1"."count_pets"`)
}

func TestEdgeOrderBy(t *testing.T) {
	e := entfield.NewEdge[entfield.P, int]("pets", petsStep)
	q, _ := render(t, e.OrderBy(sql.OrderByField("name")))
	require.Contains(t, q, `SELECT "pets"."owner_id", "pets"."name" FROM "pets"`)
	require.Contains(t, q, `ORDER BY "t1"."name"`)
}
