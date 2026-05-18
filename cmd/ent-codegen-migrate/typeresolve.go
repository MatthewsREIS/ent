// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"strconv"
	"sync"

	"golang.org/x/tools/go/packages"
)

// pkgCache stores go/packages.Load results keyed by absolute directory.
// A nil value indicates a previously-failed load (cached so we don't retry
// the slow `go list` walk per file in a doomed package).
var pkgCache sync.Map // map[string]*packages.Package

// loadPackageForFile returns the type-checked package containing filename,
// loaded via go/packages (module-aware, sees cross-package types from
// source). Results are cached by directory.
//
// The gc importer that go/types ships with can't resolve cross-package
// types from source — it expects pre-compiled .a files. That's why the
// pre-go/packages NewResolver path's Info.Types is empty for receivers
// like `gen.FromContext(ctx).Escrow.QueryRecordType(_m)` where `Escrow`
// is from a consumer package the importer never sees. go/packages
// shells out to `go list` and `go build` to type-check the whole module,
// so cross-package receivers resolve correctly.
//
// Returns nil if loading failed (caller falls back to single-file mode).
func loadPackageForFile(filename string) *packages.Package {
	abs, err := filepath.Abs(filepath.Dir(filename))
	if err != nil {
		return nil
	}
	if cached, ok := pkgCache.Load(abs); ok {
		if cached == nil {
			return nil
		}
		return cached.(*packages.Package)
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedTypesSizes,
		Dir: abs,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil || len(pkgs) == 0 || pkgs[0].TypesInfo == nil {
		pkgCache.Store(abs, (*packages.Package)(nil))
		return nil
	}
	pkgCache.Store(abs, pkgs[0])
	return pkgs[0]
}

// buildPosTypeMap indexes pkg.TypesInfo.Types by source position, so a
// freshly-parsed AST (from rewriter `src`) can look up cross-package types
// by matching positions against the loaded package's authoritative AST.
//
// Position matching is exact: if the rewriter's `src` is byte-identical to
// disk (typical on the first pass over a file), every expression in the
// fresh AST has a position that matches an expression in pkg.Syntax. After
// earlier passes mutate `src`, byte offsets shift and lookups miss — at
// which point the syntactic AST fallback in matchEdgeReceiver / friends
// kicks in (same behaviour as before this layer existed).
func buildPosTypeMap(pkg *packages.Package, filename string) map[token.Pos]types.TypeAndValue {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil
	}
	// Find the *ast.File for this filename so we know what position range
	// to map. pkg.CompiledGoFiles is ordered to match pkg.Syntax.
	abs, err := filepath.Abs(filename)
	if err != nil {
		return nil
	}
	var targetFile *ast.File
	for i, f := range pkg.CompiledGoFiles {
		fabs, ferr := filepath.Abs(f)
		if ferr == nil && fabs == abs && i < len(pkg.Syntax) {
			targetFile = pkg.Syntax[i]
			break
		}
	}
	if targetFile == nil {
		return nil
	}
	// Restrict the map to positions inside this file — Types holds entries
	// for all files in the package and looking those up wastes work.
	start, end := targetFile.Pos(), targetFile.End()
	out := make(map[token.Pos]types.TypeAndValue, len(pkg.TypesInfo.Types)/4)
	for expr, tv := range pkg.TypesInfo.Types {
		p := expr.Pos()
		if p < start || p > end {
			continue
		}
		out[p] = tv
	}
	return out
}

// Resolver provides type-aware AST navigation for a single Go file.
// It runs the type checker against the parsed file (resolving imports via
// the source importer) so rewrite passes can gate dispatch on the static
// type of a method receiver — necessary to avoid corrupting schema-DSL
// call sites that share method names with generated mutation/query types.
type Resolver struct {
	Fset *token.FileSet
	File *ast.File
	Info *types.Info
	Pkg  *types.Package
	// PosTypes is the position-keyed type map built from a successful
	// go/packages.Load of the containing package, restricted to this file.
	// It supplements Info.Types so cross-package receiver types resolve
	// where the single-file gc importer couldn't. nil when go/packages
	// failed (single-file fallback mode).
	PosTypes map[token.Pos]types.TypeAndValue
}

// NewResolver parses src and runs the type checker. The returned resolver
// is safe for read-only inspection. If type checking fails on some imports
// (common with generated code that depends on external modules), the
// resolver still returns whatever types resolved successfully; callers
// should treat MatchesReceiverType as "best-effort with skip-on-uncertain".
func NewResolver(filename, src string) (*Resolver, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{
		Importer: importer.Default(),
		Error: func(err error) {
			// Swallow per-file errors; partial type info is still useful.
		},
	}
	pkg, _ := conf.Check(file.Name.Name, fset, []*ast.File{file}, info)
	// Layer in cross-package type info from a go/packages load of the
	// containing directory. Best-effort: nil when load fails or the file
	// isn't part of a Go package (e.g. test fixtures parsed from strings).
	loaded := loadPackageForFile(filename)
	posTypes := buildPosTypeMap(loaded, filename)
	return &Resolver{Fset: fset, File: file, Info: info, Pkg: pkg, PosTypes: posTypes}, nil
}

// FindFirstCall walks the AST and returns the first ast.CallExpr whose
// Fun is a SelectorExpr with Sel.Name == name. Returns nil if none found.
// Used by tests; production callers walk via astutil.Apply.
func (r *Resolver) FindFirstCall(name string) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(r.File, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == name {
			found = call
			return false
		}
		return true
	})
	return found
}

// MatchesReceiverType reports whether the receiver expression of call
// (call.Fun.(*ast.SelectorExpr).X) has the static type wantTypeName.
// wantTypeName format: "*pkgname.TypeName" (e.g. "*ent.TaskMutation",
// "*x.TaskMutation"). The leading "*" is required for pointer receivers.
//
// Returns false if:
//   - call.Fun isn't a SelectorExpr (e.g., direct function call)
//   - the receiver type can't be resolved (type checker had insufficient info)
//   - the resolved type doesn't match wantTypeName
//
// "False on uncertain" is the safe default — a missed rewrite is fixable
// by a follow-up tool run, but a wrong rewrite corrupts consumer code.
func (r *Resolver) MatchesReceiverType(call *ast.CallExpr, wantTypeName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if tv, ok := r.Info.Types[sel.X]; ok && tv.Type != nil {
		if tv.Type.String() == wantTypeName {
			return true
		}
	}
	// Cross-package fallback via the position-keyed go/packages map.
	// See ReceiverTypeMatchesPattern for the mechanism + caveats.
	if r.PosTypes != nil {
		if tv, ok := r.PosTypes[sel.X.Pos()]; ok && tv.Type != nil {
			return tv.Type.String() == wantTypeName
		}
	}
	return false
}

// ReceiverTypeMatchesPattern is like MatchesReceiverType but accepts a
// predicate function — useful when the rewrite pass needs to match any
// of several types (e.g. "*ent.*Mutation" for all entity mutations).
//
// Resolution strategy:
//  1. Try go/types resolution — preferred when imports succeed.
//  2. Fall back to AST-only declaration scanning: if call.Fun.X is a
//     plain identifier (typical method-on-variable case), look up the
//     enclosing function's params and short-var-decls for that name,
//     render the declared type expression as text, and pass it to pred.
//
// The fallback is necessary because the integration tool runs against
// individual files where cross-package imports cannot always be
// resolved by the source importer. Without it the gate would skip
// every cross-package receiver (false negative).
func (r *Resolver) ReceiverTypeMatchesPattern(call *ast.CallExpr, pred func(typeName string) bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	// Path 1: in-file go/types resolution. The single-file checker
	// resolves expressions whose types come from imports it could load
	// (std lib, gc-importable .a files). Misses most consumer-package
	// receivers.
	if tv, ok := r.Info.Types[sel.X]; ok && tv.Type != nil {
		t := tv.Type.String()
		if t != "" && !isInvalidType(t) {
			return pred(t)
		}
	}
	// Path 2: cross-package go/packages resolution, looked up by source
	// position. Resolves chained-call receivers, nested selector
	// receivers (`client.Entity.QueryEdge`), and short-var-decl
	// receivers (`q := ...; q.WithFoo(...)`) — all the shapes the
	// in-file checker can't see because it didn't load the consumer
	// package containing the receiver type. Position lookup is exact:
	// matches only when the rewriter's `src` is byte-identical to disk
	// (first pass; or a later pass on a file no earlier pass touched).
	// On mismatch, falls through to Path 3 — same as before this layer.
	if r.PosTypes != nil {
		if tv, ok := r.PosTypes[sel.X.Pos()]; ok && tv.Type != nil {
			t := tv.Type.String()
			if t != "" && !isInvalidType(t) {
				return pred(t)
			}
		}
	}
	// Path 3: AST-only fallback. Only handles the simple case where the
	// receiver expression is a bare identifier (e.g. `m.SetTitle(...)`).
	// Chained receivers (`From("x").Ref(...)`) fall through and return
	// false — safer than guessing.
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	typeText := r.lookupVarTypeText(ident)
	if typeText == "" {
		return false
	}
	return pred(typeText)
}

// isInvalidType reports whether t is the textual representation of an
// invalid (unresolved) Go type. go/types renders unresolved imports as
// "invalid type" or types containing "invalid".
func isInvalidType(t string) bool {
	return t == "invalid type" || t == "*invalid type"
}

// lookupVarTypeText finds the declaration of the variable named ident
// within the AST and returns the rendered text of its declared type
// expression. Searches:
//  1. Enclosing FuncDecl / FuncLit parameter lists.
//  2. Top-level var/const declarations.
//
// Returns "" if no declaration is found or the declaration lacks an
// explicit type.
func (r *Resolver) lookupVarTypeText(ident *ast.Ident) string {
	want := ident.Name
	var result string
	// Find the enclosing function (FuncDecl or FuncLit) by walking the
	// path from the file to ident's position.
	var enclosing ast.Node
	ast.Inspect(r.File, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if n.Pos() <= ident.Pos() && ident.End() <= n.End() {
			switch n.(type) {
			case *ast.FuncDecl, *ast.FuncLit:
				enclosing = n
			}
			return true
		}
		return false
	})
	if enclosing != nil {
		if t := paramType(enclosing, want); t != nil {
			result = renderExpr(r.Fset, t)
			if result != "" {
				return result
			}
		}
	}
	// Top-level var declarations.
	for _, decl := range r.File.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || vs.Type == nil {
				continue
			}
			for _, n := range vs.Names {
				if n.Name == want {
					return renderExpr(r.Fset, vs.Type)
				}
			}
		}
	}
	return ""
}

// paramType returns the declared type expression for the parameter named
// name in the enclosing function, or nil if not found.
func paramType(fn ast.Node, name string) ast.Expr {
	var fields *ast.FieldList
	switch f := fn.(type) {
	case *ast.FuncDecl:
		if f.Type != nil {
			fields = f.Type.Params
		}
	case *ast.FuncLit:
		if f.Type != nil {
			fields = f.Type.Params
		}
	}
	if fields == nil {
		return nil
	}
	for _, field := range fields.List {
		for _, ident := range field.Names {
			if ident.Name == name {
				return field.Type
			}
		}
	}
	return nil
}

// renderExpr returns the source-text form of expr (e.g. "*pkg.Type").
func renderExpr(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

// GenAlias returns the local name bound to genImportPath in this file
// (the import's explicit alias, or the package-name leaf of the import
// path if no alias was given), plus true.
//
// Returns ("", false) when this file does not import genImportPath at
// all — callers should treat that as "no rewrite emits in this file" or
// add the import themselves before emitting.
//
// genImportPath must be the canonical full module path of the consumer's
// generated ent package (e.g. "github.com/foo/bar/internal/ent/gen").
// Comparison is byte-exact against the unquoted import path; aliasing
// inside other modules is irrelevant here.
func (r *Resolver) GenAlias(genImportPath string) (string, bool) {
	if r.File == nil || genImportPath == "" {
		return "", false
	}
	for _, imp := range r.File.Imports {
		raw, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if raw != genImportPath {
			continue
		}
		if imp.Name != nil && imp.Name.Name != "" {
			return imp.Name.Name, true
		}
		// No alias — the local binding is the package's declared name.
		// In the absence of cross-package type info, fall back to the
		// import path's leaf segment (the conventional case where the
		// directory name matches the package name).
		return path.Base(raw), true
	}
	return "", false
}
