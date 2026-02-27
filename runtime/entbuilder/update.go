package entbuilder

import (
	"database/sql/driver"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
)

// UpdateDescriptor describes how to translate the state stored on an update
// mutation into the sqlgraph update specification. Generated code provides the
// concrete field and edge handlers for each entity.
type UpdateDescriptor[C any, M any] struct {
	Fields []UpdateFieldDescriptor[M]
	Edges  []UpdateEdgeDescriptor[C, M]
}

// UpdateFieldDescriptor captures the handlers for updating a single field via a mutation.
type UpdateFieldDescriptor[M any] struct {
	Column string
	Type   field.Type
	// Set extracts the value assigned to the field (if any).
	Set func(M) (driver.Value, bool, error)
	// Add extracts the value that should be added to the field (for numeric increments).
	Add func(M) (driver.Value, bool, error)
	// Append returns a statement modifier that appends data to the field (e.g. JSON arrays).
	Append func(M) (func(*sql.UpdateBuilder), bool, error)
	// Clear reports whether the field should be cleared (set to NULL).
	Clear func(M) bool
}

// UpdateEdgeDescriptor holds callbacks for mutating relations during an update.
type UpdateEdgeDescriptor[C any, M any] struct {
	Clear  func(C, M) (*sqlgraph.EdgeSpec, bool, error)
	Remove func(C, M) ([]*sqlgraph.EdgeSpec, error)
	Add    func(C, M) ([]*sqlgraph.EdgeSpec, error)
}

// ApplyUpdate applies the descriptor handlers onto the provided sqlgraph.UpdateSpec.
func ApplyUpdate[C any, M any](cfg C, mutation M, desc *UpdateDescriptor[C, M], spec *sqlgraph.UpdateSpec) error {
	if desc == nil || spec == nil {
		return nil
	}
	for _, field := range desc.Fields {
		if field.Set != nil {
			if value, ok, err := field.Set(mutation); err != nil {
				return err
			} else if ok {
				spec.SetField(field.Column, field.Type, value)
			}
		}
		if field.Add != nil {
			if value, ok, err := field.Add(mutation); err != nil {
				return err
			} else if ok {
				spec.AddField(field.Column, field.Type, value)
			}
		}
		if field.Append != nil {
			if fn, ok, err := field.Append(mutation); err != nil {
				return err
			} else if ok && fn != nil {
				spec.AddModifier(fn)
			}
		}
		if field.Clear != nil && field.Clear(mutation) {
			spec.ClearField(field.Column, field.Type)
		}
	}
	for _, edge := range desc.Edges {
		if edge.Clear != nil {
			es, ok, err := edge.Clear(cfg, mutation)
			if err != nil {
				return err
			}
			if ok && es != nil {
				spec.Edges.Clear = append(spec.Edges.Clear, es)
			}
		}
		if edge.Remove != nil {
			edges, err := edge.Remove(cfg, mutation)
			if err != nil {
				return err
			}
			if len(edges) > 0 {
				spec.Edges.Clear = append(spec.Edges.Clear, edges...)
			}
		}
		if edge.Add != nil {
			edges, err := edge.Add(cfg, mutation)
			if err != nil {
				return err
			}
			if len(edges) > 0 {
				spec.Edges.Add = append(spec.Edges.Add, edges...)
			}
		}
	}
	return nil
}
