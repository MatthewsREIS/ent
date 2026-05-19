// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewritePredicate_EQ(t *testing.T) {
	descs := Descriptors{
		"User": &EntityDesc{Name: "User", Fields: map[string]FieldDesc{"name": {GoName: "Name", Type: "string"}}},
	}
	src := `package x
import "example.com/ent/user"
func q() { _ = user.NameEQ("alice") }
`
	out, err := RewritePredicateSource("q.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `where.EQ(user.FieldName, "alice")`)
	require.NotContains(t, out, "user.NameEQ")
}

func TestRewritePredicate_Contains(t *testing.T) {
	descs := Descriptors{
		"User": &EntityDesc{Name: "User", Fields: map[string]FieldDesc{"name": {GoName: "Name", Type: "string"}}},
	}
	src := `package x
import "example.com/ent/user"
func q() { _ = user.NameContains("ali") }
`
	out, err := RewritePredicateSource("q.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `where.Contains(user.FieldName, "ali")`)
}

func TestRewritePredicate_PreservesIDHelpers(t *testing.T) {
	descs := Descriptors{
		"User": &EntityDesc{Name: "User", Fields: map[string]FieldDesc{"name": {GoName: "Name", Type: "string"}}},
	}
	src := `package x
import "example.com/ent/user"
func q() { _ = user.IDEQ(7); _ = user.IDIn(1, 2, 3) }
`
	out, err := RewritePredicateSource("q.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, "user.IDEQ(7)") // unchanged
	require.Contains(t, out, "user.IDIn(1, 2, 3)")
}
