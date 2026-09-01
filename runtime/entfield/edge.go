package entfield

import (
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
)

// Edge is the generated per-edge handle. step returns a fresh neighbor step
// (the generated newXStep() constructors have exactly this signature).
// TP is the neighbor entity's predicate type (an alias of P, kept for doc value).
// The constraint (not plain "any") is required so HasWith can call preds directly.
// ID is the neighbor entity's ID type, used by the assignment methods.
type Edge[TP ~func(*sql.Selector), ID any] struct {
	name            string
	step            func() *sqlgraph.Step
	stepMods        []func(*sql.Selector, *sqlgraph.Step)
	neighborFilters []func(*sql.Selector)
	// junctionFilters constrain the M2M join table itself rather than the
	// neighbor. They take the junction *sql.SelectTable explicitly because a
	// bare column name would be ambiguous once the neighbor table is joined in
	// — both sides routinely carry the same column (deleted_at, say).
	junctionFilters []func(*sql.Selector, *sql.SelectTable)
}

// WithJunction returns a copy of e whose Has/HasWith additionally constrain the
// M2M join table. Only meaningful for M2M edges: the join row is a first-class
// record for a through-edge, so a filter on it (a soft-delete marker, most
// obviously) has to be applied to the junction, and inside the same subquery as
// the neighbor predicate. Filtering it outside would answer a different
// question — "some junction row survives" rather than "this one does".
func (e Edge[TP, ID]) WithJunction(filters ...func(*sql.Selector, *sql.SelectTable)) Edge[TP, ID] {
	e.junctionFilters = append(append([]func(*sql.Selector, *sql.SelectTable){}, e.junctionFilters...), filters...)
	return e
}

// NewEdge creates a new Edge handle for the given step constructor. name is
// the schema edge name, used by the assignment methods (SetID/AddIDs/
// RemoveIDs/Clear). neighborFilters are applied (in order, after preds)
// inside HasWith's subquery only — Has() ignores them. This supports
// generated extensions (e.g. soft-delete) that inject an extra neighbor
// predicate inside HasNeighborsWith.
func NewEdge[TP ~func(*sql.Selector), ID any](name string, step func() *sqlgraph.Step, neighborFilters ...func(*sql.Selector)) Edge[TP, ID] {
	return Edge[TP, ID]{name: name, step: step, neighborFilters: neighborFilters}
}

// NewEdgeSteps is NewEdge plus stepMods: callbacks run (in order) against the
// freshly built step before it's used, with access to the outer selector (e.g.
// to read schemaConfig off s.Context()). Used by graphs with the sql/schemaconfig
// feature to route step.To.Schema / step.Edge.Schema, mirroring the routing the
// classic per-edge Has/HasWith codegen used to inline directly.
func NewEdgeSteps[TP ~func(*sql.Selector), ID any](name string, step func() *sqlgraph.Step, stepMods []func(*sql.Selector, *sqlgraph.Step), neighborFilters ...func(*sql.Selector)) Edge[TP, ID] {
	return Edge[TP, ID]{name: name, step: step, stepMods: stepMods, neighborFilters: neighborFilters}
}

// mkStep builds a fresh step and applies stepMods against it.
func (e Edge[TP, ID]) mkStep(s *sql.Selector) *sqlgraph.Step {
	step := e.step()
	for _, mod := range e.stepMods {
		mod(s, step)
	}
	return step
}

// Has returns a predicate testing for the existence of the edge.
func (e Edge[TP, ID]) Has() P {
	return func(s *sql.Selector) {
		step := e.mkStep(s)
		if len(e.junctionFilters) == 0 {
			sqlgraph.HasNeighbors(s, step)
			return
		}
		builder := sql.Dialect(s.Dialect())
		pk1 := step.Edge.Columns[0]
		if step.Edge.Inverse {
			pk1 = step.Edge.Columns[1]
		}
		junction := builder.Table(step.Edge.Table).Schema(step.Edge.Schema)
		sel := builder.Select(junction.C(pk1)).From(junction)
		sel.WithContext(s.Context())
		for _, f := range e.junctionFilters {
			f(sel, junction)
		}
		s.Where(sql.In(s.C(step.From.Column), sel))
	}
}

// HasWith returns a predicate testing for the existence of the edge with a
// given conditions (that are applied on the "other" side of the edge).
func (e Edge[TP, ID]) HasWith(preds ...TP) P {
	return func(s *sql.Selector) {
		step := e.mkStep(s)
		neighbor := func(s *sql.Selector) {
			for _, p := range preds {
				p(s)
			}
			for _, f := range e.neighborFilters {
				f(s)
			}
		}
		if len(e.junctionFilters) == 0 {
			sqlgraph.HasNeighborsWith(s, step, neighbor)
			return
		}
		builder := sql.Dialect(s.Dialect())
		pk1, pk2 := step.Edge.Columns[1], step.Edge.Columns[0]
		if step.Edge.Inverse {
			pk1, pk2 = pk2, pk1
		}
		to := builder.Table(step.To.Table).Schema(step.To.Schema)
		junction := builder.Table(step.Edge.Table).Schema(step.Edge.Schema)
		join := builder.Select(junction.C(pk2)).
			From(junction).
			Join(to).
			On(junction.C(pk1), to.C(step.To.Column))
		matches := builder.Select().From(to)
		matches.WithContext(s.Context())
		neighbor(matches)
		join.FromSelect(matches)
		// After FromSelect, not before: it replaces the selector's WHERE clause
		// with the neighbor predicates, which would drop these.
		for _, f := range e.junctionFilters {
			f(join, junction)
		}
		s.Where(sql.In(s.C(step.From.Column), join))
	}
}

// OrderByCount orders the results by the count of the edge connections.
func (e Edge[TP, ID]) OrderByCount(opts ...sql.OrderTermOption) Order {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborsCount(s, e.mkStep(s), opts...)
	}
}

// OrderBy orders the results by terms of the edge's neighbor table.
func (e Edge[TP, ID]) OrderBy(term sql.OrderTerm, terms ...sql.OrderTerm) Order {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborTerms(s, e.mkStep(s), append([]sql.OrderTerm{term}, terms...)...)
	}
}

// SetID assigns the neighbor ID on a unique edge.
func (e Edge[TP, ID]) SetID(v ID) Assignment {
	return func(m Mutable) error { return m.SetEdgeID(e.name, v) }
}

// SetNillableID assigns *v on a unique edge when non-nil; no-op otherwise.
func (e Edge[TP, ID]) SetNillableID(v *ID) Assignment {
	return func(m Mutable) error {
		if v == nil {
			return nil
		}
		return m.SetEdgeID(e.name, *v)
	}
}

// AddIDs adds neighbor IDs on a non-unique edge.
func (e Edge[TP, ID]) AddIDs(vs ...ID) Assignment {
	return func(m Mutable) error { return m.AddEdgeIDs(e.name, toAny(vs)...) }
}

// RemoveIDs removes neighbor IDs from a non-unique edge.
func (e Edge[TP, ID]) RemoveIDs(vs ...ID) Assignment {
	return func(m Mutable) error { return m.RemoveEdgeIDs(e.name, toAny(vs)...) }
}

// Clear clears the edge.
func (e Edge[TP, ID]) Clear() Assignment {
	return func(m Mutable) error { return m.ClearEdge(e.name) }
}
