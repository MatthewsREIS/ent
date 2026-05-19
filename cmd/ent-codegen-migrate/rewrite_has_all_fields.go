// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"

	"golang.org/x/tools/go/ast/astutil"
)

// RewriteHasAllFieldsSource parses src and rewrites the post-PR-6
// HasAllFields helper that the gqlgen-collection template now emits as a
// free function on the gen package — methods on type aliases are
// forbidden post-PR-6 (Bug 9):
//
//	q.HasAllFields(fields...)  →  <alias>.<Entity>HasAllFields(q, fields...)
//
// where q is *<pkg>.<Entity>Query. Other receivers are left untouched
// (unrelated user types with overlapping method names).
//
// Note the emitted name lacks the "Query" infix that the sibling
// CollectFields / Paginate rewrites use: gqlgen's collection template
// emits "<Entity>HasAllFields" (no "Query" segment), and the rewriter
// matches that exact identifier so the generated facade resolves.
//
// <alias> resolution mirrors RewriteEdgeMethodSource and
// RewriteCollectionMethodSource: the local name the file binds
// genImportPath to, falling back to the import path's leaf segment
// (added as a fresh import) when the file does not import the gen
// package yet. When genImportPath is empty (legacy per-pass tests
// predating the flag), the rewriter falls back to the historical "ent"
// identifier.
//
// Idempotent: re-running on already-transformed code is a no-op (the
// emitted shape is a plain function call on the gen package — no
// HasAllFields method on a Query receiver remains to match).
func RewriteHasAllFieldsSource(filename, src string, descs Descriptors, genImportPath string) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	alias, addImport := resolveGenAlias(r, genImportPath)

	rewrote := false
	emit := func(c *astutil.Cursor, expr ast.Expr) {
		c.Replace(expr)
		rewrote = true
	}

	astutil.Apply(r.File, nil, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "HasAllFields" {
			return true
		}
		// Resolve the receiver to an entity. Prefer go/types; fall back to
		// the syntactic chain walker for cross-package consumer files where
		// the importer can't see the generated ent package.
		entity, matched := matchQueryReceiver(r, call, descs)
		if !matched {
			if e, ok := inferQueryReceiverEntityInFile(sel.X, r.File, descs, alias); ok {
				entity = e
				matched = true
			}
		}
		if !matched {
			return true
		}
		// Build the rewritten free-function call. The receiver becomes the
		// first argument; the variadic call.Ellipsis is preserved so a
		// `q.HasAllFields(fields...)` spread call rewrites to
		// `<alias>.<Entity>HasAllFields(q, fields...)` and not the
		// homonymous `<Entity>HasAllFields(q, fields)` (slice-as-arg) form
		// which wouldn't match the variadic signature.
		newArgs := append([]ast.Expr{sel.X}, call.Args...)
		emit(c, &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(alias),
				Sel: ast.NewIdent(entity + "HasAllFields"),
			},
			Args:     newArgs,
			Ellipsis: call.Ellipsis,
		})
		return true
	})

	if rewrote && addImport && genImportPath != "" {
		astutil.AddImport(r.Fset, r.File, genImportPath)
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, r.Fset, r.File); err != nil {
		return "", err
	}
	return buf.String(), nil
}
