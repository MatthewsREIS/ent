// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package base

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// MigrateCmd returns migration-oriented utility commands.
func MigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "run code and API migration helpers",
	}
	cmd.AddCommand(splitAPIMigrateCmd())
	return cmd
}

func splitAPIMigrateCmd() *cobra.Command {
	var (
		writeChanges bool
		reportPath   string
	)
	cmd := &cobra.Command{
		Use:   "split-api [path]",
		Short: "best-effort migration for sub-package split API changes",
		Long:  "Apply safe, mechanical rewrites for the sub-package split migration and emit a report for remaining manual fixes.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			return runSplitAPIMigration(splitAPIMigrationConfig{
				TargetPath:   target,
				WriteChanges: writeChanges,
				ReportPath:   reportPath,
				Stdout:       cmd.OutOrStdout(),
			})
		},
	}
	cmd.Flags().BoolVar(&writeChanges, "write", false, "write changed files in place")
	cmd.Flags().StringVar(&reportPath, "report", "", "write markdown residual report to file")
	return cmd
}

type splitAPIMigrationConfig struct {
	TargetPath   string
	WriteChanges bool
	ReportPath   string
	Stdout       interface{ Write([]byte) (int, error) }
}

type splitAPISummary struct {
	FilesScanned int
	FilesChanged int
	Transform    map[string]int
	Residuals    []splitAPIResidual
}

type splitAPIResidual struct {
	File    string
	Line    int
	Pattern string
	Snippet string
	Advice  string
}

func runSplitAPIMigration(cfg splitAPIMigrationConfig) error {
	abs, err := filepath.Abs(cfg.TargetPath)
	if err != nil {
		return err
	}
	load := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Dir: abs,
	}
	pks, err := packages.Load(load, "./...")
	if err != nil {
		return err
	}
	var (
		summary = splitAPISummary{Transform: make(map[string]int)}
		seen    = make(map[string]struct{})
	)
	for _, pkg := range pks {
		for i, f := range pkg.Syntax {
			if i >= len(pkg.CompiledGoFiles) {
				continue
			}
			filePath := pkg.CompiledGoFiles[i]
			if !strings.HasPrefix(filePath, abs+string(os.PathSeparator)) && filePath != abs {
				continue
			}
			if _, ok := seen[filePath]; ok {
				continue
			}
			seen[filePath] = struct{}{}
			summary.FilesScanned++
			res, err := rewriteSplitAPIFile(filePath, pkg.Fset, f, pkg.TypesInfo, cfg.WriteChanges)
			if err != nil {
				return fmt.Errorf("rewrite %s: %w", filePath, err)
			}
			if res.Changed {
				summary.FilesChanged++
			}
			for k, v := range res.Transform {
				summary.Transform[k] += v
			}
			summary.Residuals = append(summary.Residuals, res.Residuals...)
		}
	}
	if cfg.ReportPath != "" {
		if err := writeSplitAPIReport(cfg.ReportPath, summary); err != nil {
			return err
		}
	}
	printSplitAPISummary(cfg.Stdout, cfg, summary)
	return nil
}

type splitAPIFileResult struct {
	Changed   bool
	Transform map[string]int
	Residuals []splitAPIResidual
}

func rewriteSplitAPIFile(filePath string, fset *token.FileSet, file *ast.File, info *types.Info, write bool) (splitAPIFileResult, error) {
	res := splitAPIFileResult{Transform: make(map[string]int)}
	if isGenerated(file) {
		return res, nil
	}
	parent := parentMap(file)
	entAlias := detectEntAlias(file)
	scopeCache := make(map[ast.Node]splitAPIScope)

	astutil.Apply(file, func(c *astutil.Cursor) bool {
		n := c.Node()
		switch n := n.(type) {
		case *ast.CompositeLit:
			if rewriteNotFoundError(n) {
				res.Changed = true
				res.Transform["notfound-keyed"]++
			}
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn := enclosingFunc(c.Node(), parent)
			scope := scopeCache[fn]
			if fn != nil && scope.empty() {
				scope = computeScope(fn, info)
				scopeCache[fn] = scope
			}
			if rewriteEntityMethod(n, sel, info, scope, entAlias, &res, filePath, fset) {
				return true
			}
			if rewriteEdgeSetter(n, sel, info, &res) {
				return true
			}
			if sel.Sel.Name == "Tx" && len(n.Args) == 0 {
				addResidual(&res, filePath, fset, n, "mutation-tx", "m.Tx()", "manual: replace with ent.TxFromContext(ctx) and add nil check")
			}
		}
		return true
	}, nil)

	collectResiduals(filePath, fset, file, info, &res)
	if !res.Changed || !write {
		return res, nil
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return res, err
	}
	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		return res, err
	}
	return res, nil
}

type splitAPIScope struct {
	CtxName    string
	ClientName string
}

func (s splitAPIScope) empty() bool {
	return s.CtxName == "" && s.ClientName == ""
}

func computeScope(fn ast.Node, info *types.Info) splitAPIScope {
	var (
		scope       splitAPIScope
		clientNames []string
		ctxNames    []string
	)
	params := func(ft *ast.FuncType) {
		if ft == nil || ft.Params == nil {
			return
		}
		for _, field := range ft.Params.List {
			t := info.TypeOf(field.Type)
			for _, name := range field.Names {
				if isContextType(t) {
					ctxNames = append(ctxNames, name.Name)
				}
				if isEntClientType(t) {
					clientNames = append(clientNames, name.Name)
				}
			}
		}
	}
	var body *ast.BlockStmt
	switch fn := fn.(type) {
	case *ast.FuncDecl:
		params(fn.Type)
		body = fn.Body
	case *ast.FuncLit:
		params(fn.Type)
		body = fn.Body
	}
	if body != nil {
		ast.Inspect(body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := info.Defs[id]
			if obj == nil {
				return true
			}
			t := obj.Type()
			if isEntClientType(t) {
				clientNames = append(clientNames, id.Name)
			}
			if isContextType(t) {
				ctxNames = append(ctxNames, id.Name)
			}
			return true
		})
	}
	scope.CtxName = choosePreferred(ctxNames, "ctx")
	scope.ClientName = choosePreferred(clientNames, "client")
	return scope
}

func choosePreferred(names []string, preferred string) string {
	if len(names) == 0 {
		return ""
	}
	for _, n := range names {
		if n == preferred {
			return n
		}
	}
	return names[0]
}

func rewriteEntityMethod(call *ast.CallExpr, sel *ast.SelectorExpr, info *types.Info, scope splitAPIScope, entAlias string, res *splitAPIFileResult, filePath string, fset *token.FileSet) bool {
	if len(call.Args) != 0 {
		return false
	}
	isEntity, entityName := isEntityReceiver(info, sel.X)
	if !isEntity {
		if sel.Sel.Name == "Client" {
			if scope.CtxName == "" || entAlias == "" {
				addResidual(res, filePath, fset, call, "mutation-client", "m.Client()", "manual: replace with ent.FromContext(ctx)")
				return false
			}
			call.Fun = &ast.SelectorExpr{X: ast.NewIdent(entAlias), Sel: ast.NewIdent("FromContext")}
			call.Args = []ast.Expr{ast.NewIdent(scope.CtxName)}
			res.Changed = true
			res.Transform["mutation-client"]++
			return true
		}
		return false
	}

	switch {
	case strings.HasPrefix(sel.Sel.Name, "Query"):
		if scope.ClientName == "" {
			addResidual(res, filePath, fset, call, "entity-query", "entity.QueryX()", "manual: rewrite to client.<Entity>.QueryX(entity)")
			return false
		}
		call.Fun = clientEntitySelector(scope.ClientName, entityName, sel.Sel.Name)
		call.Args = []ast.Expr{sel.X}
		res.Changed = true
		res.Transform["entity-query"]++
		return true
	case sel.Sel.Name == "Update":
		if scope.ClientName == "" {
			addResidual(res, filePath, fset, call, "entity-update", "entity.Update()", "manual: rewrite to client.<Entity>.UpdateOne(entity)")
			return false
		}
		call.Fun = clientEntitySelector(scope.ClientName, entityName, "UpdateOne")
		call.Args = []ast.Expr{sel.X}
		res.Changed = true
		res.Transform["entity-update"]++
		return true
	case sel.Sel.Name == "Unwrap":
		if scope.ClientName == "" || scope.CtxName == "" {
			addResidual(res, filePath, fset, call, "entity-unwrap", "entity.Unwrap()", "manual: rewrite to client.<Entity>.GetX(ctx, entity.ID)")
			return false
		}
		call.Fun = clientEntitySelector(scope.ClientName, entityName, "GetX")
		call.Args = []ast.Expr{ast.NewIdent(scope.CtxName), &ast.SelectorExpr{X: sel.X, Sel: ast.NewIdent("ID")}}
		res.Changed = true
		res.Transform["entity-unwrap"]++
		return true
	default:
		return false
	}
}

func rewriteEdgeSetter(call *ast.CallExpr, sel *ast.SelectorExpr, info *types.Info, res *splitAPIFileResult) bool {
	name := sel.Sel.Name
	if strings.HasPrefix(name, "Set") && !strings.HasSuffix(name, "ID") && len(call.Args) == 1 {
		if isEntityArg(info, call.Args[0]) {
			sel.Sel = ast.NewIdent(name + "ID")
			call.Args = []ast.Expr{&ast.SelectorExpr{X: call.Args[0], Sel: ast.NewIdent("ID")}}
			res.Changed = true
			res.Transform["edge-setter-id"]++
			return true
		}
	}
	if strings.HasPrefix(name, "Add") && !strings.HasSuffix(name, "IDs") && len(call.Args) > 0 {
		for _, arg := range call.Args {
			if !isEntityArg(info, arg) {
				return false
			}
		}
		sel.Sel = ast.NewIdent(name + "IDs")
		newArgs := make([]ast.Expr, 0, len(call.Args))
		for _, arg := range call.Args {
			newArgs = append(newArgs, &ast.SelectorExpr{X: arg, Sel: ast.NewIdent("ID")})
		}
		call.Args = newArgs
		res.Changed = true
		res.Transform["edge-setter-ids"]++
		return true
	}
	return false
}

func collectResiduals(filePath string, fset *token.FileSet, file *ast.File, info *types.Info, res *splitAPIFileResult) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if isNotFoundErrorLit(n) && hasUnkeyedElements(n) {
				addResidual(res, filePath, fset, n, "notfound-keyed", "NotFoundError{...}", "manual: use keyed field NotFoundError{Label: ...}")
			}
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			isEntity, _ := isEntityReceiver(info, sel.X)
			if isEntity && strings.HasPrefix(sel.Sel.Name, "Query") && len(n.Args) == 0 {
				addResidual(res, filePath, fset, n, "entity-query", "entity.QueryX()", "manual: rewrite to client.<Entity>.QueryX(entity)")
			}
			if isEntity && sel.Sel.Name == "Update" && len(n.Args) == 0 {
				addResidual(res, filePath, fset, n, "entity-update", "entity.Update()", "manual: rewrite to client.<Entity>.UpdateOne(entity)")
			}
			if isEntity && sel.Sel.Name == "Unwrap" && len(n.Args) == 0 {
				addResidual(res, filePath, fset, n, "entity-unwrap", "entity.Unwrap()", "manual: rewrite to client.<Entity>.GetX(ctx, entity.ID)")
			}
			if sel.Sel.Name == "Client" && len(n.Args) == 0 {
				addResidual(res, filePath, fset, n, "mutation-client", "m.Client()", "manual: replace with ent.FromContext(ctx)")
			}
			if sel.Sel.Name == "Tx" && len(n.Args) == 0 {
				addResidual(res, filePath, fset, n, "mutation-tx", "m.Tx()", "manual: replace with ent.TxFromContext(ctx) and add nil check")
			}
		}
		return true
	})
}

func addResidual(res *splitAPIFileResult, file string, fset *token.FileSet, n ast.Node, pattern, snippet, advice string) {
	pos := fset.Position(n.Pos())
	res.Residuals = append(res.Residuals, splitAPIResidual{
		File:    file,
		Line:    pos.Line,
		Pattern: pattern,
		Snippet: snippet,
		Advice:  advice,
	})
}

func rewriteNotFoundError(lit *ast.CompositeLit) bool {
	if !isNotFoundErrorLit(lit) || len(lit.Elts) != 1 {
		return false
	}
	if _, ok := lit.Elts[0].(*ast.KeyValueExpr); ok {
		return false
	}
	lit.Elts[0] = &ast.KeyValueExpr{Key: ast.NewIdent("Label"), Value: lit.Elts[0]}
	return true
}

func isNotFoundErrorLit(lit *ast.CompositeLit) bool {
	switch t := lit.Type.(type) {
	case *ast.Ident:
		return t.Name == "NotFoundError"
	case *ast.SelectorExpr:
		return t.Sel.Name == "NotFoundError"
	default:
		return false
	}
}

func hasUnkeyedElements(lit *ast.CompositeLit) bool {
	for _, e := range lit.Elts {
		if _, ok := e.(*ast.KeyValueExpr); !ok {
			return true
		}
	}
	return false
}

func isGenerated(file *ast.File) bool {
	for _, cg := range file.Comments {
		if strings.Contains(cg.Text(), "Code generated") {
			return true
		}
	}
	return false
}

func parentMap(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return false
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		return true
	})
	return parents
}

func enclosingFunc(n ast.Node, parents map[ast.Node]ast.Node) ast.Node {
	for cur := n; cur != nil; cur = parents[cur] {
		switch cur.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return cur
		}
	}
	return nil
}

func detectEntAlias(file *ast.File) string {
	for _, imp := range file.Imports {
		pathLit, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if !strings.HasSuffix(pathLit, "/ent") {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				continue
			}
			return imp.Name.Name
		}
		return path.Base(pathLit)
	}
	return ""
}

func isContextType(t types.Type) bool {
	if t == nil {
		return false
	}
	return types.TypeString(t, nil) == "context.Context"
}

func isEntClientType(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "Client" && strings.HasSuffix(named.Obj().Pkg().Path(), "/ent")
}

func isEntityReceiver(info *types.Info, expr ast.Expr) (bool, string) {
	t := info.TypeOf(expr)
	if t == nil {
		return false, ""
	}
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false, ""
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false, ""
	}
	if !strings.Contains(named.Obj().Pkg().Path(), "/ent") {
		return false, ""
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return false, ""
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Name() == "ID" && f.Exported() {
			return true, named.Obj().Name()
		}
	}
	return false, ""
}

func isEntityArg(info *types.Info, expr ast.Expr) bool {
	ok, _ := isEntityReceiver(info, expr)
	return ok
}

func clientEntitySelector(clientName, entityName, method string) ast.Expr {
	return &ast.SelectorExpr{
		X: &ast.SelectorExpr{
			X:   ast.NewIdent(clientName),
			Sel: ast.NewIdent(entityName),
		},
		Sel: ast.NewIdent(method),
	}
}

func writeSplitAPIReport(path string, summary splitAPISummary) error {
	var b strings.Builder
	b.WriteString("# Split API Migration Report\n\n")
	b.WriteString(fmt.Sprintf("- Files scanned: %d\n", summary.FilesScanned))
	b.WriteString(fmt.Sprintf("- Files changed: %d\n", summary.FilesChanged))
	b.WriteString("\n## Applied Transformations\n\n")
	keys := make([]string, 0, len(summary.Transform))
	for k := range summary.Transform {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		b.WriteString("- none\n")
	}
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("- %s: %d\n", k, summary.Transform[k]))
	}
	b.WriteString("\n## Remaining Manual Fixes\n\n")
	if len(summary.Residuals) == 0 {
		b.WriteString("No residual migration patterns detected.\n")
	} else {
		for _, r := range summary.Residuals {
			b.WriteString(fmt.Sprintf("- `%s:%d` `%s` -> %s\n", r.File, r.Line, r.Pattern, r.Advice))
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func printSplitAPISummary(out interface{ Write([]byte) (int, error) }, cfg splitAPIMigrationConfig, summary splitAPISummary) {
	fmt.Fprintf(out, "split-api migration: scanned=%d changed=%d residual=%d\n", summary.FilesScanned, summary.FilesChanged, len(summary.Residuals))
	if !cfg.WriteChanges {
		fmt.Fprintln(out, "dry-run mode: no files were written (use --write to apply changes)")
	}
	if cfg.ReportPath != "" {
		fmt.Fprintf(out, "report written to %s\n", cfg.ReportPath)
	}
}
