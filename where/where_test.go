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
