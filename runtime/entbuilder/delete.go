package entbuilder

import (
	"database/sql/driver"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
)

// DeleteDescriptor describes how to translate the state stored on a delete
// mutation into the sqlgraph delete specification. Generated code provides the
// concrete handlers for each entity.
type DeleteDescriptor[C any, M any] struct {
	Table string
	ID    *DeleteIDDescriptor[M]
	// Schema returns the optional schema name for the delete operation. If the
	// bool result is false, the schema override is ignored.
	Schema func(C, M) (string, bool)
	// Predicates returns the set of SQL predicates collected on the mutation.
	Predicates func(M) []func(*sql.Selector)
	// Modifiers allows templates (including extension hooks) to mutate the spec
	// prior to execution.
	Modifiers []DeleteSpecModifier[C, M]
}

// DeleteSpecModifier mutates the DeleteSpec before execution. Returning an error
// aborts the delete operation.
type DeleteSpecModifier[C any, M any] func(spec *sqlgraph.DeleteSpec, cfg C, mutation M) error

// DeleteIDDescriptor holds metadata and callbacks for identifier management in
// delete operations.
type DeleteIDDescriptor[M any] struct {
	Column string
	Type   field.Type
	// Value returns the identifier provided by the mutation. ok reports whether a
	// value exists.
	Value func(M) (driver.Value, bool, error)
}

// BuildDeleteSpec constructs a sqlgraph.DeleteSpec from the provided mutation
// using the descriptor metadata.
func BuildDeleteSpec[C any, M any](cfg C, mutation M, desc *DeleteDescriptor[C, M]) (*sqlgraph.DeleteSpec, error) {
	if desc == nil {
		return nil, fmt.Errorf("entbuilder: missing delete descriptor")
	}
	var idSpec *sqlgraph.FieldSpec
	if desc.ID != nil && desc.ID.Column != "" {
		idSpec = sqlgraph.NewFieldSpec(desc.ID.Column, desc.ID.Type)
	}
	spec := sqlgraph.NewDeleteSpec(desc.Table, idSpec)
	if desc.Schema != nil {
		if schema, ok := desc.Schema(cfg, mutation); ok {
			spec.Node.Schema = schema
		}
	}
	if desc.ID != nil && desc.ID.Value != nil {
		value, ok, err := desc.ID.Value(mutation)
		if err != nil {
			return nil, err
		}
		if ok {
			spec.Node.ID.Value = value
		}
	}
	if desc.Predicates != nil {
		preds := desc.Predicates(mutation)
		if len(preds) > 0 {
			spec.Predicate = func(selector *sql.Selector) {
				for i := range preds {
					if preds[i] != nil {
						preds[i](selector)
					}
				}
			}
		}
	}
	for _, modify := range desc.Modifiers {
		if modify == nil {
			continue
		}
		if err := modify(spec, cfg, mutation); err != nil {
			return nil, err
		}
	}
	return spec, nil
}
