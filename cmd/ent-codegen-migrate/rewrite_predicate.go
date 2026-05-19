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

// predicateOps maps Go function-name suffixes to where.<Op> names.
// ID-specific helpers (IDEQ, IDIn etc.) are intentionally excluded — they
// remain as-is post-migration.
var predicateOps = map[string]string{
	"EQ":            "EQ",
	"NEQ":           "NEQ",
	"In":            "In",
	"NotIn":         "NotIn",
	"GT":            "GT",
	"GTE":           "GTE",
	"LT":            "LT",
	"LTE":           "LTE",
	"Contains":      "Contains",
	"ContainsFold":  "ContainsFold",
	"HasPrefix":     "HasPrefix",
	"HasSuffix":     "HasSuffix",
	"HasPrefixFold": "HasPrefixFold",
	"HasSuffixFold": "HasSuffixFold",
	"EqualFold":     "EqualFold",
	"IsNil":         "IsNull",
	"NotNil":        "NotNull",
}

// RewritePredicateSource parses src and rewrites <entity>.<Field><Op>(...)
// calls into where.<Op>(<entity>.Field<Field>, ...) form.
func RewritePredicateSource(filename, src string, descs Descriptors) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	// Build a lookup: entity-package-ident → entity descriptor.
	// Heuristic: package name matches lowercased entity name.
	pkgToEntity := make(map[string]*EntityDesc)
	for _, ed := range descs {
		pkgToEntity[strings.ToLower(ed.Name)] = ed
	}

	needsWhere := false
	astutil.Apply(file, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		ed, ok := pkgToEntity[pkgIdent.Name]
		if !ok {
			return true
		}
		fieldName, op := matchPredicateSuffix(sel.Sel.Name, ed)
		if fieldName == "" {
			return true
		}
		newCall := &ast.CallExpr{
			Fun: &ast.SelectorExpr{X: ast.NewIdent("where"), Sel: ast.NewIdent(op)},
			Args: append([]ast.Expr{
				&ast.SelectorExpr{X: pkgIdent, Sel: ast.NewIdent("Field" + ed.Fields[fieldName].GoName)},
			}, call.Args...),
		}
		c.Replace(newCall)
		needsWhere = true
		return true
	}, nil)

	if needsWhere {
		astutil.AddImport(fset, file, "entgo.io/ent/where")
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// matchPredicateSuffix recognises a method name like "NameEQ" → ("name", "EQ").
// Returns ("", "") if the name doesn't match a known predicate suffix or the
// inferred field name is not in the descriptor. Explicitly skips ID helpers
// (anything starting with "ID").
func matchPredicateSuffix(name string, ed *EntityDesc) (string, string) {
	if strings.HasPrefix(name, "ID") {
		return "", ""
	}
	// Try longer suffixes first (HasPrefixFold > HasPrefix > etc.)
	suffixes := []string{
		"ContainsFold", "HasPrefixFold", "HasSuffixFold", "EqualFold",
		"Contains", "HasPrefix", "HasSuffix", "NotIn", "NotNil",
		"EQ", "NEQ", "GTE", "LTE", "GT", "LT", "In", "IsNil",
	}
	for _, suf := range suffixes {
		if strings.HasSuffix(name, suf) {
			prefix := strings.TrimSuffix(name, suf)
			fieldName := strings.ToLower(prefix[:1]) + prefix[1:]
			if _, ok := ed.Fields[fieldName]; ok {
				return fieldName, predicateOps[suf]
			}
		}
	}
	return "", ""
}
