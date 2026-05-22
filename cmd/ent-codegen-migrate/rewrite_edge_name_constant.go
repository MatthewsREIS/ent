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
	"path"
	"sort"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// edgeMethodNames is the set of mutation methods retyped to take EdgeName.
// AddedIDs and RemovedIDs are NOT in this set — they're part of the upstream
// ent.Mutation interface and remain string-typed.
var edgeMethodNames = map[string]struct{}{
	"AddEdgeIDs":     {},
	"RemoveEdgeIDs":  {},
	"SetEdgeID":      {},
	"ClearEdge":      {},
	"EdgeID":         {},
	"EdgeIDs":        {},
	"RemovedEdgeIDs": {},
	"EdgeCleared":    {},
	"ResetEdge":      {},
}

// RewriteEdgeNameConstantSource parses src and rewrites string-literal first
// arguments to the 9 retyped edge methods (AddEdgeIDs, RemoveEdgeIDs, etc.)
// into qualified per-entity edge constants (<entity-pkg>.Edge<PascalCase>).
//
// Receiver entity resolution is syntactic: any call m.<EdgeMethod>(...) where
// m's declared type is "<Entity>Mutation" or "*<Entity>Mutation" (possibly
// package-qualified) and <Entity> is in descs.
//
// Non-literal first arguments are skipped — the post-signature-change compile
// error will flag them for human review. Unknown edges (not in descs[ed].Edges)
// are also skipped.
//
// genImportPath is the consumer's generated package import (e.g.
// "example.com/ent/gen"). Per-entity sub-packages are derived as
// "<genImportPath>/<lower(entityName)>".
func RewriteEdgeNameConstantSource(filename, src string, descs Descriptors, genImportPath string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	// Map mutation-type-name → entity descriptor. Convention: ent generates
	// "<Entity>Mutation" as the public alias.
	mutToEntity := make(map[string]*EntityDesc, len(descs))
	for _, ed := range descs {
		mutToEntity[ed.Name+"Mutation"] = ed
	}

	// entityFromExpr extracts the entity from a type expression of the form
	// *X, *pkg.X, X, or pkg.X — returning the descriptor when X matches
	// "<Entity>Mutation".
	entityFromExpr := func(t ast.Expr) *EntityDesc {
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X
		}
		switch n := t.(type) {
		case *ast.Ident:
			return mutToEntity[n.Name]
		case *ast.SelectorExpr:
			return mutToEntity[n.Sel.Name]
		}
		return nil
	}

	// Build flat name→entity map by walking function decls + func literals.
	// Includes method receivers and function parameters.
	flat := make(map[string]*EntityDesc)
	bindFromFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, f := range fields.List {
			ed := entityFromExpr(f.Type)
			if ed == nil {
				continue
			}
			for _, name := range f.Names {
				flat[name.Name] = ed
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			bindFromFields(node.Recv)
			if node.Type != nil {
				bindFromFields(node.Type.Params)
			}
		case *ast.FuncLit:
			if node.Type != nil {
				bindFromFields(node.Type.Params)
			}
		}
		return true
	})

	neededImports := make(map[string]struct{})

	astutil.Apply(file, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, ok := edgeMethodNames[sel.Sel.Name]; !ok {
			return true
		}
		recv, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		ed, ok := flat[recv.Name]
		if !ok {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		edgeName := strings.Trim(lit.Value, `"`)
		if _, ok := ed.Edges[edgeName]; !ok {
			return true
		}
		entityPkg := strings.ToLower(ed.Name)
		constIdent := "Edge" + pascalCaseFromSnake(edgeName)
		call.Args[0] = &ast.SelectorExpr{
			X:   ast.NewIdent(entityPkg),
			Sel: ast.NewIdent(constIdent),
		}
		neededImports[path.Join(genImportPath, entityPkg)] = struct{}{}
		return true
	}, nil)

	// Add imports in sorted order for deterministic output.
	imports := make([]string, 0, len(neededImports))
	for imp := range neededImports {
		imports = append(imports, imp)
	}
	sort.Strings(imports)
	for _, imp := range imports {
		astutil.AddImport(fset, file, imp)
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// pascalCaseFromSnake turns "primary_contact" → "PrimaryContact".
// Input is the canonical ent edge name; output is the Go identifier suffix.
func pascalCaseFromSnake(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}
