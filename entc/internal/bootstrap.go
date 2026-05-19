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
// mode replaces with `return nil`. These methods run at the consumer's runtime,
// not at codegen time, so the loader does not need their behavior — only their
// signatures, so the schema still satisfies ent.Schema.
var hookMethodNames = map[string]bool{
	"Hooks":        true,
	"Policy":       true,
	"Interceptors": true,
}

// StripHookBodies returns src with the bodies of every method whose name is in
// hookMethodNames (`Hooks`, `Policy`, `Interceptors`) replaced with `return nil`.
// Top-level functions with the same name are NOT touched — only methods (where
// FuncDecl.Recv is non-nil). After stripping, any imports made unused by the
// removal are deleted from the import block.
//
// This is the core primitive behind bootstrap mode: a schema package can
// reference generated mutation types like *ent.UserMutation from inside its
// Hooks() body, which makes the package un-typecheckable until codegen runs;
// stripping the body removes that dependency cycle so the loader succeeds.
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

// StripHookBodies returns src with the bodies of every method whose name is in
// hookMethodNames (`Hooks`, `Policy`, `Interceptors`) replaced with `return nil`.
// Top-level functions with the same name are NOT touched — only methods (where
// FuncDecl.Recv is non-nil). After stripping, any imports made unused by the
// removal are deleted from the import block.
//
// This is the core primitive behind bootstrap mode: a schema package can
// reference generated mutation types like *ent.UserMutation from inside its
// Hooks() body, which makes the package un-typecheckable until codegen runs;
// stripping the body removes that dependency cycle so the loader succeeds.
func StripHookBodies(src []byte) ([]byte, error) {
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
		fn.Body = newReturnNilBody(fset)
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
	if err := stripAndCopyTree(srcDir, dst); err != nil {
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

func stripAndCopyTree(src, dst string) error {
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
			stripped, serr := StripHookBodies(data)
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
