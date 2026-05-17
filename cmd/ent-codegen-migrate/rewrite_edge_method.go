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
//	q.Query<Edge>()        →  ent.Query<Entity><Edge>FromQuery(q)
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

	// Post-order traversal: rewrite inner calls before outer ones so a
	// chained pattern like q.QueryX().QueryY() is processed bottom-up.
	// The outer pass sees the already-rewritten inner (an `ent.QueryEFromQuery`
	// call) and can rebuild the chain receiver-type chain syntactically.
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
			// Receiver is *<Entity>Query — emit the FromQuery facade variant.
			// Try go/types resolution first, then fall back to a syntactic
			// chain walk for cross-package call sites where the importer
			// can't see consumer packages.
			if entity, matched := matchEdgeReceiver(r, call, descs, "Query", edge); matched {
				c.Replace(&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("ent"),
						Sel: ast.NewIdent("Query" + entity + edge + "FromQuery"),
					},
					Args: []ast.Expr{sel.X},
				})
				return true
			}
			if entity, ok := inferQueryReceiverEntity(sel.X, descs); ok {
				if edgeDescLookup(descs[entity], edge) {
					c.Replace(&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("ent"),
							Sel: ast.NewIdent("Query" + entity + edge + "FromQuery"),
						},
						Args: []ast.Expr{sel.X},
					})
					return true
				}
			}
		}
		return true
	})

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, r.Fset, r.File); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// inferQueryReceiverEntity tries to deduce the entity name behind a
// *<Entity>Query receiver expression by walking the AST when go/types
// resolution is unavailable (cross-package consumer files where the
// importer can't see the generated ent package).
//
// Recognised shapes (chained from a client):
//
//	X.<Entity>.Query()                  → entity
//	<expr>.Query<Edge>()                → descs[parent(expr)].Edges[edge].Target
//	<expr>.Where(...) | .Order(...) etc → recurse into <expr>
//	<expr>.Clone() | .Modify(...) etc   → recurse into <expr>
//
// Returns the entity name and true only when the chain unambiguously
// resolves to an entity present in descs. Returns false otherwise — a
// missed rewrite is recoverable, a wrong rewrite corrupts user code.
func inferQueryReceiverEntity(expr ast.Expr, descs Descriptors) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	switch sel.Sel.Name {
	case "Query":
		// X.<Entity>.Query() — the preceding selector field is the entity.
		inner, ok := sel.X.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		name := inner.Sel.Name
		if _, present := descs[name]; present {
			return name, true
		}
		return "", false
	default:
		// ent.Query<Parent><Edge>FromQuery(...) — rewritten form produced by
		// a prior post-order pass on an inner chain segment. Return the
		// edge's target entity (== Y in Query<X><Y>FromQuery → returns *<Y>Query).
		if x, ok := sel.X.(*ast.Ident); ok && x.Name == "ent" {
			if entity, found := entityFromFacadeName(sel.Sel.Name, descs); found {
				return entity, true
			}
		}
		// QueryEdge: walk back to the parent entity, then look up edge target.
		if edge, found := strings.CutPrefix(sel.Sel.Name, "Query"); found && edge != "" {
			parent, ok := inferQueryReceiverEntity(sel.X, descs)
			if !ok {
				return "", false
			}
			ed, ok := descs[parent]
			if !ok {
				return "", false
			}
			edesc, ok := lookupEdgeDesc(ed, edge)
			if !ok || edesc.Target == "" {
				return "", false
			}
			if _, present := descs[edesc.Target]; !present {
				return "", false
			}
			return edesc.Target, true
		}
		// Pass-through query-builder methods that preserve receiver type.
		// Be conservative: only recurse on names known to belong to the
		// generated *<Entity>Query builder so we don't follow random calls.
		if isQueryPassthrough(sel.Sel.Name) {
			return inferQueryReceiverEntity(sel.X, descs)
		}
		return "", false
	}
}

// entityFromFacadeName extracts the result entity from a rewritten facade
// call name of the form "Query<Parent><Edge>FromQuery". Splits by trying
// each known entity name as the <Parent> prefix; the remaining suffix
// (minus "FromQuery") is treated as the PascalCase edge identifier and
// resolved against descs[<Parent>].Edges to find the target entity.
func entityFromFacadeName(name string, descs Descriptors) (string, bool) {
	rest, ok := strings.CutPrefix(name, "Query")
	if !ok {
		return "", false
	}
	rest, ok = strings.CutSuffix(rest, "FromQuery")
	if !ok {
		return "", false
	}
	// Greedy: try the longest parent prefix first so "WrikeProject" doesn't
	// get split as parent="Wrike", edge="Project" if both happen to exist.
	type candidate struct{ parent, edge string }
	var candidates []candidate
	for ent := range descs {
		if strings.HasPrefix(rest, ent) && len(rest) > len(ent) {
			candidates = append(candidates, candidate{parent: ent, edge: rest[len(ent):]})
		}
	}
	// Sort by parent length desc so the most specific match wins.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && len(candidates[j].parent) > len(candidates[j-1].parent); j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	for _, c := range candidates {
		ed, ok := descs[c.parent]
		if !ok {
			continue
		}
		edesc, ok := lookupEdgeDesc(ed, c.edge)
		if !ok || edesc.Target == "" {
			continue
		}
		if _, present := descs[edesc.Target]; present {
			return edesc.Target, true
		}
	}
	return "", false
}

// isQueryPassthrough reports whether name is a *<Entity>Query method
// that returns the same *<Entity>Query type (so it preserves the chain
// receiver entity). Methods like Select/GroupBy/IDs are excluded
// because they return different builder/result types and cannot be
// followed by another QueryEdge call anyway.
func isQueryPassthrough(name string) bool {
	switch name {
	case "Where", "Order", "Limit", "Offset", "Unique", "Clone",
		"Modify", "ForUpdate", "ForShare":
		return true
	}
	return false
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
			if edgeDescLookup(ed, edge) {
				entity = ent
				return true
			}
		}
		return false
	})
	return entity, matched
}

// edgeDescLookup reports whether ed has an edge whose key matches
// the PascalCase edge identifier from a method name.
func edgeDescLookup(ed *EntityDesc, edge string) bool {
	_, ok := lookupEdgeDesc(ed, edge)
	return ok
}

// lookupEdgeDesc resolves a PascalCase edge identifier from a method
// name to its EdgeDesc. Tries both the camelCase form (lcFirst) and
// snake_case — descriptors use the schema-declared edge name, which
// may be either.
func lookupEdgeDesc(ed *EntityDesc, edge string) (EdgeDesc, bool) {
	if e, ok := ed.Edges[lcFirst(edge)]; ok {
		return e, true
	}
	if e, ok := ed.Edges[pascalToSnake(edge)]; ok {
		return e, true
	}
	return EdgeDesc{}, false
}

// pascalToSnake converts "WrikeProject" → "wrike_project". Inserts an
// underscore before any uppercase letter that follows a lowercase
// letter (or a digit), then lowercases the result. Sequences of
// uppercase letters (e.g. "IDs") are treated as a single token.
func pascalToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && isUpper(r) {
			prev := rune(s[i-1])
			if isLower(prev) || isDigit(prev) {
				b.WriteByte('_')
			} else if isUpper(prev) && i+1 < len(s) && isLower(rune(s[i+1])) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(toLower(r))
	}
	return b.String()
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }
func toLower(r rune) rune {
	if isUpper(r) {
		return r + ('a' - 'A')
	}
	return r
}
