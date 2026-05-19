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
	"strings"
)

// FieldDesc holds the metadata needed to rewrite call sites for one field.
type FieldDesc struct {
	GoName string // exported struct field name (e.g. "Title")
	Type   string // textual Go type as it appears in the descriptor (e.g. "string")
}

// EdgeDesc holds the metadata needed to rewrite call sites for one edge.
type EdgeDesc struct {
	Cardinality  string // "O2OUnique" / "O2M" / "M2O" / "M2M" as in entbuilder.Cardinality
	TargetIDType string
	Target       string // target entity name (e.g. "Marketing"). Optional — descriptors that predate this field leave it empty.
	// Field is the FK column name backing this edge as declared in the
	// schema via `edge.To(...).Field("X_id")` or `edge.From(...).Field("X_id")`.
	// Empty when the edge has no dedicated FK column (M2M, join-tables, or
	// edges where the FK was not exposed as a field). Used by the edge-FK
	// SetField rewriter to map `m.SetField("X_id", v)` → `m.SetEdgeID("X", v)`.
	Field string
}

// EntityDesc bundles fields and edges for one entity.
type EntityDesc struct {
	Name   string
	IDType string
	Fields map[string]FieldDesc
	Edges  map[string]EdgeDesc
}

// Descriptors maps entity name → EntityDesc.
type Descriptors map[string]*EntityDesc

// LoadDescriptors walks the given directory looking for files named
// <entity>_mutation.go that declare a `<entity>Descriptor` var of type
// `*entbuilder.Descriptor`. Returns a Descriptors map keyed by entity name.
func LoadDescriptors(dir string) (Descriptors, error) {
	out := make(Descriptors)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_mutation.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if !strings.HasSuffix(name.Name, "Descriptor") {
						continue
					}
					if i >= len(vs.Values) {
						continue
					}
					ed := parseDescriptorLiteral(vs.Values[i])
					if ed == nil {
						continue
					}
					if ed.Name == "" {
						// Fallback to the filename-derived name for descriptors that
						// (somehow) lack the Name field — preserves prior behavior.
						ent := name.Name[:len(name.Name)-len("Descriptor")]
						ed.Name = strings.Title(ent) //nolint:staticcheck // "task" → "Task"
					}
					out[ed.Name] = ed
				}
			}
		}
	}
	return out, nil
}

// parseDescriptorLiteral extracts FieldSpec / EdgeSpec entries from the
// composite literal value of a *<entity>Descriptor variable. Returns nil
// if the expression doesn't match the expected shape.
func parseDescriptorLiteral(expr ast.Expr) *EntityDesc {
	// The descriptor is `&entbuilder.Descriptor{ ... }` — strip the unary.
	un, ok := expr.(*ast.UnaryExpr)
	if !ok || un.Op != token.AND {
		return nil
	}
	cl, ok := un.X.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	ed := &EntityDesc{
		Fields: map[string]FieldDesc{},
		Edges:  map[string]EdgeDesc{},
	}
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Name":
			if s, ok := stringLiteral(kv.Value); ok {
				ed.Name = s
			}
		case "IDType":
			ed.IDType = exprToString(kv.Value)
		case "Fields":
			ed.Fields = parseFieldMap(kv.Value)
		case "Edges":
			ed.Edges = parseEdgeMap(kv.Value)
		}
	}
	return ed
}

func parseFieldMap(expr ast.Expr) map[string]FieldDesc {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	out := make(map[string]FieldDesc)
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := stringLiteral(kv.Key)
		if !ok {
			continue
		}
		fd := FieldDesc{}
		inner, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, ie := range inner.Elts {
			ikv, ok := ie.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ikey, ok := ikv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch ikey.Name {
			case "Type":
				// reflect.TypeFor[<Type>]() — extract the type arg
				if idx, ok := extractTypeForArg(ikv.Value); ok {
					fd.Type = idx
				}
			case "GoName":
				if s, ok := stringLiteral(ikv.Value); ok {
					fd.GoName = s
				}
			}
		}
		out[name] = fd
	}
	return out
}

func parseEdgeMap(expr ast.Expr) map[string]EdgeDesc {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	out := make(map[string]EdgeDesc)
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := stringLiteral(kv.Key)
		if !ok {
			continue
		}
		ed := EdgeDesc{}
		inner, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, ie := range inner.Elts {
			ikv, ok := ie.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			ikey, ok := ikv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch ikey.Name {
			case "Cardinality":
				ed.Cardinality = exprToString(ikv.Value)
			case "TargetIDType":
				if idx, ok := extractTypeForArg(ikv.Value); ok {
					ed.TargetIDType = idx
				}
			case "Target":
				if s, ok := stringLiteral(ikv.Value); ok {
					ed.Target = s
				}
			case "Field":
				if s, ok := stringLiteral(ikv.Value); ok {
					ed.Field = s
				}
			}
		}
		out[name] = ed
	}
	return out
}

// extractTypeForArg pulls the type-param out of `reflect.TypeFor[<T>]()`.
func extractTypeForArg(expr ast.Expr) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	idx, ok := call.Fun.(*ast.IndexExpr)
	if !ok {
		return "", false
	}
	return exprToString(idx.Index), true
}

func stringLiteral(expr ast.Expr) (string, bool) {
	bl, ok := expr.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(bl.Value, `"`), true
}

// exprToString renders an AST expression back to its Go source form using
// go/printer. This handles all expression node kinds uniformly — including
// MapType, ArrayType, StarExpr, FuncType, InterfaceType, StructType,
// IndexExpr, IndexListExpr — so complex generic type arguments inside
// reflect.TypeFor[<T>]() (e.g. map[string]any, []uuid.UUID, *pkg.T) round-trip
// correctly. Returns "" for a nil expression or on a printer error.
func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return buf.String()
}
