// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
)

// Resolver provides type-aware AST navigation for a single Go file.
// It runs the type checker against the parsed file (resolving imports via
// the source importer) so rewrite passes can gate dispatch on the static
// type of a method receiver — necessary to avoid corrupting schema-DSL
// call sites that share method names with generated mutation/query types.
type Resolver struct {
	Fset *token.FileSet
	File *ast.File
	Info *types.Info
	Pkg  *types.Package
}

// NewResolver parses src and runs the type checker. The returned resolver
// is safe for read-only inspection. If type checking fails on some imports
// (common with generated code that depends on external modules), the
// resolver still returns whatever types resolved successfully; callers
// should treat MatchesReceiverType as "best-effort with skip-on-uncertain".
func NewResolver(filename, src string) (*Resolver, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{
		Importer: importer.Default(),
		Error: func(err error) {
			// Swallow per-file errors; partial type info is still useful.
		},
	}
	pkg, _ := conf.Check(file.Name.Name, fset, []*ast.File{file}, info)
	return &Resolver{Fset: fset, File: file, Info: info, Pkg: pkg}, nil
}

// FindFirstCall walks the AST and returns the first ast.CallExpr whose
// Fun is a SelectorExpr with Sel.Name == name. Returns nil if none found.
// Used by tests; production callers walk via astutil.Apply.
func (r *Resolver) FindFirstCall(name string) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(r.File, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == name {
			found = call
			return false
		}
		return true
	})
	return found
}

// MatchesReceiverType reports whether the receiver expression of call
// (call.Fun.(*ast.SelectorExpr).X) has the static type wantTypeName.
// wantTypeName format: "*pkgname.TypeName" (e.g. "*ent.TaskMutation",
// "*x.TaskMutation"). The leading "*" is required for pointer receivers.
//
// Returns false if:
//   - call.Fun isn't a SelectorExpr (e.g., direct function call)
//   - the receiver type can't be resolved (type checker had insufficient info)
//   - the resolved type doesn't match wantTypeName
//
// "False on uncertain" is the safe default — a missed rewrite is fixable
// by a follow-up tool run, but a wrong rewrite corrupts consumer code.
func (r *Resolver) MatchesReceiverType(call *ast.CallExpr, wantTypeName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	tv, ok := r.Info.Types[sel.X]
	if !ok || tv.Type == nil {
		return false
	}
	return tv.Type.String() == wantTypeName
}

// ReceiverTypeMatchesPattern is like MatchesReceiverType but accepts a
// predicate function — useful when the rewrite pass needs to match any
// of several types (e.g. "*ent.*Mutation" for all entity mutations).
func (r *Resolver) ReceiverTypeMatchesPattern(call *ast.CallExpr, pred func(typeName string) bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	tv, ok := r.Info.Types[sel.X]
	if !ok || tv.Type == nil {
		return false
	}
	return pred(tv.Type.String())
}
