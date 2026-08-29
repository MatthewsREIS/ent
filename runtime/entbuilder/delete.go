// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"
	"database/sql/driver"
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
)

// Delete is the generic builder for deleting T entities. Every generated
// <Entity>Delete collapses onto an alias `entbuilder.Delete[T, I]` (mirrors
// stage 1's UpsertOne[ID] alias precedent: every entity sharing the same Go
// ID type shares one Delete[T, I] instantiation, a deliberate cross-entity
// type collapse carried forward from that precedent, not re-debated here).
//
// Table/IDColumn/IDSQLType/SchemaKey are read off mutation's own *Descriptor
// (see mutation.go) — unlike the older per-entity sqlExec bodies this
// replaces, no separate per-entity descriptor is threaded in.
type Delete[T any, I any] struct {
	drv       dialect.Driver
	hooks     []ent.Hook
	mutation  *Mutation[T, I]
	modifiers []func(*sql.DeleteBuilder)

	// schemaOf resolves a Descriptor.SchemaKey to a runtime schema name for
	// multischema apps; nil for single-schema apps (SchemaKey is then never
	// read). Mirrors ApplyUpdateSpec/ApplyCreateSpec's schemaOf parameter.
	schemaOf func(key string) string
	// ctxSchema wraps ctx with the generated package's SchemaConfig before
	// DeleteNodes executes (mirrors the generated `internal.
	// NewSchemaConfigContext` call multischema apps make) — where.go's
	// predicate closures read it back via SchemaConfigFromContext to
	// schema-qualify cross-table references. nil for single-schema apps.
	ctxSchema func(context.Context) context.Context
	// wrapConstraintErr turns a raw constraint-violation error into the
	// generated package's *ConstraintError (a different Go type per
	// package, so entbuilder can't construct it directly). nil skips
	// wrapping (the raw error is returned as-is).
	wrapConstraintErr func(msg string, wrap error) error
}

// NewDelete returns a new Delete builder. schemaOf/ctxSchema/wrapConstraintErr
// are nil for apps that don't need them (single-schema, no constraint
// wrapping).
func NewDelete[T, I any](
	drv dialect.Driver,
	hooks []ent.Hook,
	mutation *Mutation[T, I],
	schemaOf func(key string) string,
	ctxSchema func(context.Context) context.Context,
	wrapConstraintErr func(msg string, wrap error) error,
) *Delete[T, I] {
	return &Delete[T, I]{
		drv: drv, hooks: hooks, mutation: mutation,
		schemaOf: schemaOf, ctxSchema: ctxSchema, wrapConstraintErr: wrapConstraintErr,
	}
}

// Where appends a list of predicates to the builder.
func (d *Delete[T, I]) Where(ps ...func(*sql.Selector)) *Delete[T, I] {
	d.mutation.WhereP(ps...)
	return d
}

// Modify adds a statement modifier for attaching custom logic to the DELETE
// statement.
func (d *Delete[T, I]) Modify(modifiers ...func(*sql.DeleteBuilder)) *Delete[T, I] {
	d.modifiers = append(d.modifiers, modifiers...)
	return d
}

// Exec executes the deletion query and returns how many vertices were deleted.
func (d *Delete[T, I]) Exec(ctx context.Context) (int, error) {
	return runMutate[int, *Mutation[T, I]](ctx, d.hooks, d.mutation, d.sqlExec)
}

// ExecX is like Exec, but panics if an error occurs.
func (d *Delete[T, I]) ExecX(ctx context.Context) int {
	n, err := d.Exec(ctx)
	if err != nil {
		panic(err)
	}
	return n
}

func (d *Delete[T, I]) sqlExec(ctx context.Context) (int, error) {
	desc := d.mutation.desc
	var idSpec *sqlgraph.FieldSpec
	if desc.IDColumn != "" {
		idSpec = sqlgraph.NewFieldSpec(desc.IDColumn, desc.IDSQLType)
	}
	spec := sqlgraph.NewDeleteSpec(desc.Table, idSpec)
	if d.schemaOf != nil && desc.SchemaKey != "" {
		spec.Node.Schema = d.schemaOf(desc.SchemaKey)
	}
	if d.ctxSchema != nil {
		ctx = d.ctxSchema(ctx)
	}
	spec.AddModifiers(d.modifiers...)
	if ps := d.mutation.MutationPredicates(); len(ps) > 0 {
		spec.Predicate = func(s *sql.Selector) {
			for i := range ps {
				ps[i](s)
			}
		}
	}
	affected, err := sqlgraph.DeleteNodes(ctx, d.drv, spec)
	if err != nil && sqlgraph.IsConstraintError(err) && d.wrapConstraintErr != nil {
		err = d.wrapConstraintErr(err.Error(), err)
	}
	d.mutation.SetDone()
	return affected, err
}

// DeleteOne is the generic builder for deleting a single T entity.
type DeleteOne[T any, I any] struct {
	d           *Delete[T, I]
	label       string
	notFoundErr func(label string) error
}

// NewDeleteOne wraps d as a DeleteOne builder. label/notFoundErr construct
// the generated package's *NotFoundError when Exec deletes nothing (a
// different Go type per package, so entbuilder can't construct it directly).
func NewDeleteOne[T, I any](d *Delete[T, I], label string, notFoundErr func(label string) error) *DeleteOne[T, I] {
	return &DeleteOne[T, I]{d: d, label: label, notFoundErr: notFoundErr}
}

// Where appends a list of predicates to the builder.
func (d *DeleteOne[T, I]) Where(ps ...func(*sql.Selector)) *DeleteOne[T, I] {
	d.d.mutation.WhereP(ps...)
	return d
}

// Exec executes the deletion query.
func (d *DeleteOne[T, I]) Exec(ctx context.Context) error {
	n, err := d.d.Exec(ctx)
	switch {
	case err != nil:
		return err
	case n == 0:
		return d.notFoundErr(d.label)
	default:
		return nil
	}
}

// ExecX is like Exec, but panics if an error occurs.
func (d *DeleteOne[T, I]) ExecX(ctx context.Context) {
	if err := d.Exec(ctx); err != nil {
		panic(err)
	}
}

// DeleteDescriptor describes how to translate the state stored on a delete
// mutation into the sqlgraph delete specification. Generated code provides the
// concrete handlers for each entity.
//
// Deprecated: retained only for generated code pinned before the stage-3
// generic Delete[T, I]/DeleteOne[T, I] (examples/ and downstream regen lag);
// remove when those are retired.
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
//
// Deprecated: retained only for generated code pinned before the stage-3
// generic Delete[T, I]/DeleteOne[T, I] (examples/ and downstream regen lag);
// remove when those are retired.
type DeleteSpecModifier[C any, M any] func(spec *sqlgraph.DeleteSpec, cfg C, mutation M) error

// DeleteIDDescriptor holds metadata and callbacks for identifier management in
// delete operations.
//
// Deprecated: retained only for generated code pinned before the stage-3
// generic Delete[T, I]/DeleteOne[T, I] (examples/ and downstream regen lag);
// remove when those are retired.
type DeleteIDDescriptor[M any] struct {
	Column string
	Type   field.Type
	// Value returns the identifier provided by the mutation. ok reports whether a
	// value exists.
	Value func(M) (driver.Value, bool, error)
}

// BuildDeleteSpec constructs a sqlgraph.DeleteSpec from the provided mutation
// using the descriptor metadata.
//
// Deprecated: retained only for generated code pinned before the stage-3
// generic Delete[T, I]/DeleteOne[T, I] (examples/ and downstream regen lag);
// remove when those are retired.
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

// DeleteState holds the generic state every generated <Entity>Delete and
// <Entity>DeleteOne builder carries.
//
// Deprecated: retained only for generated code pinned before the stage-3
// generic Delete[T, I]/DeleteOne[T, I] (examples/ and downstream regen lag);
// remove when those are retired.
type DeleteState[M any] struct {
	Hooks    []ent.Hook
	Mutation M
}

// RunDelete executes the Exec-shaped terminal for *<Entity>Delete builders.
//
// sqlExec is the per-entity SQL execution; it receives the prepared ctx.
// When state.Hooks is non-empty, the hook chain is built and invoked via
// the package-private runMutate helper shared with RunUpdate/RunUpdateOne
// (see update.go).
//
// Deprecated: retained only for generated code pinned before the stage-3
// generic Delete[T, I]/DeleteOne[T, I] (examples/ and downstream regen lag);
// remove when those are retired.
func RunDelete[M ent.Mutation](
	ctx context.Context,
	state *DeleteState[M],
	sqlExec func(context.Context) (int, error),
) (int, error) {
	return runMutate[int, M](ctx, state.Hooks, state.Mutation, sqlExec)
}
