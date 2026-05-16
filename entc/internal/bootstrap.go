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
	"strconv"

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
