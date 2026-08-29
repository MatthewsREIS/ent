// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// newDeleteMutation returns a fresh delete mutation over the thingDescriptor
// fixture (shared with sqlspec_test.go), with an ID predicate applied via
// WhereP the way a generated <Entity>DeleteOne.Where(id.ID(x)) would.
func newDeleteMutation(t *testing.T, id int) *entbuilder.Mutation[thingEnt, int] {
	t.Helper()
	m := entbuilder.NewMutation[thingEnt, int](nil, ent.OpDeleteOne, thingDescriptor())
	m.WhereP(func(s *sql.Selector) { s.Where(sql.EQ(s.C("id"), id)) })
	return m
}

func mockDriver(t *testing.T) (dialect.Driver, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return sql.OpenDB("", db), mock
}

func TestDeleteExec_BuildsSpecAndDeletes(t *testing.T) {
	drv, mock := mockDriver(t)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `things` WHERE `things`.`id` = ?")).
		WithArgs(7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	m := newDeleteMutation(t, 7)
	d := entbuilder.NewDelete[thingEnt, int](drv, nil, m, nil, nil, nil)
	n, err := d.Exec(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteExec_SchemaQualifiesTable(t *testing.T) {
	drv, mock := mockDriver(t)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `mydb`.`things` WHERE `mydb`.`things`.`id` = ?")).
		WithArgs(7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var ctxWrapped bool
	m := newDeleteMutation(t, 7)
	d := entbuilder.NewDelete[thingEnt, int](drv, nil, m,
		func(key string) string { require.Equal(t, "Thing", key); return "mydb" },
		func(ctx context.Context) context.Context { ctxWrapped = true; return ctx },
		nil,
	)
	n, err := d.Exec(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.True(t, ctxWrapped)
}

func TestDeleteExec_WrapsConstraintError(t *testing.T) {
	drv, mock := mockDriver(t)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `things` WHERE `things`.`id` = ?")).
		WithArgs(7).
		WillReturnError(errors.New("FOREIGN KEY constraint failed"))

	m := newDeleteMutation(t, 7)
	wrapped := errors.New("wrapped")
	d := entbuilder.NewDelete[thingEnt, int](drv, nil, m, nil, nil,
		func(msg string, wrap error) error {
			require.Contains(t, msg, "FOREIGN KEY constraint failed")
			return wrapped
		},
	)
	_, err := d.Exec(context.Background())
	require.Same(t, wrapped, err)
}

func TestDeleteExec_RunsHooks(t *testing.T) {
	drv, mock := mockDriver(t)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `things` WHERE `things`.`id` = ?")).
		WithArgs(7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var ran []string
	hook := ent.Hook(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			ran = append(ran, "hook")
			return next.Mutate(ctx, m)
		})
	})
	m := newDeleteMutation(t, 7)
	d := entbuilder.NewDelete[thingEnt, int](drv, []ent.Hook{hook}, m, nil, nil, nil)
	n, err := d.Exec(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, []string{"hook"}, ran)
}

func TestDeleteOneExec_NotFound(t *testing.T) {
	drv, mock := mockDriver(t)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `things` WHERE `things`.`id` = ?")).
		WithArgs(7).
		WillReturnResult(sqlmock.NewResult(0, 0))

	m := newDeleteMutation(t, 7)
	d := entbuilder.NewDelete[thingEnt, int](drv, nil, m, nil, nil, nil)
	one := entbuilder.NewDeleteOne(d, "Thing", func(label string) error {
		return &notFoundStub{label: label}
	})
	err := one.Exec(context.Background())
	var nf *notFoundStub
	require.ErrorAs(t, err, &nf)
	require.Equal(t, "Thing", nf.label)
}

func TestDeleteOneExec_Success(t *testing.T) {
	drv, mock := mockDriver(t)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `things` WHERE `things`.`id` = ?")).
		WithArgs(7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	m := newDeleteMutation(t, 7)
	d := entbuilder.NewDelete[thingEnt, int](drv, nil, m, nil, nil, nil)
	one := entbuilder.NewDeleteOne(d, "Thing", func(label string) error {
		return &notFoundStub{label: label}
	})
	require.NoError(t, one.Exec(context.Background()))
}

type notFoundStub struct{ label string }

func (e *notFoundStub) Error() string { return "not found: " + e.label }
