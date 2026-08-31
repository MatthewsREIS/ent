package entfield

import (
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
)

// Edge is the generated per-edge handle. step returns a fresh neighbor step
// (the generated newXStep() constructors have exactly this signature).
// TP is the neighbor entity's predicate type (an alias of P, kept for doc value).
// The constraint (not plain "any") is required so HasWith can call preds directly.
type Edge[TP ~func(*sql.Selector)] struct {
	step            func() *sqlgraph.Step
	stepMods        []func(*sql.Selector, *sqlgraph.Step)
	neighborFilters []func(*sql.Selector)
}

// NewEdge creates a new Edge handle for the given step constructor. neighborFilters
// are applied (in order, after preds) inside HasWith's subquery only — Has() ignores
// them. This supports generated extensions (e.g. soft-delete) that inject an extra
// neighbor predicate inside HasNeighborsWith.
func NewEdge[TP ~func(*sql.Selector)](step func() *sqlgraph.Step, neighborFilters ...func(*sql.Selector)) Edge[TP] {
	return Edge[TP]{step: step, neighborFilters: neighborFilters}
}

// NewEdgeSteps is NewEdge plus stepMods: callbacks run (in order) against the
// freshly built step before it's used, with access to the outer selector (e.g.
// to read schemaConfig off s.Context()). Used by graphs with the sql/schemaconfig
// feature to route step.To.Schema / step.Edge.Schema, mirroring the routing the
// classic per-edge Has/HasWith codegen used to inline directly.
func NewEdgeSteps[TP ~func(*sql.Selector)](step func() *sqlgraph.Step, stepMods []func(*sql.Selector, *sqlgraph.Step), neighborFilters ...func(*sql.Selector)) Edge[TP] {
	return Edge[TP]{step: step, stepMods: stepMods, neighborFilters: neighborFilters}
}

// mkStep builds a fresh step and applies stepMods against it.
func (e Edge[TP]) mkStep(s *sql.Selector) *sqlgraph.Step {
	step := e.step()
	for _, mod := range e.stepMods {
		mod(s, step)
	}
	return step
}

// Has returns a predicate testing for the existence of the edge.
func (e Edge[TP]) Has() P {
	return func(s *sql.Selector) {
		sqlgraph.HasNeighbors(s, e.mkStep(s))
	}
}

// HasWith returns a predicate testing for the existence of the edge with a
// given conditions (that are applied on the "other" side of the edge).
func (e Edge[TP]) HasWith(preds ...TP) P {
	return func(s *sql.Selector) {
		sqlgraph.HasNeighborsWith(s, e.mkStep(s), func(s *sql.Selector) {
			for _, p := range preds {
				p(s)
			}
			for _, f := range e.neighborFilters {
				f(s)
			}
		})
	}
}

// OrderByCount orders the results by the count of the edge connections.
func (e Edge[TP]) OrderByCount(opts ...sql.OrderTermOption) Order {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborsCount(s, e.mkStep(s), opts...)
	}
}

// OrderBy orders the results by terms of the edge's neighbor table.
func (e Edge[TP]) OrderBy(term sql.OrderTerm, terms ...sql.OrderTerm) Order {
	return func(s *sql.Selector) {
		sqlgraph.OrderByNeighborTerms(s, e.mkStep(s), append([]sql.OrderTerm{term}, terms...)...)
	}
}
