// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package internal

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// hookMethodNames is the set of Schema-interface methods whose hook-slice
// return expressions bootstrap mode rewrites to count-preserving stubs. These
// methods run at the consumer's runtime, not at codegen time, so the loader
// does not need their real behavior — only their per-instance slot counts.
var hookMethodNames = map[string]bool{
	"Hooks":        true,
	"Policy":       true,
	"Interceptors": true,
}

// hookSliceTypes maps hook/interceptor method names to the Go slice type used
// in the count-preserving make() stub expression. Policy() has no entry: its
// returns are rewritten to nil (it is a single value, never slot-counted).
var hookSliceTypes = map[string]string{
	"Hooks":        "ent.Hook",
	"Interceptors": "ent.Interceptor",
}

// makeSliceExpr builds the expression `make([]elemType, n)` (e.g.
// `make([]ent.Hook, 1)`), used to replace a hook/interceptor return value while
// preserving its element count for codegen.
func makeSliceExpr(elemType string, n int) ast.Expr {
	var elt ast.Expr
	if i := strings.IndexByte(elemType, '.'); i >= 0 {
		elt = &ast.SelectorExpr{X: ast.NewIdent(elemType[:i]), Sel: ast.NewIdent(elemType[i+1:])}
	} else {
		elt = ast.NewIdent(elemType)
	}
	return &ast.CallExpr{
		Fun: ast.NewIdent("make"),
		Args: []ast.Expr{
			&ast.ArrayType{Elt: elt},
			&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(n)},
		},
	}
}

// UncountableHookReturnError is returned by StripHookBodies when a
// Hooks()/Interceptors() method has a return expression whose hook-slice
// element count cannot be determined statically — anything other than a
// composite literal, a bare nil, or a call to a helper recorded by
// CountHookFunctions. Silently stubbing such a return to nil would wire zero
// slots and drop the entity's hooks at runtime with no error, so bootstrap
// fails closed instead.
type UncountableHookReturnError struct {
	Method string // the Hooks/Interceptors method name
	Expr   string // the offending return expression, rendered
}

func (e *UncountableHookReturnError) Error() string {
	return fmt.Sprintf("cannot determine hook count for %s() return %q: return a composite "+
		"literal (e.g. []ent.Hook{...}), nil, or a helper function whose body is a single "+
		"composite-literal return so CountHookFunctions can count it", e.Method, e.Expr)
}

// exprString renders an AST expression back to source for error messages.
func exprString(fset *token.FileSet, e ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, e); err != nil {
		return fmt.Sprintf("%T", e)
	}
	return buf.String()
}

// checkHookMethodVisibility fails closed when a schema type declares a
// Hooks/Interceptors/Policy method ONLY in build-excluded (!entcodegen) files.
//
// Codegen loads the schema with -tags entcodegen, so a method that lives solely
// in a !entcodegen file is invisible to the loader: it wires zero hooks for that
// type and the real hooks silently never fire at runtime. The method
// declaration must live in a file compiled under entcodegen (typically the
// untagged entity file); only the hook/interceptor *implementations* (which
// reference the not-yet-generated gen package) belong in !entcodegen files.
//
// It scans every .go file regardless of build tags (like CountHookFunctions),
// records which files declaring each type's hook method are visible under
// entcodegen, and errors for any method seen only in excluded files.
func checkHookMethodVisibility(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("bootstrap: read dir: %w", err)
	}
	fset := token.NewFileSet()
	visible := make(map[string]bool)      // "Type.Method" -> declared in a codegen-visible file
	excludedIn := make(map[string][]string) // "Type.Method" -> excluded files declaring it
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil {
			continue
		}
		f, perr := parser.ParseFile(fset, e.Name(), data, parser.ParseComments)
		if perr != nil {
			continue
		}
		excluded := fileExcludedUnderEntcodegen(data)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !hookMethodNames[fn.Name.Name] {
				continue
			}
			recv := recvTypeName(fn)
			if recv == "" {
				continue
			}
			key := recv + "." + fn.Name.Name
			if excluded {
				excludedIn[key] = append(excludedIn[key], e.Name())
				if _, seen := visible[key]; !seen {
					visible[key] = false
				}
			} else {
				visible[key] = true
			}
		}
	}
	var bad []string
	for key, vis := range visible {
		if !vis {
			bad = append(bad, fmt.Sprintf("%s (only in %s)", key, strings.Join(excludedIn[key], ", ")))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("bootstrap: hook method(s) declared only in build-excluded (!entcodegen) files, so "+
			"codegen would wire zero hooks for them and the real hooks would silently never fire at runtime — move "+
			"the method declaration to a file compiled under entcodegen (e.g. the untagged entity file), keeping the "+
			"implementation in the !entcodegen file: %s", strings.Join(bad, "; "))
	}
	return nil
}

// recvTypeName returns the receiver type name of a method (dereferencing a
// pointer receiver), or "" if it cannot be determined.
func recvTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// fileExcludedUnderEntcodegen reports whether src's //go:build constraint
// excludes the file when only the entcodegen tag is set (the codegen build).
// A file with no build constraint is included (false). Schema files use only
// the entcodegen/entc tag dimension, so evaluating with entcodegen=true and
// every other tag=false matches the codegen environment.
func fileExcludedUnderEntcodegen(src []byte) bool {
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "//") {
			break // build constraints must precede the package clause
		}
		if expr, err := constraint.Parse(line); err == nil {
			return !expr.Eval(func(tag string) bool { return tag == "entcodegen" })
		}
	}
	return false
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

// StripHookBodies returns src with the hook-slice return expressions of every
// method whose name is in hookMethodNames (`Hooks`, `Policy`, `Interceptors`)
// replaced with a count-preserving stub, while PRESERVING the method's control
// flow. Each single-value return is rewritten as:
//   - Hooks/Interceptors: `return make([]T, N)` when N > 0, else `return nil`.
//   - Policy: `return nil` (a single value, never slot-counted).
//
// N is derived from the return expression: composite-literal length, or the
// funcCounts entry for a named-function call (e.g. `return myHooks()`),
// defaulting to 0.
//
// Preserving control flow matters for guarded methods such as
//
//	func (b BaseMixin) Hooks() []ent.Hook {
//		if b.DisableSoftDelete { return nil }
//		return baseMixinHooks(b)
//	}
//
// Replacing the whole body with a static `make([]ent.Hook, 1)` would lose the
// guard, making the loader count one slot for hard-delete mixins too and
// panicking the generated runtime.go (which then indexes a nil hook slice). By
// rewriting only the return expressions, the loader runs the guard per
// instance: 0 slots for DisableSoftDelete=true, 1 otherwise. Returns inside
// nested function literals are left untouched.
//
// Top-level functions are NOT touched — only methods (FuncDecl.Recv non-nil).
// After rewriting, any imports made unused are deleted from the import block.
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
		if !ok || fn.Recv == nil {
			// Top-level functions are not stripped; only methods.
			continue
		}
		if !hookMethodNames[fn.Name.Name] {
			continue
		}
		if err := stripHookMethodReturns(fset, fn, hookSliceTypes[fn.Name.Name], funcCounts); err != nil {
			return nil, err
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
	// Fail closed if any type's Hooks/Interceptors/Policy method is declared only
	// in a build-excluded (!entcodegen) file: codegen would never see it and would
	// wire zero hooks, dropping them silently at runtime.
	if err := checkHookMethodVisibility(srcDir); err != nil {
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
				var ue *UncountableHookReturnError
				if errors.As(serr, &ue) {
					// An undeterminable hook count would be silently stubbed to
					// zero and drop the entity's hooks at runtime. Fail closed.
					return fmt.Errorf("bootstrap: %s: %w", path, serr)
				}
				// File doesn't parse cleanly -- copy verbatim and let the
				// downstream loader surface the real error. Don't pretend to
				// have stripped a file we couldn't parse.
				stripped = data
			}
			data = stripped
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// stripHookMethodReturns rewrites every single-value return statement in a hook
// method body to a count-preserving stub, preserving the surrounding control
// flow. Returns inside nested function literals are left untouched.
//
// For Hooks/Interceptors (elemType != ""): the return value becomes
// make([]elemType, N) where N is the count implied by the original expression,
// or nil when N == 0. For Policy (elemType == ""): every return becomes nil.
func stripHookMethodReturns(fset *token.FileSet, fn *ast.FuncDecl, elemType string, funcCounts map[string]int) error {
	if fn.Body == nil {
		return nil
	}
	var stripErr error
	astutil.Apply(fn.Body, func(c *astutil.Cursor) bool {
		if stripErr != nil {
			return false
		}
		switch node := c.Node().(type) {
		case *ast.FuncLit:
			return false // never rewrite returns inside nested closures
		case *ast.ReturnStmt:
			if len(node.Results) != 1 {
				return false
			}
			if elemType == "" {
				node.Results[0] = ast.NewIdent("nil")
				return false
			}
			n, ok := countReturnResult(node.Results[0], funcCounts)
			if !ok {
				stripErr = &UncountableHookReturnError{
					Method: fn.Name.Name,
					Expr:   exprString(fset, node.Results[0]),
				}
				return false
			}
			if n > 0 {
				node.Results[0] = makeSliceExpr(elemType, n)
			} else {
				node.Results[0] = ast.NewIdent("nil")
			}
			return false
		}
		return true
	}, nil)
	return stripErr
}

// countReturnResult returns the hook/interceptor element count implied by a
// single return expression, and whether that count could be determined. A
// composite literal counts its elements; a bare `nil` is a deliberate zero; a
// call to a helper recorded by CountHookFunctions uses that count. Any other
// form — an unrecorded helper call, a selector call (pkg.Fn()), a bare
// identifier, append(), etc. — is uncountable: ok is false so the caller can
// fail closed instead of silently stubbing the return to nil and dropping the
// entity's hooks at runtime.
func countReturnResult(expr ast.Expr, funcCounts map[string]int) (count int, ok bool) {
	switch r := expr.(type) {
	case *ast.CompositeLit:
		return len(r.Elts), true
	case *ast.Ident:
		if r.Name == "nil" {
			return 0, true
		}
	case *ast.CallExpr:
		if ident, ok := r.Fun.(*ast.Ident); ok {
			if n, found := funcCounts[ident.Name]; found {
				return n, true
			}
		}
	}
	return 0, false
}
