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

// RewriteEdgeFKSetFieldSource rewrites consumer call sites of the form
//
//	m.SetField("X_id", v)   →  entbuilder.SetEdgeID(m, "X", v)
//	m.Field("X_id")         →  entbuilder.EdgeID(m, "X")
//
// where "X_id" is the foreign-key column backing edge "X" in some entity's
// descriptor (i.e. an edge declared in the schema as `edge.To("X", T).Field("X_id")`).
//
// Pre-PR-6, each per-entity Mutation registered "X_id" inside the
// MutationFields map alongside scalar fields, so SetField worked. Post-PR-6
// the generic Mutation only knows about real fields, not edge FK columns
// — SetField("X_id", v) fails at runtime with "unknown <Entity> field X_id".
//
// Consumer mixins typed `m ent.Mutation` cannot call SetEdgeID directly
// (the public ent.Mutation interface omits it). entbuilder.SetEdgeID /
// EdgeID type-assert m to the generic mutation type internally; the rewrite
// uses those bridges so a single emission shape works for both interface
// and concrete receivers without per-call AST gymnastics.
//
// Resolution rules:
//   - The pass aggregates Edges across all descriptors into a single
//     fkColumn → edgeName map. If every entity that exposes "owner_id"
//     ties it to edge "owner", that mapping is used unambiguously.
//   - If "owner_id" maps to multiple distinct edge names across entities,
//     that's an ambiguity. The rewrite is skipped for that fk column with
//     no error — leaving the call site for manual review is safer than
//     guessing wrong.
//
// Receiver gating: the rewrite fires only when the receiver of SetField /
// Field is a known mutation type — either ent.Mutation (interface) or
// *<Entity>Mutation (concrete). The schema DSL declares edges with
// edge.To(...).Field("X_id"), so an unfiltered match would rewrite that
// DSL chain into entbuilder.SetEdgeID(edgeBuilder, "X", ...) and corrupt
// the schema package. Pattern-based filtering is the same approach the
// existing rewrite_mutation pass takes for the schema-DSL false-positive
// class of bugs.
//
// Idempotent: the rewritten shape is a top-level call on entbuilder, not
// a method on m, so subsequent passes don't re-match it.
func RewriteEdgeFKSetFieldSource(filename, src string, descs Descriptors, _ string) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	fkToEdge := buildFKEdgeMap(descs)
	if len(fkToEdge) == 0 {
		return src, nil
	}
	needsEntbuilder := false
	astutil.Apply(r.File, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "SetField":
			// SetField(name, value) — 2 args; name must be a string literal.
			if len(call.Args) != 2 {
				return true
			}
			fk, ok := stringArg(call.Args[0])
			if !ok {
				return true
			}
			edge, ok := fkToEdge[fk]
			if !ok {
				return true
			}
			if !isMutationReceiver(r, call, descs) {
				return true
			}
			c.Replace(buildSetEdgeIDCall(sel.X, edge, call.Args[1]))
			needsEntbuilder = true
		case "Field":
			// Field(name) — 1 arg; name must be a string literal.
			if len(call.Args) != 1 {
				return true
			}
			fk, ok := stringArg(call.Args[0])
			if !ok {
				return true
			}
			edge, ok := fkToEdge[fk]
			if !ok {
				return true
			}
			if !isMutationReceiver(r, call, descs) {
				return true
			}
			c.Replace(buildEdgeIDHelperCall(sel.X, edge))
			needsEntbuilder = true
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

// isMutationReceiver reports whether call's receiver is statically typed as
// either an ent.Mutation interface or a *<Entity>Mutation matching one of
// the loaded descriptors. The pattern matches:
//
//   - "entgo.io/ent.Mutation" — qualified ent.Mutation interface (consumer
//     mixins receive `m ent.Mutation` in MutateFunc callbacks)
//   - "ent.Mutation"          — short rendering used by some type checkers
//   - "*<pkg>.<Entity>Mutation" or "*<Entity>Mutation" where Entity is in descs
//
// False on anything else — including unresolved types and the schema DSL's
// edge.To(...).Field("x_id") chain whose receiver is *assocBuilder, not a
// mutation. False-on-uncertain matches the "safer to miss than corrupt"
// convention every other pass follows.
func isMutationReceiver(r *Resolver, call *ast.CallExpr, descs Descriptors) bool {
	return r.ReceiverTypeMatchesPattern(call, func(typeName string) bool {
		// ent.Mutation interface (qualified or short).
		if typeName == "entgo.io/ent.Mutation" || typeName == "ent.Mutation" {
			return true
		}
		// *<Entity>Mutation for a known entity.
		if !strings.HasPrefix(typeName, "*") {
			return false
		}
		for ent := range descs {
			if strings.HasSuffix(typeName, "."+ent+"Mutation") || typeName == "*"+ent+"Mutation" {
				return true
			}
		}
		return false
	})
}

// buildFKEdgeMap aggregates EdgeDesc.Field → edge-name across every entity
// descriptor. If the same FK column maps to different edge names in
// different entities, the entry is left out (ambiguous: rewriter skips
// to avoid wrong guesses). Per-entity consistency is the common case
// because schema authors typically name FK columns after the edge.
func buildFKEdgeMap(descs Descriptors) map[string]string {
	candidates := map[string]map[string]struct{}{}
	for _, ed := range descs {
		if ed == nil {
			continue
		}
		for edgeName, eg := range ed.Edges {
			if eg.Field == "" {
				continue
			}
			if candidates[eg.Field] == nil {
				candidates[eg.Field] = map[string]struct{}{}
			}
			candidates[eg.Field][edgeName] = struct{}{}
		}
	}
	out := map[string]string{}
	for fk, edges := range candidates {
		if len(edges) != 1 {
			continue
		}
		for e := range edges {
			out[fk] = e
		}
	}
	return out
}

func stringArg(expr ast.Expr) (string, bool) {
	bl, ok := expr.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	// Strip the surrounding quotes; ignore raw-string escaping subtleties
	// (FK column names are plain ASCII identifiers in practice).
	if len(bl.Value) < 2 {
		return "", false
	}
	return bl.Value[1 : len(bl.Value)-1], true
}

func buildSetEdgeIDCall(recv ast.Expr, edge string, value ast.Expr) ast.Expr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("SetEdgeID")},
		Args: []ast.Expr{recv, strLit(edge), value},
	}
}

func buildEdgeIDHelperCall(recv ast.Expr, edge string) ast.Expr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("EdgeID")},
		Args: []ast.Expr{recv, strLit(edge)},
	}
}

func strLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", s)}
}
