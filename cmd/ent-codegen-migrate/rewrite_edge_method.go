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

// RewriteEdgeMethodSource parses src and rewrites the cross-entity edge
// methods that PR 6 lifted out of per-entity sub-packages into the root
// facade:
//
//	q.With<Edge>(opts...)  →  ent.With<Entity><Edge>(q, opts...)
//	c.Query<Edge>(t)       →  ent.Query<Entity><Edge>(c, t)
//
// where q is *<pkg>.<Entity>Query and c is *<pkg>.<Entity>Client.
// Other receivers are left untouched (schema-DSL and unrelated user
// types with overlapping method names).
//
// Idempotent: re-running on already-transformed code is a no-op (the
// new shapes are plain function calls — no With/Query-prefixed method
// on a Query/Client receiver remains to match).
func RewriteEdgeMethodSource(filename, src string, descs Descriptors) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	astutil.Apply(r.File, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		methodName := sel.Sel.Name

		if edge, ok := strings.CutPrefix(methodName, "With"); ok && edge != "" {
			if entity, matched := matchEdgeReceiver(r, call, descs, "Query", edge); matched {
				newArgs := append([]ast.Expr{sel.X}, call.Args...)
				c.Replace(&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("ent"),
						Sel: ast.NewIdent("With" + entity + edge),
					},
					Args: newArgs,
				})
				return true
			}
		}
		if edge, ok := strings.CutPrefix(methodName, "Query"); ok && edge != "" {
			if entity, matched := matchEdgeReceiver(r, call, descs, "Client", edge); matched {
				newArgs := append([]ast.Expr{sel.X}, call.Args...)
				c.Replace(&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("ent"),
						Sel: ast.NewIdent("Query" + entity + edge),
					},
					Args: newArgs,
				})
				return true
			}
		}
		return true
	}, nil)

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, r.Fset, r.File); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// matchEdgeReceiver inspects call's receiver type. If it resolves to
// *<pkg>.<Entity><suffix> and edge (lowercased) is a known edge on
// <Entity>, returns the entity name and true.
//
// suffix is "Query" or "Client". edge is the pascal-case substring
// stripped from the method name (e.g. "Teams" from "WithTeams").
func matchEdgeReceiver(r *Resolver, call *ast.CallExpr, descs Descriptors, suffix, edge string) (string, bool) {
	var entity string
	matched := r.ReceiverTypeMatchesPattern(call, func(typeName string) bool {
		if !strings.HasPrefix(typeName, "*") {
			return false
		}
		for ent, ed := range descs {
			if !strings.HasSuffix(typeName, "."+ent+suffix) && typeName != "*"+ent+suffix {
				continue
			}
			edgeKey := lcFirst(edge)
			if _, ok := ed.Edges[edgeKey]; ok {
				entity = ent
				return true
			}
		}
		return false
	})
	return entity, matched
}
