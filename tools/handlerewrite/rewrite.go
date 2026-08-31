// Command handlerewrite mechanically rewrites generated-predicate/order call
// sites (e.g. escrow.NameEQ, escrow.HasParcelsWith, escrow.ByStatus) to the
// F/E handle form (escrow.F.Name.EQ, escrow.E.Parcels.HasWith,
// escrow.F.Status.Order) across a codebase, driven by a manifest describing
// which identifiers are fields/edges per entity package.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PkgEntry describes the fields and edges of one entity package, as
// produced by codegen into the manifest JSON.
type PkgEntry struct {
	Fields map[string]bool `json:"fields"`
	Edges  map[string]bool `json:"edges"`
	// NoBareEQ lists fields (present in Fields) that never got the classic
	// bare-equality shortcut (func Name(v T) predicate.X): enum fields,
	// whose bare name is the enum type, and fields whose name collides
	// with another package-level identifier (Label, Table, ...). A bare
	// pkg.Name reference for one of these is never a rewriteable call.
	NoBareEQ map[string]bool `json:"noBareEQ"`

	// ImportPath is the entity package's full import path (e.g.
	// "example.com/gen/escrow"), used by -chains mode to insert the
	// import when emitting a F/E handle reference. Unused in v1 (syntax)
	// mode, which resolves packages via the consumer file's own imports.
	ImportPath string `json:"importPath"`
	// Setters describes the assignment-side (Set/Add/Append/Clear) API for
	// each field/edge, keyed by its PascalCase struct-field name (matching
	// Fields/Edges). Used only by -chains mode. Unused in v1 mode.
	Setters map[string]SetterEntry `json:"setters"`
}

// SetterEntry describes one field or edge's assignment-side API, as
// produced by codegen into the manifest JSON's "setters" table. It governs
// which old setter method names (Set<F>, Add<F>, Set<E>ID, ...) a -chains
// mode call site may decompose into.
type SetterEntry struct {
	// Kind is "field" (F.<Name>.Set/...), "edgefield" (F.<Name>.Set/... but
	// the assignment routes through an edge), or "edge" (E.<Name>.SetID/...).
	Kind string `json:"kind"`
	// Nillable reports whether SetNillable<Name>(/ID) exists (field or
	// unique edge).
	Nillable bool `json:"nillable"`
	// CanAdd reports whether Add<Name>(v) exists (kind=field only).
	CanAdd bool `json:"canAdd"`
	// CanAppend reports whether Append<Name>(v) exists (kind=field only).
	CanAppend bool `json:"canAppend"`
	// Unique governs which edge assignment shape applies (kind=edge only):
	// true selects Set<Name>ID/SetNillable<Name>ID, false selects
	// Add<Name>IDs/Remove<Name>IDs.
	Unique bool `json:"unique"`
	// MethodBase is the base token real entc codegen uses in the old
	// Add<MethodBase>IDs/Remove<MethodBase>IDs method names for a
	// non-unique edge (kind=edge, unique=false only) — the edge's
	// StructField *singularized* (e.g. edge "Parcels" -> MethodBase
	// "Parcel" -> "AddParcelIDs"), per entc/gen's Edge.MutationAdd/
	// MutationRemove. Required for a real (Task-3-emitted) manifest;
	// Set<Name>ID/SetNillable<Name>ID/Clear<Name> never singularize and so
	// don't need it — they use Name (this entry's key) as-is.
	MethodBase string `json:"methodBase"`
}

// Manifest maps an entity package's import name (e.g. "escrow") to its
// field/edge membership.
type Manifest map[string]PkgEntry

// LoadManifest reads and decodes a manifest JSON file.
func LoadManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ops are the known predicate operation suffixes, matched longest-first so
// e.g. "NameNEQ" decomposes as field "Name" + op "NEQ", not "NameN" + "EQ".
var ops = func() []string {
	o := []string{
		"EQ", "NEQ", "In", "NotIn", "GT", "GTE", "LT", "LTE",
		"Contains", "HasPrefix", "HasSuffix", "EqualFold", "ContainsFold",
		"IsNil", "NotNil",
	}
	sort.Slice(o, func(i, j int) bool { return len(o[i]) > len(o[j]) })
	return o
}()

func isField(e PkgEntry, name string) bool { return name == "ID" || e.Fields[name] }
func isEdge(e PkgEntry, name string) bool  { return e.Edges[name] }

// reading is one valid decomposition of a selector name.
type reading struct{ kind, name, op string }

// String renders a reading the way a rewritten call site would read, e.g.
// "F.ContactListContactsCount.Order".
func (r reading) String() string { return r.kind + "." + r.name + "." + r.op }

// decomposeAll returns every valid reading of sel against e's manifest
// entry. Ordinarily there's at most one; more than one means sel is
// genuinely ambiguous (e.g. a field "XCount" and an edge "X" both exist, so
// "ByXCount" reads equally well as By<Field> or By<Edge>Count) and the
// caller must refuse to rewrite rather than guess.
func decomposeAll(sel string, e PkgEntry) []reading {
	var rs []reading
	add := func(kind, name, op string) { rs = append(rs, reading{kind, name, op}) }
	// Op suffix against a field (includes the implicit "ID" field).
	for _, o := range ops {
		field := strings.TrimSuffix(sel, o)
		if field == sel || field == "" {
			continue
		}
		if isField(e, field) {
			add("Field", field, o)
		}
	}
	// Has<Edge>() / Has<Edge>With(preds...)
	if rest, ok := strings.CutPrefix(sel, "Has"); ok && rest != "" {
		if edge, ok := strings.CutSuffix(rest, "With"); ok && edge != "" && isEdge(e, edge) {
			add("Edge", edge, "HasWith")
		}
		if isEdge(e, rest) {
			add("Edge", rest, "Has")
		}
	}
	// By<Field>(opts...) / By<Edge>Count(opts...) / By<Edge>(term, terms...)
	if rest, ok := strings.CutPrefix(sel, "By"); ok && rest != "" {
		if isField(e, rest) {
			add("Field", rest, "Order")
		}
		if edge, ok := strings.CutSuffix(rest, "Count"); ok && edge != "" && isEdge(e, edge) {
			add("Edge", edge, "OrderByCount")
		}
		if isEdge(e, rest) {
			add("Edge", rest, "OrderBy")
		}
	}
	// Bare equality: pkg.Name(v) / pkg.ID(v). Excludes fields listed in
	// NoBareEQ (enum fields, and fields colliding with a reserved
	// identifier) — old codegen never emitted a bare shortcut for those,
	// so a bare pkg.Name there is something else (an enum type reference,
	// the reserved identifier itself, ...), not a predicate call.
	if isField(e, sel) && !e.NoBareEQ[sel] {
		add("Field", sel, "EQ")
	}
	return rs
}

// decompose resolves sel to exactly one reading. ok is false either when no
// rule matches (cands is empty — nothing to rewrite) or when more than one
// rule matches (cands has 2+ entries — ambiguous; the caller reports it and
// leaves the selector untouched rather than picking one by rule order).
func decompose(sel string, e PkgEntry) (kind, name, op string, cands []reading, ok bool) {
	cands = decomposeAll(sel, e)
	if len(cands) != 1 {
		return "", "", "", cands, false
	}
	return cands[0].kind, cands[0].name, cands[0].op, cands, true
}

// importAliases maps each local identifier a file uses to refer to an
// import (its alias, or the import's base package name) to the manifest
// key it resolves to, for imports whose base package name is in manifest.
// When prefixes is non-empty, only imports whose full path starts with one
// of them are eligible (guards against a same-basename import from an
// unrelated tree, e.g. a vendored "escrow" package); an empty prefixes
// list is the permissive default (resolve by base name alone).
func importAliases(file *ast.File, manifest Manifest, prefixes []string) map[string]string {
	aliases := map[string]string{}
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if len(prefixes) > 0 && !hasAnyPrefix(p, prefixes) {
			continue
		}
		base := path.Base(p)
		if _, ok := manifest[base]; !ok {
			continue
		}
		local := base
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "_" || local == "." {
			continue
		}
		aliases[local] = base
	}
	return aliases
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// isGenerated reports whether file has a leading (pre-package-clause)
// comment containing "Code generated", the standard marker for generated
// Go files that a later codegen pass will overwrite anyway.
//
// ponytail: substring match, not the canonical
// "^// Code generated .* DO NOT EDIT\.$" regex — good enough since every
// codegen this tool targets (entc, gemini) emits that exact header; tighten
// if a handwritten file ever legitimately mentions the phrase.
func isGenerated(file *ast.File) bool {
	for _, cg := range file.Comments {
		if cg.Pos() > file.Package {
			break
		}
		if strings.Contains(cg.Text(), "Code generated") {
			return true
		}
	}
	return false
}

// shadowedNames returns the set of candidate names that decl rebinds as a
// local (non-import) declaration anywhere in its subtree: func/func-literal
// params, named results, receivers, ":=" and "var" locals, and range vars.
// It does not model exact block scoping — a name shadowed anywhere in decl
// (including inside a nested closure) is treated as shadowed everywhere in
// decl. That's deliberately over-conservative: a call site we leave alone
// surfaces as a compile error the next build; one we wrongly rewrite
// silently changes behavior.
func shadowedNames(decl ast.Decl) map[string]bool {
	shadowed := map[string]bool{}
	add := func(id *ast.Ident) {
		if id != nil && id.Name != "_" {
			shadowed[id.Name] = true
		}
	}
	addFieldList := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				add(n)
			}
		}
	}
	ast.Inspect(decl, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.FuncDecl:
			addFieldList(d.Recv)
			addFieldList(d.Type.Params)
			addFieldList(d.Type.Results)
		case *ast.FuncLit:
			addFieldList(d.Type.Params)
			addFieldList(d.Type.Results)
		case *ast.AssignStmt:
			if d.Tok == token.DEFINE {
				for _, lhs := range d.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						add(id)
					}
				}
			}
		case *ast.RangeStmt:
			if d.Tok == token.DEFINE {
				if id, ok := d.Key.(*ast.Ident); ok {
					add(id)
				}
				if id, ok := d.Value.(*ast.Ident); ok {
					add(id)
				}
			}
		case *ast.ValueSpec:
			for _, n := range d.Names {
				add(n)
			}
		}
		return true
	})
	return shadowed
}

// warnAmbiguous reports (to stderr) a selector that decomposed into more
// than one valid reading, identifying the call site and the competing
// readings so a human can resolve it. The selector is left unrewritten.
func warnAmbiguous(fset *token.FileSet, sel *ast.SelectorExpr, pkg string, cands []reading) {
	pos := fset.Position(sel.Pos())
	names := make([]string, len(cands))
	for i, c := range cands {
		names[i] = c.String()
	}
	fmt.Fprintf(os.Stderr, "handlerewrite: %s:%d: ambiguous rewrite for %s.%s — competing readings: %s — refusing to rewrite\n",
		pos.Filename, pos.Line, pkg, sel.Sel.Name, strings.Join(names, ", "))
}

// rewriteSelectors walks root, rewriting every pkg.Sel selector (call
// position or bare value position) whose pkg resolves to a manifest entry
// via aliases and whose Sel decomposes into exactly one field/edge + op,
// into the nested pkg.F.Name.Op / pkg.E.Name.Op form. A selector that
// decomposes ambiguously (more than one valid reading) is left untouched
// and reported via warnAmbiguous instead of guessed at. It reports whether
// any rewrite was made.
func rewriteSelectors(root ast.Node, aliases map[string]string, manifest Manifest, fset *token.FileSet) bool {
	if len(aliases) == 0 {
		return false
	}
	changed := false
	ast.Inspect(root, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		pkgKey, ok := aliases[ident.Name]
		if !ok {
			return true
		}
		kind, name, op, cands, ok := decompose(sel.Sel.Name, manifest[pkgKey])
		if !ok {
			if len(cands) > 1 {
				warnAmbiguous(fset, sel, ident.Name, cands)
			}
			return true
		}
		sel.X = &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(ident.Name), Sel: ast.NewIdent(kind)},
			Sel: ast.NewIdent(name),
		}
		sel.Sel = ast.NewIdent(op)
		changed = true
		return false
	})
	return changed
}

// rewriteAST rewrites file's call sites in place. Each top-level
// declaration gets its own shadow analysis (see shadowedNames), so a
// package name shadowed by a local in one function doesn't suppress
// rewrites in sibling declarations. It reports whether any rewrite was
// made.
func rewriteAST(file *ast.File, manifest Manifest, prefixes []string, fset *token.FileSet) bool {
	aliases := importAliases(file, manifest, prefixes)
	if len(aliases) == 0 {
		return false
	}
	changed := false
	for _, decl := range file.Decls {
		scoped := aliases
		if shadowed := shadowedNames(decl); len(shadowed) > 0 {
			scoped = map[string]string{}
			for local, key := range aliases {
				if !shadowed[local] {
					scoped[local] = key
				}
			}
			if len(scoped) == 0 {
				continue
			}
		}
		if rewriteSelectors(decl, scoped, manifest, fset) {
			changed = true
		}
	}
	return changed
}

// RewriteSource rewrites the call sites in one Go source file's contents.
// filename is used only for parse error messages / position info. prefixes
// restricts import resolution to matching import paths (see importAliases);
// pass nil for the permissive default. It returns the (possibly unchanged)
// source, whether any rewrite was made, and a parse error if src isn't
// valid Go.
func RewriteSource(filename string, src []byte, manifest Manifest, prefixes []string) ([]byte, bool, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	if isGenerated(file) {
		return src, false, nil
	}
	if !rewriteAST(file, manifest, prefixes, fset) {
		return src, false, nil
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, false, err
	}
	// Re-format canonically: nodes we synthesized carry no source
	// position, so the printer's output can have odd spacing; a second
	// gofmt-style pass over the printed text normalizes it.
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// ProcessFile rewrites path in place if it needs rewriting.
func ProcessFile(path string, manifest Manifest, prefixes []string) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	out, changed, err := RewriteSource(path, src, manifest, prefixes)
	if err != nil || !changed {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// ProcessDirs walks dirs recursively, rewriting every .go file in place
// (skipping testdata directories and generated files), and returns the
// list of files that were changed.
func ProcessDirs(dirs []string, manifest Manifest, prefixes []string) ([]string, error) {
	var changedFiles []string
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") {
				return nil
			}
			changed, err := ProcessFile(p, manifest, prefixes)
			if err != nil {
				return err
			}
			if changed {
				changedFiles = append(changedFiles, p)
			}
			return nil
		})
		if err != nil {
			return changedFiles, err
		}
	}
	return changedFiles, nil
}
