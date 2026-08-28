// -chains mode: a types-aware rewriter that decomposes old generated
// setter-chain calls (Set<F>, Add<F>, Set<E>ID, ...) on entc Create/Update
// builders into the F/E handle assignment form, folding consecutive
// rewritten calls into one .With(...). Unlike v1 (syntax mode), receiver
// types are resolved via go/packages type-checking, so there is no
// identifier-shadowing heuristic to get conservative about: a call only
// rewrites when its receiver's *static type* is a manifest-tracked builder,
// regardless of what any local variable happens to be named.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// setterReading is one decomposition of a builder method name against a
// manifest entity's setters table: "call <Handle>.<Name>.<Op>(args...)".
type setterReading struct {
	handle string // "F" or "E"
	name   string // the field/edge's PascalCase name
	op     string // the handle method to call: Set, SetNillable, Add, ...
}

func (r setterReading) String() string { return r.handle + "." + r.name + "." + r.op }

// decomposeSetter returns every valid reading of method against e's setters
// table. Given the manifest's setters map is keyed uniquely per
// field/edge name, most method names decompose at most one way; two
// readings are possible when a method name's op-suffix and no-suffix forms
// both resolve against different keys (e.g. "SetStatusID" reads as
// F.StatusID.Set if a field "StatusID" exists, and as E.Status.SetID if a
// unique edge "Status" exists) — genuinely ambiguous, and the caller must
// refuse to guess.
func decomposeSetter(method string, setters map[string]SetterEntry) []setterReading {
	var cands []setterReading
	add := func(handle, name, op string) { cands = append(cands, setterReading{handle, name, op}) }
	get := func(name string) (SetterEntry, bool) { e, ok := setters[name]; return e, ok }

	// Field / EdgeField forms: SetNillable<F> before Set<F> (prefix overlap).
	if name, ok := strings.CutPrefix(method, "SetNillable"); ok && name != "" {
		if e, exists := get(name); exists && e.Nillable && (e.Kind == "field" || e.Kind == "edgefield") {
			add("F", name, "SetNillable")
		}
	} else if name, ok := strings.CutPrefix(method, "Set"); ok && name != "" {
		if e, exists := get(name); exists && (e.Kind == "field" || e.Kind == "edgefield") {
			add("F", name, "Set")
		}
	}
	if name, ok := strings.CutPrefix(method, "Add"); ok && name != "" {
		if e, exists := get(name); exists && e.Kind == "field" && e.CanAdd {
			add("F", name, "Add")
		}
	}
	if name, ok := strings.CutPrefix(method, "Append"); ok && name != "" {
		if e, exists := get(name); exists && e.Kind == "field" && e.CanAppend {
			add("F", name, "Append")
		}
	}
	if name, ok := strings.CutPrefix(method, "Clear"); ok && name != "" {
		if e, exists := get(name); exists && (e.Kind == "field" || e.Kind == "edgefield") {
			add("F", name, "Clear")
		}
	}

	// Edge forms.
	if name, ok := cutPrefixSuffix(method, "SetNillable", "ID"); ok {
		if e, exists := get(name); exists && e.Kind == "edge" && e.Unique && e.Nillable {
			add("E", name, "SetNillableID")
		}
	}
	if name, ok := cutPrefixSuffix(method, "Set", "ID"); ok {
		if e, exists := get(name); exists && e.Kind == "edge" && e.Unique {
			add("E", name, "SetID")
		}
	}
	if name, ok := cutPrefixSuffix(method, "Add", "IDs"); ok {
		if e, exists := get(name); exists && e.Kind == "edge" && !e.Unique {
			add("E", name, "AddIDs")
		}
	}
	if name, ok := cutPrefixSuffix(method, "Remove", "IDs"); ok {
		if e, exists := get(name); exists && e.Kind == "edge" && !e.Unique {
			add("E", name, "RemoveIDs")
		}
	}
	if name, ok := strings.CutPrefix(method, "Clear"); ok && name != "" {
		if e, exists := get(name); exists && e.Kind == "edge" {
			add("E", name, "Clear")
		}
	}
	return cands
}

// cutPrefixSuffix strips both prefix and suffix from s, returning the
// middle and true only if s has both, the middle is non-empty, and prefix
// and suffix don't overlap (len(prefix)+len(suffix) <= len(s)).
func cutPrefixSuffix(s, prefix, suffix string) (string, bool) {
	rest, ok := strings.CutPrefix(s, prefix)
	if !ok {
		return "", false
	}
	rest, ok = strings.CutSuffix(rest, suffix)
	if !ok || rest == "" {
		return "", false
	}
	return rest, true
}

// splitBuilderType reports whether name is a generated builder type name
// (<Entity>Create, <Entity>Update, or <Entity>UpdateOne) and, if so, the
// entity name. "UpdateOne" is checked before "Update" (longer suffix
// first), since every "UpdateOne" name also ends in "One" but NOT in the
// bare "Update" suffix's usual sense — checked explicitly to avoid ever
// trimming "UpdateOne" down to "...UpdateOne" minus "Update" = "...One".
func splitBuilderType(name string) (entity string, ok bool) {
	switch {
	case strings.HasSuffix(name, "UpdateOne"):
		return strings.TrimSuffix(name, "UpdateOne"), true
	case strings.HasSuffix(name, "Update"):
		return strings.TrimSuffix(name, "Update"), true
	case strings.HasSuffix(name, "Create"):
		return strings.TrimSuffix(name, "Create"), true
	}
	return "", false
}

// namedType unwraps a (possibly pointer) type down to its *types.Named, or
// nil if t isn't a (pointer to a) named type.
func namedType(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, _ := t.(*types.Named)
	return n
}

// rewriteCtx holds the per-file state for one -chains rewrite pass: the
// type info to resolve receivers against, the manifest and pkgprefix guard,
// and a cache of import aliases already resolved/added in this file.
type rewriteCtx struct {
	fset      *token.FileSet
	info      *types.Info
	manifest  Manifest
	prefixes  []string
	file      *ast.File
	filename  string
	aliases   map[string]string // importPath -> local alias, resolved lazily
	changed   bool
	ambiguous bool
}

func (rc *rewriteCtx) typeOf(e ast.Expr) types.Type {
	if tv, ok := rc.info.Types[e]; ok {
		return tv.Type
	}
	return nil
}

// builderEntry reports the manifest entry for t, if t is a (pointer to a)
// generated builder type (<Entity>Create/Update/UpdateOne) whose package
// path passes the -pkgprefix guard and whose entity has a manifest entry
// with a non-nil setters table.
func (rc *rewriteCtx) builderEntry(t types.Type) (PkgEntry, bool) {
	named := namedType(t)
	if named == nil {
		return PkgEntry{}, false
	}
	obj := named.Obj()
	if obj.Pkg() == nil {
		return PkgEntry{}, false
	}
	if len(rc.prefixes) > 0 && !hasAnyPrefix(obj.Pkg().Path(), rc.prefixes) {
		return PkgEntry{}, false
	}
	entity, ok := splitBuilderType(obj.Name())
	if !ok {
		return PkgEntry{}, false
	}
	entry, ok := rc.manifest[strings.ToLower(entity)]
	if !ok || entry.Setters == nil {
		return PkgEntry{}, false
	}
	return entry, true
}

// ensureImport returns the local identifier this file uses to refer to
// importPath, adding the import (unaliased) if the file doesn't have it
// yet. An existing import's alias (explicit or, absent one, the import's
// base package name) is reused rather than duplicated.
func (rc *rewriteCtx) ensureImport(importPath string) string {
	if alias, ok := rc.aliases[importPath]; ok {
		return alias
	}
	for _, imp := range rc.file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != importPath {
			continue
		}
		alias := path.Base(p)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		rc.aliases[importPath] = alias
		return alias
	}
	astutil.AddImport(rc.fset, rc.file, importPath)
	alias := path.Base(importPath)
	rc.aliases[importPath] = alias
	return alias
}

func (rc *rewriteCtx) warnAmbiguous(sel *ast.SelectorExpr, cands []setterReading) {
	pos := rc.fset.Position(sel.Pos())
	names := make([]string, len(cands))
	for i, c := range cands {
		names[i] = c.String()
	}
	fmt.Fprintf(os.Stderr, "handlerewrite: %s:%d: ambiguous chain rewrite for .%s(...) — competing readings: %s — refusing to rewrite\n",
		pos.Filename, pos.Line, sel.Sel.Name, strings.Join(names, ", "))
	rc.ambiguous = true
}

// exprResult is the outcome of processExpr on one expression.
type exprResult struct {
	expr ast.Expr
	// changed reports whether anything under expr (including expr itself)
	// was rewritten.
	changed bool
	// openWith, if non-nil, is expr's own *ast.CallExpr — a synthesized
	// `.With(...)` call that a caller decomposing the *next* link in the
	// same chain may extend in place (fold) rather than wrap in a new
	// `.With(...)`. nil means the chain is "closed" here: expr's outermost
	// call was left as an ordinary call (not a decomposed setter), so the
	// next link (if any) must start a fresh `.With(...)`.
	openWith *ast.CallExpr
}

// buildHandleCall renders `<pkgAlias>.<r.handle>.<r.name>.<r.op>(args...)`.
func buildHandleCall(pkgAlias string, r setterReading, args []ast.Expr) *ast.CallExpr {
	sel := &ast.SelectorExpr{
		X: &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(pkgAlias), Sel: ast.NewIdent(r.handle)},
			Sel: ast.NewIdent(r.name),
		},
		Sel: ast.NewIdent(r.op),
	}
	return &ast.CallExpr{Fun: sel, Args: args}
}

// processExpr recursively rewrites e (bottom-up: receivers and arguments
// before the call itself), folding consecutive decomposed setter calls in
// one method chain into a single `.With(...)`.
func (rc *rewriteCtx) processExpr(e ast.Expr) exprResult {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return exprResult{expr: e}
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		// Not a method call (e.g. a plain function call) — still walk args
		// for nested chains, e.g. Must(client.X.Create().SetName(v)...).
		changed := false
		for i, a := range call.Args {
			r := rc.processExpr(a)
			if r.changed {
				call.Args[i] = r.expr
				changed = true
			}
		}
		return exprResult{expr: call, changed: changed}
	}

	origRecv := sel.X
	recvType := rc.typeOf(origRecv)
	recvRes := rc.processExpr(origRecv)
	sel.X = recvRes.expr

	argsChanged := false
	for i, a := range call.Args {
		r := rc.processExpr(a)
		if r.changed {
			call.Args[i] = r.expr
			argsChanged = true
		}
	}

	if entry, ok := rc.builderEntry(recvType); ok {
		switch cands := decomposeSetter(sel.Sel.Name, entry.Setters); len(cands) {
		case 1:
			alias := rc.ensureImport(entry.ImportPath)
			argExpr := buildHandleCall(alias, cands[0], call.Args)
			if recvRes.openWith != nil {
				recvRes.openWith.Args = append(recvRes.openWith.Args, argExpr)
				return exprResult{expr: recvRes.expr, changed: true, openWith: recvRes.openWith}
			}
			withCall := &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: recvRes.expr, Sel: ast.NewIdent("With")},
				Args: []ast.Expr{argExpr},
			}
			return exprResult{expr: withCall, changed: true, openWith: withCall}
		case 0:
			// Not a setter name (Where/Save/Exec/OnConflict/Mutation/...):
			// an ordinary call. Falls through — closes any open fold.
		default:
			rc.warnAmbiguous(sel, cands)
			// Refuse to rewrite this call; falls through as ordinary.
		}
	}

	call.Fun = sel
	return exprResult{expr: call, changed: recvRes.changed || argsChanged}
}

// RewriteChainsFile rewrites file's setter-chain call sites in place,
// against pkg's type info. It reports whether any rewrite was made.
func RewriteChainsFile(fset *token.FileSet, info *types.Info, file *ast.File, filename string, manifest Manifest, prefixes []string) bool {
	if isGenerated(file) {
		return false
	}
	rc := &rewriteCtx{fset: fset, info: info, manifest: manifest, prefixes: prefixes, file: file, filename: filename, aliases: map[string]string{}}
	astutil.Apply(file, func(cur *astutil.Cursor) bool {
		call, ok := cur.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		res := rc.processExpr(call)
		if res.changed {
			cur.Replace(res.expr)
			rc.changed = true
		}
		return false
	}, nil)
	return rc.changed
}

// ProcessPackages loads the Go packages matching patterns (rooted at dir;
// dir == "" means the current working directory) and rewrites every
// non-generated file's setter-chain call sites in place, returning the
// list of files changed. A package that fails to load or type-check is
// reported and skipped (its files left untouched) rather than aborting the
// whole run — consistent with v1's per-file error handling, since a
// migration over many packages shouldn't be all-or-nothing.
func ProcessPackages(dir string, patterns []string, manifest Manifest, prefixes []string) ([]string, error) {
	cfg := &packages.Config{
		Dir: dir,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	var changedFiles []string
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				fmt.Fprintf(os.Stderr, "handlerewrite: package %s: %v\n", pkg.PkgPath, e)
			}
			continue
		}
		for i, file := range pkg.Syntax {
			if i >= len(pkg.CompiledGoFiles) {
				continue
			}
			filename := pkg.CompiledGoFiles[i]
			if seen[filename] {
				continue
			}
			seen[filename] = true
			if !RewriteChainsFile(pkg.Fset, pkg.TypesInfo, file, filename, manifest, prefixes) {
				continue
			}
			var buf bytes.Buffer
			if err := format.Node(&buf, pkg.Fset, file); err != nil {
				fmt.Fprintf(os.Stderr, "handlerewrite: format %s: %v\n", filename, err)
				continue
			}
			out, err := format.Source(buf.Bytes())
			if err != nil {
				fmt.Fprintf(os.Stderr, "handlerewrite: gofmt %s: %v\n", filename, err)
				continue
			}
			if err := os.WriteFile(filename, out, 0o644); err != nil {
				return changedFiles, err
			}
			changedFiles = append(changedFiles, filename)
		}
	}
	return changedFiles, nil
}
