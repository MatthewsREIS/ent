// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/printer"
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
//
// Resolution strategy:
//  1. Try go/types resolution — preferred when imports succeed.
//  2. Fall back to AST-only declaration scanning: if call.Fun.X is a
//     plain identifier (typical method-on-variable case), look up the
//     enclosing function's params and short-var-decls for that name,
//     render the declared type expression as text, and pass it to pred.
//
// The fallback is necessary because the integration tool runs against
// individual files where cross-package imports cannot always be
// resolved by the source importer. Without it the gate would skip
// every cross-package receiver (false negative).
func (r *Resolver) ReceiverTypeMatchesPattern(call *ast.CallExpr, pred func(typeName string) bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	// Path 1: go/types resolution.
	if tv, ok := r.Info.Types[sel.X]; ok && tv.Type != nil {
		t := tv.Type.String()
		if t != "" && !isInvalidType(t) {
			return pred(t)
		}
	}
	// Path 2: AST-only fallback. Only handles the simple case where the
	// receiver expression is a bare identifier (e.g. `m.SetTitle(...)`).
	// Chained receivers (`From("x").Ref(...)`) fall through and return
	// false — safer than guessing.
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	typeText := r.lookupVarTypeText(ident)
	if typeText == "" {
		return false
	}
	return pred(typeText)
}

// isInvalidType reports whether t is the textual representation of an
// invalid (unresolved) Go type. go/types renders unresolved imports as
// "invalid type" or types containing "invalid".
func isInvalidType(t string) bool {
	return t == "invalid type" || t == "*invalid type"
}

// lookupVarTypeText finds the declaration of the variable named ident
// within the AST and returns the rendered text of its declared type
// expression. Searches:
//  1. Enclosing FuncDecl / FuncLit parameter lists.
//  2. Top-level var/const declarations.
//
// Returns "" if no declaration is found or the declaration lacks an
// explicit type.
func (r *Resolver) lookupVarTypeText(ident *ast.Ident) string {
	want := ident.Name
	var result string
	// Find the enclosing function (FuncDecl or FuncLit) by walking the
	// path from the file to ident's position.
	var enclosing ast.Node
	ast.Inspect(r.File, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if n.Pos() <= ident.Pos() && ident.End() <= n.End() {
			switch n.(type) {
			case *ast.FuncDecl, *ast.FuncLit:
				enclosing = n
			}
			return true
		}
		return false
	})
	if enclosing != nil {
		if t := paramType(enclosing, want); t != nil {
			result = renderExpr(r.Fset, t)
			if result != "" {
				return result
			}
		}
	}
	// Top-level var declarations.
	for _, decl := range r.File.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || vs.Type == nil {
				continue
			}
			for _, n := range vs.Names {
				if n.Name == want {
					return renderExpr(r.Fset, vs.Type)
				}
			}
		}
	}
	return ""
}

// paramType returns the declared type expression for the parameter named
// name in the enclosing function, or nil if not found.
func paramType(fn ast.Node, name string) ast.Expr {
	var fields *ast.FieldList
	switch f := fn.(type) {
	case *ast.FuncDecl:
		if f.Type != nil {
			fields = f.Type.Params
		}
	case *ast.FuncLit:
		if f.Type != nil {
			fields = f.Type.Params
		}
	}
	if fields == nil {
		return nil
	}
	for _, field := range fields.List {
		for _, ident := range field.Names {
			if ident.Name == name {
				return field.Type
			}
		}
	}
	return nil
}

// renderExpr returns the source-text form of expr (e.g. "*pkg.Type").
func renderExpr(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}
