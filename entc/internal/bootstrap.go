// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package internal

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// returnNilBody is the source we parse to obtain a properly-positioned BlockStmt
// that formats as a multi-line body:
//
//	{
//		return nil
//	}
const returnNilBody = "package p\nfunc f(){\nreturn nil\n}\n"

// hookMethodNames is the set of Schema-interface methods whose bodies bootstrap
// mode replaces with a count-preserving stub. These methods run at the
// consumer's runtime, not at codegen time, so the loader does not need their
// behavior — only their signatures and slot counts.
var hookMethodNames = map[string]bool{
	"Hooks":        true,
	"Policy":       true,
	"Interceptors": true,
}

// hookSliceTypes maps hook/interceptor method names to the Go slice type used
// in the count-preserving make() stub body.
var hookSliceTypes = map[string]string{
	"Hooks":        "ent.Hook",
	"Interceptors": "ent.Interceptor",
}

// newReturnNilBody parses a tiny stub into fset and returns the *ast.BlockStmt
// whose positions cause go/format to render it as a multi-line body:
//
//	{
//		return nil
//	}
func newReturnNilBody(fset *token.FileSet) *ast.BlockStmt {
	stub, err := parser.ParseFile(fset, "<bootstrap>", returnNilBody, 0)
	if err != nil {
		// returnNilBody is a constant literal; this must never fail.
		panic(fmt.Sprintf("bootstrap: parse stub: %v", err))
	}
	return stub.Decls[0].(*ast.FuncDecl).Body
}

// newReturnMakeBody returns a BlockStmt that formats as:
//
//	{
//		return make([]T, N)
//	}
func newReturnMakeBody(fset *token.FileSet, elemType string, n int) *ast.BlockStmt {
	src := fmt.Sprintf("package p\nfunc f(){\nreturn make([]%s, %d)\n}\n", elemType, n)
	stub, err := parser.ParseFile(fset, "<bootstrap>", src, 0)
	if err != nil {
		panic(fmt.Sprintf("bootstrap: parse make stub: %v", err))
	}
	return stub.Decls[0].(*ast.FuncDecl).Body
}

// countReturnElems returns the number of elements in the first return
// statement of fn's body if that statement returns a composite literal
// (e.g. []ent.Hook{h1, h2}), or -1 if the pattern is not recognised.
func countReturnElems(fn *ast.FuncDecl) int {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return 0
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return -1
	}
	switch r := ret.Results[0].(type) {
	case *ast.CompositeLit:
		return len(r.Elts)
	case *ast.Ident:
		if r.Name == "nil" {
			return 0
		}
	}
	return -1
}

// CountHookFunctions scans all .go files (regardless of build tags) in dir
// and returns a map from top-level function name to the number of hook or
// interceptor elements it returns. Only functions whose return type is
// []ent.Hook or []ent.Interceptor and whose body returns a recognisable
// composite literal are included.
//
// This is used by StageStrippedSchema to supply accurate slot counts to
// StripHookBodies so the generated runtime.go allocates the right number of
// hook/interceptor slots even when the real function bodies are in tagged
// files excluded from packages.Load.
func CountHookFunctions(dir string) (map[string]int, error) {
	counts := make(map[string]int)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read dir: %w", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), data, 0)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue // only top-level functions
			}
			if !returnsHookSlice(fn) {
				continue
			}
			n := countReturnElems(fn)
			if n >= 0 {
				counts[fn.Name.Name] = n
			}
		}
	}
	return counts, nil
}

// returnsHookSlice reports whether fn's return type is []ent.Hook or
// []ent.Interceptor (the two slice types whose counts matter for codegen).
func returnsHookSlice(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}
	arr, ok := fn.Type.Results.List[0].Type.(*ast.ArrayType)
	if !ok || arr.Elt == nil {
		return false
	}
	sel, ok := arr.Elt.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok || x.Name != "ent" {
		return false
	}
	return sel.Sel.Name == "Hook" || sel.Sel.Name == "Interceptor"
}

// StripHookBodies returns src with the bodies of every method whose name is in
// hookMethodNames (`Hooks`, `Policy`, `Interceptors`) replaced with a
// count-preserving stub:
//   - `return make([]ent.Hook, N)` when N > 0 (preserves slot count for codegen)
//   - `return nil` when N == 0
//
// The count N is determined by inspecting the method body:
//  1. Composite literal return (e.g. `return []ent.Hook{h1, h2}`): N = len(elts).
//  2. Named-function call (e.g. `return myHooks()`): N = funcCounts["myHooks"],
//     defaulting to 0 when the name is absent.
//  3. nil return or unrecognised pattern: N = 0.
//
// Top-level functions with the same name are NOT touched — only methods (where
// FuncDecl.Recv is non-nil). After stripping, any imports made unused by the
// removal are deleted from the import block.
//
// funcCounts is typically built by CountHookFunctions over the schema dir and
// may be nil (treated as empty).
func StripHookBodies(src []byte, funcCounts map[string]int) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: parse: %w", err)
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Recv == nil {
			// Top-level function, not a method. Skip.
			continue
		}
		if !hookMethodNames[fn.Name.Name] {
			continue
		}
		n := resolveHookCount(fn, funcCounts)
		elemType := hookSliceTypes[fn.Name.Name]
		if n > 0 && elemType != "" {
			fn.Body = newReturnMakeBody(fset, elemType, n)
		} else {
			fn.Body = newReturnNilBody(fset)
		}
	}

	// Remove imports made unused by stripping. Iterate over a copy because
	// astutil.DeleteImport mutates f.Imports.
	imports := append([]*ast.ImportSpec(nil), f.Imports...)
	for _, imp := range imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		// Preserve blank imports (used for side effects) and named imports
		// (the user explicitly aliased these — leaving them in keeps intent).
		if imp.Name != nil && (imp.Name.Name == "_" || imp.Name.Name == ".") {
			continue
		}
		if astutil.UsesImport(f, path) {
			continue
		}
		if imp.Name != nil {
			astutil.DeleteNamedImport(fset, f, imp.Name.Name, path)
		} else {
			astutil.DeleteImport(fset, f, path)
		}
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, fmt.Errorf("bootstrap: format: %w", err)
	}
	return buf.Bytes(), nil
}

// StageStrippedSchema copies the schema directory at srcDir into a fresh
// temporary directory, AST-stripping the bodies of Hooks() / Policy() /
// Interceptors() methods on every .go file along the way. Non-.go files are
// copied verbatim. The source directory is never modified.
//
// Caller is responsible for removing the returned directory.
//
// This is the entrypoint bootstrap mode uses before invoking the loader:
// `dst, err := StageStrippedSchema(schemaPath); defer os.RemoveAll(dst);
// loader.Load(dst, ...)`.
func StageStrippedSchema(srcDir string) (string, error) {
	if _, err := os.Stat(srcDir); err != nil {
		return "", fmt.Errorf("bootstrap: stat src: %w", err)
	}
	// Create the temp dir adjacent to srcDir (i.e. in the parent directory)
	// rather than in the OS-global temp dir. This ensures the stripped copy
	// is within the same Go module as the original schema, which is required
	// for packages.Load to resolve the module context correctly.
	dst, err := os.MkdirTemp(filepath.Dir(srcDir), "ent-bootstrap-*")
	if err != nil {
		return "", fmt.Errorf("bootstrap: tempdir: %w", err)
	}
	// First pass: collect hook function counts across the whole source dir so
	// that the second pass can emit correct make([]T, N) stubs.
	funcCounts, err := CountHookFunctions(srcDir)
	if err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	if err := stripAndCopyTree(srcDir, dst, funcCounts); err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	// packages.Load distinguishes import paths from filesystem paths by the
	// presence of "./" / "../" / "/". MkdirTemp returns a path joined onto
	// its dir argument, so if srcDir was "./src/ent/schema" the result is
	// "src/ent/ent-bootstrap-XXXX" — which packages.Load then treats as an
	// import path and tries to resolve under GOROOT/src. Return an absolute
	// path so packages.Load resolves it as a filesystem dir unambiguously.
	abs, err := filepath.Abs(dst)
	if err != nil {
		_ = os.RemoveAll(dst)
		return "", fmt.Errorf("bootstrap: abs path: %w", err)
	}
	return abs, nil
}

func stripAndCopyTree(src, dst string, funcCounts map[string]int) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if strings.HasSuffix(d.Name(), ".go") {
			stripped, serr := StripHookBodies(data, funcCounts)
			if serr != nil {
				// File doesn't parse cleanly -- copy verbatim and let the
				// downstream loader surface the real error. Don't pretend
				// to have stripped a file we couldn't parse.
				stripped = data
			}
			data = stripped
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// resolveHookCount determines the hook/interceptor slot count for a method
// whose body is being stripped. It checks:
//  1. Composite literal return: count the elements directly.
//  2. Named-function call: look up the function in funcCounts.
//  3. Anything else (nil, unknown): return 0.
func resolveHookCount(fn *ast.FuncDecl, funcCounts map[string]int) int {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return 0
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return 0
	}
	switch r := ret.Results[0].(type) {
	case *ast.CompositeLit:
		return len(r.Elts)
	case *ast.Ident:
		if r.Name == "nil" {
			return 0
		}
	case *ast.CallExpr:
		// Named function call: e.g. `return boxFolderHooks()`
		if ident, ok := r.Fun.(*ast.Ident); ok {
			if n, found := funcCounts[ident.Name]; found {
				return n
			}
		}
	}
	return 0
}
