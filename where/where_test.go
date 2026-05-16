// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package where_test

import (
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/where"
	"github.com/stretchr/testify/require"
)

// predicateToSQL applies p to a Selector and returns the resulting WHERE clause + args.
// Mirrors how the generated code composes predicates against a sql.Selector.
func predicateToSQL(t *testing.T, p func(*sql.Selector)) (string, []any) {
	t.Helper()
	s := sql.Dialect(dialect.SQLite).Select("*").From(sql.Table("users"))
	p(s)
	q, args := s.Query()
	return q, args
}

func TestEQ_Int(t *testing.T) {
	q, args := predicateToSQL(t, where.EQ("age", 30))
	require.Contains(t, q, "`age` = ?")
	require.Equal(t, []any{30}, args)
}

func TestEQ_String(t *testing.T) {
	q, args := predicateToSQL(t, where.EQ("name", "alice"))
	require.Contains(t, q, "`name` = ?")
	require.Equal(t, []any{"alice"}, args)
}

func TestNEQ_Int(t *testing.T) {
	q, args := predicateToSQL(t, where.NEQ("age", 30))
	require.Contains(t, q, "`age` <> ?")
	require.Equal(t, []any{30}, args)
}

func TestIn_Int(t *testing.T) {
	q, args := predicateToSQL(t, where.In("age", 30, 40, 50))
	require.Contains(t, q, "`age` IN (?, ?, ?)")
	require.Equal(t, []any{30, 40, 50}, args)
}

func TestIn_String(t *testing.T) {
	q, args := predicateToSQL(t, where.In("name", "a", "b"))
	require.Contains(t, q, "`name` IN (?, ?)")
	require.Equal(t, []any{"a", "b"}, args)
}

func TestNotIn(t *testing.T) {
	q, args := predicateToSQL(t, where.NotIn("age", 1, 2))
	require.Contains(t, q, "`age` NOT IN (?, ?)")
	require.Equal(t, []any{1, 2}, args)
}

func TestGT(t *testing.T) {
	q, args := predicateToSQL(t, where.GT("age", 18))
	require.Contains(t, q, "`age` > ?")
	require.Equal(t, []any{18}, args)
}

func TestGTE(t *testing.T) {
	q, args := predicateToSQL(t, where.GTE("age", 18))
	require.Contains(t, q, "`age` >= ?")
	require.Equal(t, []any{18}, args)
}

func TestLT(t *testing.T) {
	q, args := predicateToSQL(t, where.LT("age", 65))
	require.Contains(t, q, "`age` < ?")
	require.Equal(t, []any{65}, args)
}

func TestLTE(t *testing.T) {
	q, args := predicateToSQL(t, where.LTE("age", 65))
	require.Contains(t, q, "`age` <= ?")
	require.Equal(t, []any{65}, args)
}

func TestIsNull(t *testing.T) {
	q, _ := predicateToSQL(t, where.IsNull("deleted_at"))
	require.Contains(t, q, "`deleted_at` IS NULL")
}

func TestNotNull(t *testing.T) {
	q, _ := predicateToSQL(t, where.NotNull("deleted_at"))
	require.Contains(t, q, "`deleted_at` IS NOT NULL")
}
