// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/imports"
)

var formatOnly = &imports.Options{Comments: true, TabIndent: true, TabWidth: 8, FormatOnly: true}

// FormatSource formats a generated file the way goimports does. goimports
// resolves packages for every file that qualifies a name with no import, and
// each resolution spawns `go env` and walks the module cache. A file whose
// qualifiers are all imports or package-level names of its own package only
// needs its unused imports dropped and its import block sorted, so that case
// skips the resolution. packageNames holds the names the package declares in
// other files; see PackageLevelNames.
func FormatSource(filename string, src []byte, packageNames map[string]bool) ([]byte, error) {
	if pruned, ok := pruneUnusedImports(filename, src, packageNames); ok {
		return imports.Process(filename, pruned, formatOnly)
	}
	return imports.Process(filename, src, nil)
}

// PackageLevelNames returns the names a file declares at package level. A
// package name can be spelled unlike its import path, so a qualifier that is
// neither an import nor one of these names needs goimports to resolve it.
func PackageLevelNames(filename string, src []byte) []string {
	f, err := parser.ParseFile(token.NewFileSet(), filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	var names []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				names = append(names, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names = append(names, n.Name)
					}
				case *ast.TypeSpec:
					names = append(names, s.Name.Name)
				}
			}
		}
	}
	return names
}

// pruneUnusedImports drops the imports no selector in the file qualifies with.
// It reports false when a selector qualifies a name that neither an import nor
// packageNames declares.
func pruneUnusedImports(filename string, src []byte, packageNames map[string]bool) ([]byte, bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, false
	}
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil {
				used[id.Name] = true
			}
		}
		return true
	})
	declared := map[string]bool{}
	for _, imp := range f.Imports {
		declared[importName(imp)] = true
	}
	for name := range used {
		if !declared[name] && !packageNames[name] {
			return nil, false
		}
	}
	for _, imp := range append([]*ast.ImportSpec(nil), f.Imports...) {
		name := importName(imp)
		if name == "_" || name == "." || used[name] {
			continue
		}
		p, _ := strconv.Unquote(imp.Path.Value)
		if imp.Name != nil {
			astutil.DeleteNamedImport(fset, f, name, p)
		} else {
			astutil.DeleteImport(fset, f, p)
		}
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func importName(imp *ast.ImportSpec) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	p, _ := strconv.Unquote(imp.Path.Value)
	return assumedPackageName(p)
}

// assumedPackageName mirrors goimports' guess of a package name from its path.
func assumedPackageName(importPath string) string {
	base := path.Base(importPath)
	if strings.HasPrefix(base, "v") {
		if _, err := strconv.Atoi(base[1:]); err == nil {
			if dir := path.Dir(importPath); dir != "." {
				base = path.Base(dir)
			}
		}
	}
	base = strings.TrimPrefix(base, "go-")
	if i := strings.IndexFunc(base, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }); i >= 0 {
		base = base[:i]
	}
	return base
}
