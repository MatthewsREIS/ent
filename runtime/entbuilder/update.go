package entbuilder

import (
	"context"
	"database/sql/driver"
	"fmt"

	"entgo.io/ent"
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

// UpdateState holds the generic state every generated <Entity>Update and
// <Entity>UpdateOne builder carries. Generated builders embed this and
// delegate Save/Exec to RunUpdate/RunUpdateOne.
//
// M is the entity's mutation pointer type, e.g., *UserMutation.
type UpdateState[M any] struct {
	Hooks    []ent.Hook
	Mutation M
}

// RunUpdate executes the Save-shaped terminal for *<Entity>Update builders.
// Returns the number of affected rows.
//
// sqlSave is the per-entity SQL execution; it receives the prepared ctx.
//
// When state.Hooks is non-empty, the hook chain (mutate-func → hooks → final
// sqlSave) is built and invoked. The chain mirrors the existing
// per-entity-package WithHooks helper that ent codegen has emitted for years.
func RunUpdate[M ent.Mutation](
	ctx context.Context,
	state *UpdateState[M],
	sqlSave func(context.Context) (int, error),
) (int, error) {
	return runMutate[int, M](ctx, state.Hooks, state.Mutation, sqlSave)
}

// RunUpdateOne executes the Save-shaped terminal for *<Entity>UpdateOne builders.
// Returns the updated entity.
func RunUpdateOne[T any, M ent.Mutation](
	ctx context.Context,
	state *UpdateState[M],
	sqlSave func(context.Context) (*T, error),
) (*T, error) {
	return runMutate[*T, M](ctx, state.Hooks, state.Mutation, sqlSave)
}

// runMutate is the shared hook-chaining mechanic. Mirrors the per-entity
// WithHooks helper that ent codegen has emitted: builds the chain in reverse
// (so the first registered hook wraps outermost), then invokes it.
//
// Shared between RunUpdate / RunUpdateOne (this file) and RunDelete
// (delete.go in Task 6) — keep package-private so it's an implementation detail.
func runMutate[V ent.Value, M ent.Mutation](
	ctx context.Context,
	hooks []ent.Hook,
	mutation M,
	exec func(context.Context) (V, error),
) (V, error) {
	var mut ent.Mutator = ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		return exec(ctx)
	})
	for i := len(hooks) - 1; i >= 0; i-- {
		mut = hooks[i](mut)
	}
	rv, err := mut.Mutate(ctx, mutation)
	if err != nil {
		var zero V
		return zero, err
	}
	v, ok := rv.(V)
	if !ok {
		var zero V
		return zero, fmt.Errorf("entbuilder: unexpected mutation result type %T", rv)
	}
	return v, nil
}
