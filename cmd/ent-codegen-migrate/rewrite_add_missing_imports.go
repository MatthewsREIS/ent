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
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// pkgImports maps a package-name alias (the LHS of "pkg.Type") to the
// full import path observed in the descriptor source files. Populated
// once at startup by LoadDescriptorImports.
//
// Earlier passes emit calls like `entbuilder.GetField[time.Time](mutation, ...)`
// and `entbuilder.EdgeIDsAs[uuid.UUID](mutation, ...)`. The original
// per-entity method carried the type implicitly in its signature, so the
// consumer file didn't need to import the type's package. After the rewrite
// the consumer file references the type by name and the import becomes
// mandatory — without it the file fails to compile with "undefined: time"
// or "undefined: uuid". This map lets the final pass add the right import.
var pkgImports = map[string]string{}

// LoadDescriptorImports scans every <entity>_mutation.go in dir and records
// each (alias → import-path) binding into pkgImports. Aliases default to
// the import path's trailing path segment; an explicit alias in the import
// declaration wins. Collisions (same alias mapping to two different paths)
// keep the shorter — std-lib imports tend to be short and the right pick.
// Idempotent.
func LoadDescriptorImports(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_mutation.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, imp := range f.Imports {
			raw, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			alias := basePathSegment(raw)
			if imp.Name != nil && imp.Name.Name != "" {
				alias = imp.Name.Name
			}
			if cur, exists := pkgImports[alias]; exists && len(cur) <= len(raw) {
				continue
			}
			pkgImports[alias] = raw
		}
	}
	return nil
}

// RewriteAddMissingImportsSource scans src for "<pkg>.<Name>" selectors
// whose LHS package alias isn't currently imported in the file but IS
// known to pkgImports. Each missing import is added.
//
// Runs as the final pass: earlier passes that emit type-args (GetField,
// EdgeIDAs, EdgeIDsAs, OldFieldAs) don't track which packages the type-args
// reference; this picks them all up uniformly.
//
// Skips selectors covered by an existing import, selectors whose LHS
// isn't a bare identifier (chained calls), and identifiers not in pkgImports.
// Idempotent.
func RewriteAddMissingImportsSource(filename, src string, _ Descriptors, _ string) (string, error) {
	if len(pkgImports) == 0 {
		return src, nil
	}
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	existing := map[string]struct{}{}
	for _, imp := range r.File.Imports {
		raw, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if imp.Name != nil && imp.Name.Name != "" {
			existing[imp.Name.Name] = struct{}{}
			continue
		}
		existing[basePathSegment(raw)] = struct{}{}
	}
	needed := map[string]string{}
	ast.Inspect(r.File, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, present := existing[ident.Name]; present {
			return true
		}
		path, known := pkgImports[ident.Name]
		if !known {
			return true
		}
		needed[ident.Name] = path
		return true
	})
	if len(needed) == 0 {
		return src, nil
	}
	for _, path := range needed {
		astutil.AddImport(r.Fset, r.File, path)
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, r.Fset, r.File); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// basePathSegment returns the trailing path segment of a slash-separated
// import path — the conventional alias when no explicit alias is set.
func basePathSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
