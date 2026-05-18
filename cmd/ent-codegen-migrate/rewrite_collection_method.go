// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// RewriteCollectionMethodSource parses src and rewrites the entgql
// CollectFields and Paginate methods that the post-PR-6 entgql templates
// (collection_entity.tmpl, pagination_entity.tmpl) now emit as free
// functions in the gen package — methods on type aliases are forbidden
// post-PR-6 (Bug 9):
//
//	q.CollectFields(ctx, satisfies...)          → <alias>.<Entity>QueryCollectFields(q, ctx, satisfies...)
//	q.Paginate(ctx, after, first, before, last, opts...)
//	                                            → <alias>.<Entity>QueryPaginate(q, ctx, after, first, before, last, opts...)
//
// where q is *<pkg>.<Entity>Query. Other receivers are left untouched
// (schema-DSL Paginate and unrelated user types with overlapping method
// names).
//
// <alias> resolution mirrors RewriteEdgeMethodSource: the local name the
// file binds genImportPath to, falling back to the import path's leaf
// segment (added as a fresh import) when the file does not import the
// gen package yet. When genImportPath is empty (legacy per-pass tests
// predating the flag), the rewriter falls back to the historical "ent"
// identifier.
//
// Idempotent: re-running on already-transformed code is a no-op (the
// emitted shape is a plain function call on the gen package — no
// CollectFields/Paginate method on a Query receiver remains to match).
func RewriteCollectionMethodSource(filename, src string, descs Descriptors, genImportPath string) (string, error) {
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
		methodName := sel.Sel.Name
		if methodName != "CollectFields" && methodName != "Paginate" {
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
		// Build the rewritten free-function call. Args are prepended with
		// the receiver and otherwise preserved in their original order — this
		// works uniformly for both CollectFields (ctx, satisfies...) and
		// Paginate (ctx, after, first, before, last, opts...) since both
		// signatures accept the receiver as the first parameter in the
		// free-function form.
		newArgs := append([]ast.Expr{sel.X}, call.Args...)
		emit(c, &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(alias),
				Sel: ast.NewIdent(entity + "Query" + methodName),
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

// matchQueryReceiver gates on the call's receiver static type being
// *<pkg>.<Entity>Query where <Entity> is a known descriptor entry.
// Returns the entity name and true on match. Unlike matchEdgeReceiver,
// no edge lookup is performed — the method name (CollectFields / Paginate)
// is fixed and applies to every entity's *<Entity>Query.
func matchQueryReceiver(r *Resolver, call *ast.CallExpr, descs Descriptors) (string, bool) {
	var entity string
	matched := r.ReceiverTypeMatchesPattern(call, func(typeName string) bool {
		if !strings.HasPrefix(typeName, "*") {
			return false
		}
		for ent := range descs {
			if strings.HasSuffix(typeName, "."+ent+"Query") || typeName == "*"+ent+"Query" {
				entity = ent
				return true
			}
		}
		return false
	})
	return entity, matched
}
