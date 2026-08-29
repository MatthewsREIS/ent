// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
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
