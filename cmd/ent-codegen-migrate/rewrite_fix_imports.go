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
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixImportsConfig configures the fix-imports pass.
type FixImportsConfig struct {
	// ModuleRoot is the filesystem path to the Go module whose generated
	// packages are the rebinding target. The pass uses go/packages.Load with
	// Dir=ModuleRoot to enumerate symbols.
	ModuleRoot string
	// EntRootPaths is the set of import-path prefixes whose imports are
	// eligible for rebinding (e.g. "example.com/foo/internal/ent/gen/").
	// Imports outside these prefixes are never rewritten — this protects
	// against accidental misrouting when a symbol name collides with a
	// third-party package's export.
	EntRootPaths []string
}

// symbolIndex maps exported identifier → set of package paths under the
// EntRootPaths prefixes that export that identifier.
type symbolIndex map[string][]string

// buildSymbolIndex loads ModuleRoot via go/packages and indexes exported
// names of every package whose path matches one of cfg.EntRootPaths.
func buildSymbolIndex(cfg FixImportsConfig) (symbolIndex, error) {
	pkgCfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: cfg.ModuleRoot,
	}
	patterns := make([]string, 0, len(cfg.EntRootPaths))
	for _, p := range cfg.EntRootPaths {
		patterns = append(patterns, p+"...")
	}
	pkgs, err := packages.Load(pkgCfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %w", err)
	}
	idx := make(symbolIndex)
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if obj == nil || !obj.Exported() {
				continue
			}
			idx[name] = append(idx[name], pkg.PkgPath)
		}
	}
	for k := range idx {
		sort.Strings(idx[k])
	}
	return idx, nil
}

// importIsUnderEntRoot reports whether path has any of cfg.EntRootPaths as a
// prefix.
func importIsUnderEntRoot(p string, cfg FixImportsConfig) bool {
	for _, prefix := range cfg.EntRootPaths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// importResolves reports whether p can be loaded by go/packages from
// ModuleRoot.
func importResolves(p string, cfg FixImportsConfig) bool {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName,
		Dir:  cfg.ModuleRoot,
	}, p)
	if err != nil {
		return false
	}
	for _, pkg := range pkgs {
		if pkg.PkgPath == p && len(pkg.Errors) == 0 {
			return true
		}
	}
	return false
}

// RewriteFixImportsSource parses src and rewrites broken imports under
// cfg.EntRootPaths to the new package paths discovered via symbol-index
// lookup. Imports outside cfg.EntRootPaths are never touched.
//
// Behavior per broken import:
//   - Collect every <localname>.Sym selector in the file where <localname>
//     matches the import's local name (alias or last path segment).
//   - For each unique Sym, look up in the symbol index.
//   - If exactly one package exports all the symbols used, rewrite the
//     import path.
//   - Otherwise (ambiguous or no match), leave the import alone and log.
func RewriteFixImportsSource(filename, src string, cfg FixImportsConfig) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	idx, err := buildSymbolIndex(cfg)
	if err != nil {
		return "", err
	}

	type pending struct {
		oldPath, newPath string
	}
	var rewrites []pending
	for _, imp := range file.Imports {
		oldPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if !importIsUnderEntRoot(oldPath, cfg) {
			continue
		}
		if importResolves(oldPath, cfg) {
			continue
		}
		local := lastSegment(oldPath)
		if imp.Name != nil && imp.Name.Name != "" && imp.Name.Name != "_" {
			local = imp.Name.Name
		}
		symbols := collectSelectorsFor(file, local)
		if len(symbols) == 0 {
			continue
		}
		candidate := intersectCandidates(symbols, idx)
		if len(candidate) != 1 {
			fmt.Printf("fix-imports: %s: import %q used %d symbol(s); %d candidate(s) — skipping\n",
				filename, oldPath, len(symbols), len(candidate))
			continue
		}
		rewrites = append(rewrites, pending{oldPath: oldPath, newPath: candidate[0]})
	}

	// No rewrites needed — return original source unchanged to avoid spurious
	// formatting changes from printer.Fprint.
	if len(rewrites) == 0 {
		return src, nil
	}

	for _, r := range rewrites {
		if !astutil.RewriteImport(fset, file, r.oldPath, r.newPath) {
			fmt.Printf("fix-imports: %s: failed to rewrite %q → %q\n", filename, r.oldPath, r.newPath)
			continue
		}
		oldLocal := lastSegment(r.oldPath)
		newLocal := lastSegment(r.newPath)
		if oldLocal != newLocal {
			renameSelectorsFor(file, oldLocal, newLocal)
		}
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// renameSelectorsFor walks file and renames all uses of oldLocal as a
// selector qualifier (e.g. oldLocal.X → newLocal.X).
func renameSelectorsFor(file *ast.File, oldLocal, newLocal string) {
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name == oldLocal {
			id.Name = newLocal
		}
		return true
	})
}

// lastSegment returns the final slash-separated element of p.
func lastSegment(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return p
	}
	return p[i+1:]
}

// collectSelectorsFor walks file and returns selector names in expressions
// of the form local.X.
func collectSelectorsFor(file *ast.File, local string) []string {
	seen := map[string]struct{}{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name != local {
			return true
		}
		seen[sel.Sel.Name] = struct{}{}
		return true
	})
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// intersectCandidates returns the package paths that export every symbol
// in symbols.
func intersectCandidates(symbols []string, idx symbolIndex) []string {
	if len(symbols) == 0 {
		return nil
	}
	candidates := append([]string(nil), idx[symbols[0]]...)
	for _, s := range symbols[1:] {
		next := idx[s]
		filtered := candidates[:0]
		for _, c := range candidates {
			for _, n := range next {
				if c == n {
					filtered = append(filtered, c)
					break
				}
			}
		}
		candidates = filtered
		if len(candidates) == 0 {
			break
		}
	}
	sort.Strings(candidates)
	return candidates
}
