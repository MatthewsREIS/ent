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

// RewriteTypedEdgeAccessorSource rewrites the typed edge accessors that
// PR 5 removed from generated <entity>_mutation.go:
//
//	m.<Edge>ID()     →  entbuilder.EdgeIDAs[<TargetIDType>](m, "<edge>")
//	m.<Edge>IDs()    →  entbuilder.EdgeIDsAs[<TargetIDType>](m, "<edge>")
//
// Gated on the receiver being *<pkg>.<Entity>Mutation and <edge>
// matching a known edge in descs[entity].Edges.
//
// Idempotent: the rewritten shape is a generic function call on the
// entbuilder package, not a method on a Mutation receiver, so the
// matcher won't see it on a second pass.
func RewriteTypedEdgeAccessorSource(filename, src string, descs Descriptors) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	needsEntbuilder := false

	astutil.Apply(r.File, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := sel.Sel.Name

		// Match <Edge>IDs (plural) before <Edge>ID (singular) — order matters.
		if edge, ok := strings.CutSuffix(name, "IDs"); ok && edge != "" {
			if edgeName, td, matched := matchEdgeOnMutation(r, call, descs, edge); matched {
				c.Replace(buildEdgeIDsCall(sel.X, edgeName, td.TargetIDType))
				needsEntbuilder = true
				return true
			}
		}
		if edge, ok := strings.CutSuffix(name, "ID"); ok && edge != "" {
			if edgeName, td, matched := matchEdgeOnMutation(r, call, descs, edge); matched {
				c.Replace(buildEdgeIDCall(sel.X, edgeName, td.TargetIDType))
				needsEntbuilder = true
				return true
			}
		}
		return true
	}, nil)

	if needsEntbuilder {
		astutil.AddImport(r.Fset, r.File, "entgo.io/ent/runtime/entbuilder")
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, r.Fset, r.File); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// matchEdgeOnMutation gates on receiver type being a *<pkg>.<Entity>Mutation
// and returns the descriptor edge entry whose key equals lcFirst(edge).
// edge is the PascalCase substring stripped from the method name
// (e.g. "Owner" from "OwnerID", "Teams" from "TeamsIDs").
func matchEdgeOnMutation(r *Resolver, call *ast.CallExpr, descs Descriptors, edge string) (edgeName string, td EdgeDesc, ok bool) {
	matched := r.ReceiverTypeMatchesPattern(call, func(typeName string) bool {
		if !strings.HasPrefix(typeName, "*") {
			return false
		}
		for ent, ed := range descs {
			if !strings.HasSuffix(typeName, "."+ent+"Mutation") && typeName != "*"+ent+"Mutation" {
				continue
			}
			key := lcFirst(edge)
			if d, exists := ed.Edges[key]; exists {
				edgeName = key
				td = d
				return true
			}
		}
		return false
	})
	return edgeName, td, matched
}

func buildEdgeIDCall(recv ast.Expr, edge, idType string) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("EdgeIDAs")},
			Index: ast.NewIdent(idType),
		},
		Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", edge)}},
	}
}

func buildEdgeIDsCall(recv ast.Expr, edge, idType string) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("EdgeIDsAs")},
			Index: ast.NewIdent(idType),
		},
		Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", edge)}},
	}
}
