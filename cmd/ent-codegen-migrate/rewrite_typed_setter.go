// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// RewriteTypedSetterSource rewrites consumer code that performs an interface
// assertion expecting a per-entity typed setter method (post-PR-6 those
// per-entity methods no longer exist on the generic Mutation alias).
//
// The narrowly-scoped target is the SetDeletedAt soft-delete pattern that
// consumer mixin_base.go variants use:
//
//	mx, ok := m.(interface {
//	    SetOp(ent.Op)
//	    SetDeletedAt(time.Time)
//	    WhereP(...func(*sql.Selector))
//	})
//	if !ok { return nil, fmt.Errorf("unexpected mutation type %T", m) }
//	...
//	mx.SetDeletedAt(now)
//
// Pre-PR-6 every per-entity Mutation had a generated SetDeletedAt method.
// Post-PR-6 the generic mutation only knows about its declarative SetField
// interface; the assertion above fails at runtime for every soft-deleting
// entity.
//
// Two coordinated rewrites:
//
//  1. The InterfaceType in the type-assertion: drop the SetDeletedAt method
//     entry and add a SetField(string, ent.Value) error entry so callers can
//     route through the generic API.
//  2. Every <x>.SetDeletedAt(t) call where <x> is a variable typed by an
//     assertion of the above shape: rewrite to
//     `if err := <x>.SetField("deleted_at", t); err != nil { return <zero>, fmt.Errorf(...) }`
//     The enclosing function's return signature is inspected so the generated
//     `return` matches its arity and types.
//
// Idempotent: after rewrite the InterfaceType no longer contains SetDeletedAt
// and the call site no longer holds the .SetDeletedAt(...) shape, so the
// matcher misses on subsequent passes.
//
// Generalisation note: the current scope is SetDeletedAt only. The internal
// machinery (interface-method rewrite, call-site rewrite, assertion-variable
// tracking) generalises trivially if more lost setters surface in real
// consumer code — extend setterMap and the call detection switch.
func RewriteTypedSetterSource(filename, src string, _ Descriptors, _ string) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	// Variables in this file that hold the asserted interface.
	// name → struct{} (presence-only).
	asserted := map[string]struct{}{}

	// Pass A: scan for assertions of shape `var, ok := m.(interface { ...; SetDeletedAt(time.Time); ... })`
	// and rewrite the interface's method set. Track the LHS variable name so
	// pass B can recognise calls on it.
	ast.Inspect(r.File, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE {
			return true
		}
		for i, rhs := range assign.Rhs {
			ta, ok := rhs.(*ast.TypeAssertExpr)
			if !ok || ta.Type == nil {
				continue
			}
			itf, ok := ta.Type.(*ast.InterfaceType)
			if !ok || itf.Methods == nil {
				continue
			}
			if !hasInterfaceMethod(itf, "SetDeletedAt") {
				continue
			}
			// Rewrite the interface: drop SetDeletedAt, add
			// SetField(string, ent.Value) error if not already present.
			itf.Methods = transformInterfaceMethods(itf.Methods)
			// Record the LHS variable for pass B.
			if i < len(assign.Lhs) {
				if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
					asserted[ident.Name] = struct{}{}
				}
			}
		}
		return true
	})

	if len(asserted) == 0 {
		// Nothing to rewrite (no matching assertion found).
		// Skip the printer emit to keep source byte-identical when no work
		// fired — matches the idempotency contract.
		return src, nil
	}

	// Pass B: rewrite calls <x>.SetDeletedAt(t) where x is in asserted set.
	// The transformation replaces the surrounding expression statement with
	// an if-statement that handles the SetField error.
	rewriteSetDeletedAtCalls(r.File, asserted)

	// Pass C: ensure entgo.io/ent import (for ent.Value) — only if rewrites
	// happened that emit ent.Value.
	astutil.AddImport(r.Fset, r.File, "entgo.io/ent")

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, r.Fset, r.File); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// hasInterfaceMethod reports whether the interface type literal declares a
// method with the given name (e.g. "SetDeletedAt"). Used to gate the
// transform — we only rewrite assertions that actually depend on the
// lost typed setter.
func hasInterfaceMethod(itf *ast.InterfaceType, name string) bool {
	if itf.Methods == nil {
		return false
	}
	for _, f := range itf.Methods.List {
		for _, ident := range f.Names {
			if ident.Name == name {
				return true
			}
		}
	}
	return false
}

// transformInterfaceMethods returns a new FieldList that:
//   - drops every method named SetDeletedAt
//   - appends SetField(string, ent.Value) error if not already present
//
// Preserves the relative order of surviving methods and skips fields that
// were grouped (rare in practice but supported defensively).
func transformInterfaceMethods(fl *ast.FieldList) *ast.FieldList {
	out := &ast.FieldList{Opening: fl.Opening, Closing: fl.Closing}
	hasSetField := false
	for _, f := range fl.List {
		// Walk each name; if any field has multiple names (one with
		// SetDeletedAt and one without) that's atypical, so we split.
		survivors := make([]*ast.Ident, 0, len(f.Names))
		for _, ident := range f.Names {
			if ident.Name == "SetDeletedAt" {
				continue
			}
			if ident.Name == "SetField" {
				hasSetField = true
			}
			survivors = append(survivors, ident)
		}
		if len(survivors) == 0 {
			continue
		}
		nf := *f
		nf.Names = survivors
		out.List = append(out.List, &nf)
	}
	if !hasSetField {
		out.List = append(out.List, setFieldInterfaceField())
	}
	return out
}

// setFieldInterfaceField builds the AST for
//
//	SetField(string, ent.Value) error
//
// embedded as a method in an InterfaceType. The argument list is unnamed —
// matching the way ent.Mutation declares SetField.
func setFieldInterfaceField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent("SetField")},
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Type: ast.NewIdent("string")},
				{Type: &ast.SelectorExpr{X: ast.NewIdent("ent"), Sel: ast.NewIdent("Value")}},
			}},
			Results: &ast.FieldList{List: []*ast.Field{
				{Type: ast.NewIdent("error")},
			}},
		},
	}
}

// rewriteSetDeletedAtCalls walks file looking for expression statements of
// the form `<x>.SetDeletedAt(t)` where x is in the asserted set. Each such
// statement is replaced with an if-statement that calls SetField and
// returns the wrapped error on failure. The replacement's return values
// match the enclosing function's signature (looked up by walking up the
// AST). When the enclosing function isn't found or has a return signature
// the rewriter can't render (e.g. multiple results with types it doesn't
// know how to zero-value), the rewrite is skipped — safer than guessing.
func rewriteSetDeletedAtCalls(file *ast.File, asserted map[string]struct{}) {
	// Walk each top-level FuncDecl so we know the enclosing return signature
	// when we rewrite a stmt. Nested function literals are walked recursively
	// with the nearest enclosing FuncType.
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		rewriteSetDeletedAtInBlock(fn.Body, fn.Type, asserted)
	}
}

// rewriteSetDeletedAtInBlock walks block; for each ExprStmt that matches
// <x>.SetDeletedAt(t) (with x in asserted), replace it with an if-err block.
// Nested function literals get their own FuncType for return rendering.
func rewriteSetDeletedAtInBlock(block *ast.BlockStmt, enclosing *ast.FuncType, asserted map[string]struct{}) {
	if block == nil {
		return
	}
	for i := 0; i < len(block.List); i++ {
		stmt := block.List[i]
		// Recurse into nested blocks first.
		recurseStmt(stmt, enclosing, asserted)

		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SetDeletedAt" {
			continue
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		if _, tracked := asserted[recv.Name]; !tracked {
			continue
		}
		if len(call.Args) != 1 {
			continue
		}
		replacement := buildSetFieldDeletedAtStmt(recv, call.Args[0], enclosing)
		if replacement == nil {
			continue
		}
		block.List[i] = replacement
	}
}

// recurseStmt descends into nested blocks, switches, ranges, ifs, fors and
// function literals so the rewrite reaches statements nested under control
// flow inside the soft-delete hook closure (`return ent.MutateFunc(func(...) {
// ... mx.SetDeletedAt(now) ... })`).
func recurseStmt(stmt ast.Stmt, enclosing *ast.FuncType, asserted map[string]struct{}) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		rewriteSetDeletedAtInBlock(s, enclosing, asserted)
	case *ast.IfStmt:
		rewriteSetDeletedAtInBlock(s.Body, enclosing, asserted)
		if s.Else != nil {
			if eb, ok := s.Else.(*ast.BlockStmt); ok {
				rewriteSetDeletedAtInBlock(eb, enclosing, asserted)
			} else {
				recurseStmt(s.Else, enclosing, asserted)
			}
		}
	case *ast.ForStmt:
		rewriteSetDeletedAtInBlock(s.Body, enclosing, asserted)
	case *ast.RangeStmt:
		rewriteSetDeletedAtInBlock(s.Body, enclosing, asserted)
	case *ast.SwitchStmt:
		rewriteSetDeletedAtInBlock(s.Body, enclosing, asserted)
	case *ast.TypeSwitchStmt:
		rewriteSetDeletedAtInBlock(s.Body, enclosing, asserted)
	case *ast.SelectStmt:
		rewriteSetDeletedAtInBlock(s.Body, enclosing, asserted)
	case *ast.CaseClause:
		// Case bodies are []Stmt, not BlockStmt — walk directly.
		for _, child := range s.Body {
			recurseStmt(child, enclosing, asserted)
		}
	case *ast.CommClause:
		for _, child := range s.Body {
			recurseStmt(child, enclosing, asserted)
		}
	case *ast.ExprStmt:
		// Walk inside any function literals embedded in the expression.
		ast.Inspect(s.X, func(n ast.Node) bool {
			fl, ok := n.(*ast.FuncLit)
			if !ok {
				return true
			}
			rewriteSetDeletedAtInBlock(fl.Body, fl.Type, asserted)
			return true
		})
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			ast.Inspect(rhs, func(n ast.Node) bool {
				fl, ok := n.(*ast.FuncLit)
				if !ok {
					return true
				}
				rewriteSetDeletedAtInBlock(fl.Body, fl.Type, asserted)
				return true
			})
		}
	case *ast.ReturnStmt:
		for _, expr := range s.Results {
			ast.Inspect(expr, func(n ast.Node) bool {
				fl, ok := n.(*ast.FuncLit)
				if !ok {
					return true
				}
				rewriteSetDeletedAtInBlock(fl.Body, fl.Type, asserted)
				return true
			})
		}
	}
}

// buildSetFieldDeletedAtStmt produces
//
//	if err := mx.SetField("deleted_at", t); err != nil {
//	    return <zero>, fmt.Errorf("soft-delete: set deleted_at: %w", err)
//	}
//
// where the return list matches the enclosing function signature. Returns
// nil when the signature can't be rendered (caller leaves the original
// statement in place).
func buildSetFieldDeletedAtStmt(recv *ast.Ident, value ast.Expr, enclosing *ast.FuncType) ast.Stmt {
	returnStmt := buildErrorReturn(enclosing)
	if returnStmt == nil {
		return nil
	}
	setField := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("SetField")},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"deleted_at"`}, value},
	}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("err")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{setField},
		},
		Cond: &ast.BinaryExpr{
			X:  ast.NewIdent("err"),
			Op: token.NEQ,
			Y:  ast.NewIdent("nil"),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{returnStmt}},
	}
}

// buildErrorReturn constructs the return statement that propagates the
// SetField error. The shape is "return <zero1>, <zero2>, ..., <wrapped err>"
// where:
//   - the LAST return value is fmt.Errorf("soft-delete: set deleted_at: %w", err)
//   - earlier return values are the zero-value expression for the
//     corresponding result type
//
// Requires that the enclosing function's last result type be `error` (the
// MutateFunc signature `(ent.Value, error)` guarantees this in practice).
// Returns nil when the last result isn't error or any non-last result has
// a type whose zero value the rewriter can't render literally.
func buildErrorReturn(enclosing *ast.FuncType) *ast.ReturnStmt {
	if enclosing == nil || enclosing.Results == nil || len(enclosing.Results.List) == 0 {
		// No results — emit a bare return; the caller of SetField can't
		// then propagate the error, but that's a legitimate sink in func
		// signatures we shouldn't auto-modify. Treat as "no rewrite".
		return nil
	}
	results := flattenResults(enclosing.Results)
	if len(results) == 0 {
		return nil
	}
	last := results[len(results)-1]
	if !isErrorType(last) {
		return nil
	}
	values := make([]ast.Expr, 0, len(results))
	for _, t := range results[:len(results)-1] {
		zv := zeroValueExpr(t)
		if zv == nil {
			return nil
		}
		values = append(values, zv)
	}
	values = append(values, &ast.CallExpr{
		Fun: &ast.SelectorExpr{X: ast.NewIdent("fmt"), Sel: ast.NewIdent("Errorf")},
		Args: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: `"soft-delete: set deleted_at: %w"`},
			ast.NewIdent("err"),
		},
	})
	return &ast.ReturnStmt{Results: values}
}

// flattenResults expands grouped return fields (e.g. `(a, b string)`) into a
// flat list of types — one per logical return slot — so zero-value rendering
// can pair 1:1 with positional return values.
func flattenResults(fl *ast.FieldList) []ast.Expr {
	var out []ast.Expr
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			out = append(out, f.Type)
			continue
		}
		for range f.Names {
			out = append(out, f.Type)
		}
	}
	return out
}

// isErrorType reports whether expr renders as the unqualified type "error".
// Aliased imports (rare) wouldn't match, but the soft-delete sink in
// MutateFunc returns the plain ent.Value / error pair.
func isErrorType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
}

// zeroValueExpr returns a literal expression for the zero value of t. This
// covers the cases the soft-delete sink emits — pointer/interface (nil),
// numeric (0), bool (false), string (""), named struct (T{}) — and returns
// nil for shapes the rewriter shouldn't guess at (e.g. unnamed structs,
// arrays). nil signals "give up on this rewrite" upstream.
func zeroValueExpr(t ast.Expr) ast.Expr {
	switch tt := t.(type) {
	case *ast.Ident:
		switch tt.Name {
		case "string":
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		case "bool":
			return ast.NewIdent("false")
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"float32", "float64", "byte", "rune", "uintptr":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case "any", "error":
			// any / error: zero value is nil.
			return ast.NewIdent("nil")
		default:
			// Named type from this package — its zero value is T{} when
			// it's a struct. We can't tell here, so emit nil for non-PascalCase
			// (interfaces typically have lowercase first), and T{} for the rest.
			// In practice the ent.Value sink uses ent.Value (interface) so
			// the SelectorExpr branch handles it.
			if isExported(tt.Name) {
				return &ast.CompositeLit{Type: tt}
			}
			return ast.NewIdent("nil")
		}
	case *ast.SelectorExpr:
		// Cross-package named type. ent.Value is the practical case here
		// (interface → nil); for other selectors we err on the safe side
		// and emit nil. If the type is actually a struct the consumer will
		// see a compile error; that's recoverable, unlike a silent zero
		// shaped wrong.
		return ast.NewIdent("nil")
	case *ast.StarExpr, *ast.InterfaceType, *ast.MapType, *ast.ArrayType, *ast.ChanType, *ast.FuncType:
		return ast.NewIdent("nil")
	}
	return nil
}

func isExported(name string) bool {
	if name == "" {
		return false
	}
	return strings.ToUpper(name[:1]) == name[:1] && name[0] != '_'
}
