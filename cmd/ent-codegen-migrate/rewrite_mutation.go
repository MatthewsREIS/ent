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

	"github.com/go-openapi/inflect"
	"golang.org/x/tools/go/ast/astutil"
)

var pluralRules = inflect.NewDefaultRuleset()

// RewriteMutationSource parses src, applies mutation API rewrites, and
// returns the rewritten source. Used directly by tests; production callers
// go through RewritePackage which file-walks.
//
// Rewrites are gated on the receiver's static type being a
// *<pkg>.<Entity>Mutation — schema-DSL types with coincidentally-named
// methods are left untouched. False-on-uncertain is the safe default:
// a missed rewrite is recoverable by a follow-up tool run; a wrong
// rewrite corrupts consumer code.
//
// Idempotent: re-running on already-transformed code produces the same
// output (the new SetField/GetField/EdgeIDAs call shapes don't match
// matchMutationCall patterns).
func RewriteMutationSource(filename, src string, descs Descriptors) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", err
	}
	fset := r.Fset
	file := r.File

	needsEntbuilder := false
	astutil.Apply(file, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		_, action, fieldOrEdge, isEdge := matchMutationCall(sel.Sel.Name)
		if action == "" {
			return true
		}
		// Receiver-type gate: rewrite only if the receiver is a
		// *<pkg>.<Entity>Mutation for some Entity in descs. Schema-DSL
		// receivers (EdgeBuilder, FieldBuilder, custom helpers) are
		// skipped — this was the bug that corrupted consumer schema files.
		if !isMutationReceiver(r, call, descs) {
			return true
		}
		// "where" doesn't need descriptor lookup — it's a pure rename.
		if action == "where" {
			newCall, _ := rewriteCall(sel.X, action, "", FieldDesc{}, EdgeDesc{}, call.Args)
			if newCall != nil {
				c.Replace(newCall)
			}
			return true
		}
		// Look up field/edge across all descriptors; first match wins.
		// For "cleared" action the name may refer to either a field or an edge
		// (e.g. m.OwnerCleared() where "owner" is an edge). Try both; if we
		// match an edge, upgrade isEdge so rewriteCall uses EdgeCleared.
		var (
			fd      FieldDesc
			edgeD   EdgeDesc
			matched bool
		)
		for _, ed := range descs {
			if isEdge {
				if e, ok := ed.Edges[fieldOrEdge]; ok {
					edgeD = e
					matched = true
					break
				}
				// SetXID with no matching edge "<x>" — could be a typed
				// field setter for an "<x>_id" column (consumer schemas
				// frequently define raw FK columns without a declared
				// edge). Fall back to a field lookup and reclassify.
				if action == "setEdge" {
					fieldName := fieldOrEdge + "_id"
					if f, ok := ed.Fields[fieldName]; ok {
						fd = f
						fieldOrEdge = fieldName
						action = "set"
						isEdge = false
						matched = true
						break
					}
				}
				// <X>ID getter: schemas frequently expose a raw FK column
				// as field "<x>_id" rather than declaring an edge. Reclassify
				// as a plain field getter so we emit GetField[T](...).
				if action == "edgeID" {
					fieldName := fieldOrEdge + "_id"
					if f, ok := ed.Fields[fieldName]; ok {
						fd = f
						fieldOrEdge = fieldName
						action = "get"
						isEdge = false
						matched = true
						break
					}
				}
			} else {
				if f, ok := ed.Fields[fieldOrEdge]; ok {
					fd = f
					matched = true
					break
				}
				// "cleared" is ambiguous: also look in edges.
				if action == "cleared" {
					if e, ok := ed.Edges[fieldOrEdge]; ok {
						edgeD = e
						isEdge = true
						matched = true
						break
					}
				}
				// Clear<X>() also targets edges (e.g. ClearProperties on
				// an edge named "properties"). The generic mutation API
				// for edges is ClearEdge("<name>"); the "clear" case in
				// rewriteCall dispatches on edgeD to pick the right method.
				if action == "clear" {
					if e, ok := ed.Edges[fieldOrEdge]; ok {
						edgeD = e
						isEdge = true
						matched = true
						break
					}
				}
			}
		}
		if !matched {
			return true
		}
		newCall, addImport := rewriteCall(sel.X, action, fieldOrEdge, fd, edgeD, call.Args)
		if newCall == nil {
			return true
		}
		c.Replace(newCall)
		needsEntbuilder = needsEntbuilder || addImport
		return true
	}, nil)

	if needsEntbuilder {
		astutil.AddImport(fset, file, "entgo.io/ent/runtime/entbuilder")
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// isMutationReceiver reports whether the receiver of call has a static
// type matching any *<pkg>.<Entity>Mutation in descs. Returns false on
// uncertain (no type info) — safer to skip than to corrupt.
func isMutationReceiver(r *Resolver, call *ast.CallExpr, descs Descriptors) bool {
	return r.ReceiverTypeMatchesPattern(call, func(typeName string) bool {
		// typeName looks like "*x.TaskMutation" or "*example.com/foo.TaskMutation".
		// Match the trailing "<Entity>Mutation" against descriptor keys.
		if !strings.HasPrefix(typeName, "*") {
			return false
		}
		for ent := range descs {
			if strings.HasSuffix(typeName, "."+ent+"Mutation") {
				return true
			}
		}
		return false
	})
}

// matchMutationCall recognises method names like "SetTitle", "Title",
// "OldTitle", "AddTeamIDs" and returns the parsed pieces. The entity name
// can't be inferred from method alone — caller resolves via the receiver
// expression's type (which we approximate via the descriptors lookup).
//
// Returns ("", "", "", false) if the method doesn't match a known pattern.
//
// Action vocabulary:
//
//	"set"     → SetX(v)
//	"get"     → X() → typed pair
//	"old"     → OldX(ctx) → typed pair
//	"reset"   → ResetX()
//	"clear"   → ClearX() or ClearXEdge
//	"cleared" → XCleared()
//	"addIDs"  → AddXIDs
//	"rmIDs"   → RemoveXIDs
//	"ids"     → XIDs / XID
//	"setEdge" → SetXID
//
// fieldOrEdge is the schema name (lowercase singular). The descriptors map
// is keyed this way.
//
// isEdge true when the method shape is edge-shaped (AddXIDs etc.).
//
// Note: the entity returned is best-effort; the actual entity is determined
// by the caller via type info (or in our minimal approach, by matching the
// receiver's apparent type name via NamesSeen in a future enhancement).
// For Tasks 14-15's scope, we apply the rewrite if ANY known descriptor
// has the field name — the test suite covers this.
func matchMutationCall(name string) (entity, action, fieldOrEdge string, isEdge bool) {
	switch {
	case strings.HasPrefix(name, "Set") && strings.HasSuffix(name, "ID") && len(name) > 5:
		// SetTeamID etc. → unique-edge set
		return "", "setEdge", pascalToSnake(strings.TrimSuffix(strings.TrimPrefix(name, "Set"), "ID")), true
	case strings.HasPrefix(name, "Add") && strings.HasSuffix(name, "IDs"):
		return "", "addIDs", pluralizeSnake(pascalToSnake(strings.TrimSuffix(strings.TrimPrefix(name, "Add"), "IDs"))), true
	case strings.HasPrefix(name, "Removed") && strings.HasSuffix(name, "IDs"):
		// Removed<Edge>IDs() → returns IDs removed from <Edge> (no args).
		// Distinct from Remove<Edge>IDs(ids...) which mutates the edge.
		// Place this case BEFORE the Remove<Edge>IDs(...) one because
		// "Removed" has "Remove" as a prefix and the switch is order-sensitive.
		return "", "removedIDs", pluralizeSnake(pascalToSnake(strings.TrimSuffix(strings.TrimPrefix(name, "Removed"), "IDs"))), true
	case strings.HasPrefix(name, "Remove") && strings.HasSuffix(name, "IDs"):
		return "", "rmIDs", pluralizeSnake(pascalToSnake(strings.TrimSuffix(strings.TrimPrefix(name, "Remove"), "IDs"))), true
	case strings.HasPrefix(name, "Set"):
		return "", "set", pascalToSnake(strings.TrimPrefix(name, "Set")), false
	case strings.HasPrefix(name, "Old"):
		return "", "old", pascalToSnake(strings.TrimPrefix(name, "Old")), false
	case strings.HasPrefix(name, "Reset"):
		return "", "reset", pascalToSnake(strings.TrimPrefix(name, "Reset")), false
	case strings.HasPrefix(name, "Clear"):
		return "", "clear", pascalToSnake(strings.TrimPrefix(name, "Clear")), false
	case strings.HasSuffix(name, "Cleared"):
		return "", "cleared", pascalToSnake(strings.TrimSuffix(name, "Cleared")), false
	case strings.HasSuffix(name, "IDs"):
		return "", "ids", pluralizeSnake(pascalToSnake(strings.TrimSuffix(name, "IDs"))), true
	case strings.HasSuffix(name, "ID"):
		return "", "edgeID", pascalToSnake(strings.TrimSuffix(name, "ID")), true
	case name == "Where":
		// m.Where(ps...) → m.WhereP(ps...) — handled by a separate rewriter
		// rule below; signal "skip" by returning empty fieldOrEdge.
		return "", "where", "", false
	default:
		// Plain getter (single field) — covered by leaving entity empty;
		// the caller filters to known field names from descriptors.
		return "", "get", pascalToSnake(name), false
	}
}

func lcFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// pluralizeSnake pluralizes a snake_case name using the same inflect
// ruleset that entc/gen uses, so the inverted edge name matches the
// schema's declared edge key (e.g. "property" → "properties",
// "wrike_folder" → "wrike_folders"). The previous naive
// "append 's' unless ends in s" form mishandled English plurals like
// y→ies and broke lookups for edges such as Properties.
func pluralizeSnake(s string) string {
	if s == "" {
		return s
	}
	return pluralRules.Pluralize(s)
}

// rewriteCall builds the new call AST. Returns nil if rewrite doesn't apply.
// addImport indicates whether entbuilder import is required.
func rewriteCall(recv ast.Expr, action, name string, fd FieldDesc, edgeD EdgeDesc, args []ast.Expr) (newCall ast.Expr, addImport bool) {
	strLit := func(s string) *ast.BasicLit {
		return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", s)}
	}
	switch action {
	case "set":
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("SetField")},
			Args: append([]ast.Expr{strLit(name)}, args...),
		}, false
	case "get":
		return &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("GetField")},
				Index: ast.NewIdent(fd.Type),
			},
			Args: []ast.Expr{recv, strLit(name)},
		}, true
	case "old":
		// args contains ctx
		return &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("OldFieldAs")},
				Index: ast.NewIdent(fd.Type),
			},
			Args: append(append([]ast.Expr{}, args...), recv, strLit(name)),
		}, true
	case "reset":
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("ResetField")},
			Args: []ast.Expr{strLit(name)},
		}, false
	case "clear":
		// Edges: ClearEdge("<name>"); fields: ClearField("<name>"). EdgeDesc
		// is populated only when the descriptor lookup matched an edge.
		method := "ClearField"
		if edgeD.TargetIDType != "" || edgeD.Cardinality != "" {
			method = "ClearEdge"
		}
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent(method)},
			Args: []ast.Expr{strLit(name)},
		}, false
	case "cleared":
		// When the name resolved to an edge (isEdge was upgraded in the
		// lookup loop), use EdgeCleared; otherwise use FieldCleared.
		method := "FieldCleared"
		if edgeD.TargetIDType != "" || edgeD.Cardinality != "" {
			method = "EdgeCleared"
		}
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent(method)},
			Args: []ast.Expr{strLit(name)},
		}, false
	case "addIDs":
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("AddEdgeIDs")},
			Args: append([]ast.Expr{strLit(name)}, &ast.CallExpr{
				Fun: &ast.IndexExpr{
					X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("ToAny")},
					Index: ast.NewIdent(edgeD.TargetIDType),
				},
				Args: args,
			}),
		}, true
	case "rmIDs":
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("RemoveEdgeIDs")},
			Args: append([]ast.Expr{strLit(name)}, &ast.CallExpr{
				Fun: &ast.IndexExpr{
					X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("ToAny")},
					Index: ast.NewIdent(edgeD.TargetIDType),
				},
				Args: args,
			}),
		}, true
	case "setEdge":
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("SetEdgeID")},
			Args: append([]ast.Expr{strLit(name)}, args...),
		}, false
	case "edgeID":
		return &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("EdgeIDAs")},
				Index: ast.NewIdent(edgeD.TargetIDType),
			},
			Args: []ast.Expr{recv, strLit(name)},
		}, true
	case "ids":
		return &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("EdgeIDsAs")},
				Index: ast.NewIdent(edgeD.TargetIDType),
			},
			Args: []ast.Expr{recv, strLit(name)},
		}, true
	case "removedIDs":
		// m.Removed<Edge>IDs() → m.RemovedEdgeIDs("<edge>"). Returns []any;
		// callers wanting a typed slice should iterate and type-assert
		// (no typed helper exists for the removed-IDs view).
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("RemovedEdgeIDs")},
			Args: []ast.Expr{strLit(name)},
		}, false
	case "where":
		// m.Where(ps...) → m.WhereP(ps...) (typed predicate.X is now func(*sql.Selector))
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("WhereP")},
			Args: args,
		}, false
	}
	return nil, false
}
