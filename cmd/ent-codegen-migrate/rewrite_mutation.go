// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// RewriteMutationSource parses src, applies mutation API rewrites, and
// returns the rewritten source. Used directly by tests; production callers
// go through RewritePackage which file-walks.
func RewriteMutationSource(filename, src string, descs Descriptors) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	// Walk every CallExpr. Heuristic dispatch (no type info):
	//   1. matchMutationCall parses the method name into (action, fieldOrEdge, isEdge)
	//   2. lookup field/edge in any descriptor — first match wins
	//   3. apply rewriteCall
	// Special action "where" applies without descriptor lookup.
	// False-positive risk: a non-mutation receiver with a method named like
	// "SetTitle" gets rewritten. Mitigated by the integration test (Task 16)
	// which runs `go build` on rewritten output; bad rewrites surface as
	// compile errors. Production callers can pass `-dry-run` first.
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
		// "where" doesn't need descriptor lookup — it's a pure rename.
		if action == "where" {
			newCall, _ := rewriteCall(sel.X, action, "", FieldDesc{}, EdgeDesc{}, call.Args)
			if newCall != nil {
				c.Replace(newCall)
			}
			return true
		}
		// Look up field/edge across all descriptors; first match wins.
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
			} else {
				if f, ok := ed.Fields[fieldOrEdge]; ok {
					fd = f
					matched = true
					break
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
		return "", "setEdge", lcFirst(strings.TrimSuffix(strings.TrimPrefix(name, "Set"), "ID")), true
	case strings.HasPrefix(name, "Add") && strings.HasSuffix(name, "IDs"):
		return "", "addIDs", lcPlural(strings.TrimSuffix(strings.TrimPrefix(name, "Add"), "IDs")), true
	case strings.HasPrefix(name, "Remove") && strings.HasSuffix(name, "IDs"):
		return "", "rmIDs", lcPlural(strings.TrimSuffix(strings.TrimPrefix(name, "Remove"), "IDs")), true
	case strings.HasPrefix(name, "Set"):
		return "", "set", lcFirst(strings.TrimPrefix(name, "Set")), false
	case strings.HasPrefix(name, "Old"):
		return "", "old", lcFirst(strings.TrimPrefix(name, "Old")), false
	case strings.HasPrefix(name, "Reset"):
		return "", "reset", lcFirst(strings.TrimPrefix(name, "Reset")), false
	case strings.HasPrefix(name, "Clear"):
		return "", "clear", lcFirst(strings.TrimPrefix(name, "Clear")), false
	case strings.HasSuffix(name, "Cleared"):
		return "", "cleared", lcFirst(strings.TrimSuffix(name, "Cleared")), false
	case strings.HasSuffix(name, "IDs"):
		return "", "ids", lcPlural(strings.TrimSuffix(name, "IDs")), true
	case strings.HasSuffix(name, "ID"):
		return "", "edgeID", lcFirst(strings.TrimSuffix(name, "ID")), true
	case name == "Where":
		// m.Where(ps...) → m.WhereP(ps...) — handled by a separate rewriter
		// rule below; signal "skip" by returning empty fieldOrEdge.
		return "", "where", "", false
	default:
		// Plain getter (single field) — covered by leaving entity empty;
		// the caller filters to known field names from descriptors.
		return "", "get", lcFirst(name), false
	}
}

func lcFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func lcPlural(s string) string {
	// Method "AddTeamIDs" → trim "Add"/"IDs" → "Team" (singular). Ent edge
	// names are usually lowercase plural ("teams"), so append "s" if missing.
	// Deviation from plan: plan's lcPlural was a stub that only lowercased
	// the first letter, which causes the descriptor lookup to miss the edge.
	out := lcFirst(s)
	if out != "" && !strings.HasSuffix(out, "s") {
		out += "s"
	}
	return out
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
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("ClearField")},
			Args: []ast.Expr{strLit(name)},
		}, false
	case "cleared":
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("FieldCleared")},
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
	case "where":
		// m.Where(ps...) → m.WhereP(ps...) (typed predicate.X is now func(*sql.Selector))
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ast.NewIdent("WhereP")},
			Args: args,
		}, false
	}
	return nil, false
}
