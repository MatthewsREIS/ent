// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"go/parser"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDescriptors_FromFixturePackage(t *testing.T) {
	// Resolve the in-repo privacy fixture path relative to the test file.
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	pkg := filepath.Join(root, "entc/integration/privacy/ent/internal")

	descs, err := LoadDescriptors(pkg)
	require.NoError(t, err)
	require.Contains(t, descs, "Task")
	require.Contains(t, descs, "User")
	require.Contains(t, descs, "Team")

	task := descs["Task"]
	require.Contains(t, task.Fields, "title")
	require.Equal(t, "Title", task.Fields["title"].GoName)
	require.Equal(t, "string", task.Fields["title"].Type)
	require.Contains(t, task.Edges, "teams")
	require.Contains(t, task.Edges, "owner")
}

// TestExprToString covers the printer-based renderer used to extract the
// generic type argument from `reflect.TypeFor[<T>]()` in descriptor specs.
// Regression: the prior ad-hoc switch only handled Ident/SelectorExpr/
// BasicLit, so complex types like map[string]any produced "", which then
// flowed into the rewriter as `entbuilder.GetField[](mutation, ...)` and
// broke the next parse pass.
func TestExprToString_MapType(t *testing.T) {
	expr, err := parser.ParseExpr("map[string]any")
	require.NoError(t, err)
	require.Equal(t, "map[string]any", exprToString(expr))
}

func TestExprToString_SliceType(t *testing.T) {
	expr, err := parser.ParseExpr("[]uuid.UUID")
	require.NoError(t, err)
	require.Equal(t, "[]uuid.UUID", exprToString(expr))
}

func TestExprToString_PointerType(t *testing.T) {
	expr, err := parser.ParseExpr("*int")
	require.NoError(t, err)
	require.Equal(t, "*int", exprToString(expr))
}

func TestExprToString_GenericIndexExpr(t *testing.T) {
	expr, err := parser.ParseExpr("Map[K, V]")
	require.NoError(t, err)
	// go/printer renders the comma-separated index list with a single space
	// after the comma — same as gofmt would emit.
	require.Equal(t, "Map[K, V]", exprToString(expr))
}

func TestExprToString_BasicLit(t *testing.T) {
	intExpr, err := parser.ParseExpr("42")
	require.NoError(t, err)
	require.Equal(t, "42", exprToString(intExpr))

	strExpr, err := parser.ParseExpr(`"hello"`)
	require.NoError(t, err)
	// STRING basic literals include their surrounding quotes — same as the
	// old switch's `e.Value` behavior.
	require.Equal(t, `"hello"`, exprToString(strExpr))
}

func TestExprToString_Ident(t *testing.T) {
	expr, err := parser.ParseExpr("Foo")
	require.NoError(t, err)
	require.Equal(t, "Foo", exprToString(expr))
}

func TestExprToString_SelectorExpr(t *testing.T) {
	expr, err := parser.ParseExpr("pkg.Type")
	require.NoError(t, err)
	require.Equal(t, "pkg.Type", exprToString(expr))
}

func TestExprToString_Nil(t *testing.T) {
	require.Equal(t, "", exprToString(nil))
}
