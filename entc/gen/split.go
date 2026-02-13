// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// SplitMode defines the strategy used for file splitting.
type SplitMode string

const (
	// SplitModeType splits declarations into deterministic files by schema type.
	SplitModeType   SplitMode = "type"
	splitFileMarker           = "// entc:split-file"
)

// SplitConfig configures optional splitting of generated Go assets.
//
// If SplitConfig is nil, generated files keep their existing monolithic layout.
type SplitConfig struct {
	// Mode controls the split strategy. Defaults to SplitModeType.
	Mode SplitMode

	// Include narrows down which files should be split by glob patterns that match
	// template names and generated output file names. If empty, only core ent
	// templates are eligible for splitting.
	Include []string

	// Exclude skips files by glob patterns that match template names and generated
	// output file names.
	Exclude []string
}

// SplitOption configures SplitConfig using functional options.
type SplitOption func(*SplitConfig) error

// SplitByType configures file splitting by schema type.
func SplitByType() SplitOption {
	return func(cfg *SplitConfig) error {
		cfg.Mode = SplitModeType
		return nil
	}
}

// SplitInclude appends include glob patterns for split selection.
func SplitInclude(patterns ...string) SplitOption {
	return func(cfg *SplitConfig) error {
		cfg.Include = append(cfg.Include, patterns...)
		return nil
	}
}

// SplitExclude appends exclude glob patterns for split selection.
func SplitExclude(patterns ...string) SplitOption {
	return func(cfg *SplitConfig) error {
		cfg.Exclude = append(cfg.Exclude, patterns...)
		return nil
	}
}

// Normalize applies defaults and validates split configuration.
func (c *SplitConfig) Normalize() error {
	if c == nil {
		return nil
	}
	if c.Mode == "" {
		c.Mode = SplitModeType
	}
	if c.Mode != SplitModeType {
		return fmt.Errorf("entc/gen: invalid split mode %q: allowed values are %q", c.Mode, SplitModeType)
	}
	include, err := normalizeSplitPatterns("include", c.Include)
	if err != nil {
		return err
	}
	exclude, err := normalizeSplitPatterns("exclude", c.Exclude)
	if err != nil {
		return err
	}
	c.Include = include
	c.Exclude = exclude
	return nil
}

func normalizeSplitPatterns(kind string, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, raw := range patterns {
		pattern := filepath.ToSlash(strings.TrimSpace(raw))
		if pattern == "" {
			return nil, fmt.Errorf("entc/gen: split %s pattern cannot be empty", kind)
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("entc/gen: invalid split %s pattern %q: %w", kind, pattern, err)
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		normalized = append(normalized, pattern)
	}
	return normalized, nil
}

func (c *Config) normalizeSplit() error {
	if c.Split == nil {
		return nil
	}
	return c.Split.Normalize()
}

type splitAssetMeta struct {
	Template string
	Output   string
	Core     bool
	Origin   string
}

func applySplit(g *Graph, assets *assets) error {
	if g.Config.Split == nil {
		return nil
	}
	cfg := *g.Config.Split
	if err := cfg.Normalize(); err != nil {
		return err
	}
	g.Config.Split = &cfg
	switch cfg.Mode {
	case SplitModeType:
		return splitAssetsByType(g, assets, cfg)
	default:
		return fmt.Errorf("entc/gen: unexpected split mode %q", cfg.Mode)
	}
}

func splitAssetsByType(g *Graph, assets *assets, cfg SplitConfig) error {
	if len(assets.files) == 0 {
		return nil
	}
	matchers := splitTypes(g.Nodes)
	if len(matchers) == 0 {
		return nil
	}
	next := make(map[string]assetFile, len(assets.files))
	for path, file := range assets.files {
		if !strings.HasSuffix(path, ".go") || !cfg.shouldSplit(file.meta) {
			next[path] = file
			continue
		}
		splitFiles, ok, err := splitGoByType(path, file, matchers)
		if err != nil {
			return fmt.Errorf("split %q by type: %w", path, err)
		}
		if !ok {
			next[path] = file
			continue
		}
		for name, f := range splitFiles {
			next[name] = f
		}
	}
	assets.files = next
	return nil
}

func (c SplitConfig) shouldSplit(meta splitAssetMeta) bool {
	if meta.Output == "" {
		return false
	}
	include := false
	if len(c.Include) == 0 {
		include = meta.Core
	} else {
		include = matchSplitTarget(c.Include, meta)
	}
	if !include {
		return false
	}
	return !matchSplitTarget(c.Exclude, meta)
}

func matchSplitTarget(patterns []string, meta splitAssetMeta) bool {
	if len(patterns) == 0 {
		return false
	}
	output := filepath.ToSlash(meta.Output)
	base := path.Base(output)
	stem := strings.TrimSuffix(base, ".go")
	targets := []string{meta.Template, output, base, stem}
	for _, pattern := range patterns {
		for _, target := range targets {
			ok, err := path.Match(pattern, target)
			if err != nil {
				continue
			}
			if ok {
				return true
			}
		}
	}
	return false
}

type splitType struct {
	Name       string
	Prefix     string
	LowerName  string
	LowerPref  string
	PartSuffix string
}

func splitTypes(nodes []*Type) []splitType {
	types := make([]splitType, 0, len(nodes))
	for _, node := range nodes {
		suffix := splitPartSuffix(node.PackageDir())
		if suffix == "" {
			continue
		}
		types = append(types, splitType{
			Name:       node.Name,
			Prefix:     node.PackageDir(),
			LowerName:  strings.ToLower(node.Name),
			LowerPref:  strings.ToLower(node.PackageDir()),
			PartSuffix: suffix,
		})
	}
	return types
}

func splitPartSuffix(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func splitGoByType(path string, file assetFile, types []splitType) (map[string]assetFile, bool, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, file.content, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	prologue, err := splitPrologue(file.content, fset, node)
	if err != nil {
		return nil, false, err
	}
	importDecls, baseDecls, typedDecls := splitDecls(file.content, fset, node, types)
	if len(typedDecls) == 0 {
		return nil, false, nil
	}

	originPath := file.meta.Origin
	if originPath == "" {
		originPath = path
	}
	originOutput := file.meta.Output
	if originOutput == "" {
		originOutput = filepath.Base(originPath)
	}
	originOutput = filepath.ToSlash(originOutput)

	basePathPrefix := strings.TrimSuffix(originPath, ".go")
	baseOutputPrefix := strings.TrimSuffix(originOutput, ".go")
	basePath := basePathPrefix + "_base.go"
	baseOutput := baseOutputPrefix + "_base.go"

	files := make(map[string]assetFile, len(typedDecls)+1)
	baseMeta := file.meta
	baseMeta.Output = baseOutput
	baseMeta.Origin = originPath
	files[basePath] = assetFile{
		content: splitFileContent(prologue, importDecls, baseDecls, true),
		meta:    baseMeta,
	}

	keys := make([]string, 0, len(typedDecls))
	for key := range typedDecls {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		partPath := fmt.Sprintf("%s_%s.go", basePathPrefix, key)
		partMeta := file.meta
		partMeta.Output = fmt.Sprintf("%s_%s.go", baseOutputPrefix, key)
		partMeta.Origin = originPath
		files[partPath] = assetFile{
			content: splitFileContent(prologue, importDecls, typedDecls[key], true),
			meta:    partMeta,
		}
	}
	return files, true, nil
}

func splitPrologue(src []byte, fset *token.FileSet, file *ast.File) ([]byte, error) {
	start := positionOffset(fset, file.Package)
	if start < 0 || start >= len(src) {
		return nil, fmt.Errorf("invalid package position")
	}
	end := bytes.IndexByte(src[start:], '\n')
	if end == -1 {
		end = len(src)
	} else {
		end += start + 1
	}
	out := bytes.TrimRight(src[:end], "\n")
	out = append(append([]byte(nil), out...), '\n', '\n')
	return out, nil
}

func splitDecls(src []byte, fset *token.FileSet, file *ast.File, types []splitType) ([][]byte, [][]byte, map[string][][]byte) {
	imports := make([][]byte, 0, 1)
	base := make([][]byte, 0, len(file.Decls))
	typed := make(map[string][][]byte)
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			if segment := splitDeclSource(src, fset, decl); len(segment) > 0 {
				imports = append(imports, segment)
			}
			continue
		}
		segment := splitDeclSource(src, fset, decl)
		if len(segment) == 0 {
			continue
		}
		if key := splitDeclOwner(decl, types); key != "" {
			typed[key] = append(typed[key], segment)
			continue
		}
		base = append(base, segment)
	}
	return imports, base, typed
}

func splitDeclSource(src []byte, fset *token.FileSet, decl ast.Decl) []byte {
	start := positionOffset(fset, decl.Pos())
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Doc != nil {
			start = positionOffset(fset, d.Doc.Pos())
		}
	case *ast.GenDecl:
		if d.Doc != nil {
			start = positionOffset(fset, d.Doc.Pos())
		}
	}
	end := positionOffset(fset, decl.End())
	if start < 0 || end < 0 || end > len(src) || start >= end {
		return nil
	}
	trimmed := bytes.TrimSpace(src[start:end])
	if len(trimmed) == 0 {
		return nil
	}
	return append([]byte(nil), trimmed...)
}

func splitDeclOwner(decl ast.Decl, types []splitType) string {
	idents := splitDeclIdentifiers(decl)
	if len(idents) == 0 {
		return ""
	}
	matches := make(map[string]struct{})
	for _, ident := range idents {
		key, ok := splitIdentType(ident, types)
		if !ok {
			continue
		}
		matches[key] = struct{}{}
	}
	if len(matches) != 1 {
		return ""
	}
	for key := range matches {
		return key
	}
	return ""
}

func splitDeclIdentifiers(decl ast.Decl) []string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		idents := []string{d.Name.Name}
		if d.Recv != nil && len(d.Recv.List) > 0 {
			idents = append(idents, splitExprIdent(d.Recv.List[0].Type))
		}
		return idents
	case *ast.GenDecl:
		idents := make([]string, 0, len(d.Specs))
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				idents = append(idents, s.Name.Name)
			case *ast.ValueSpec:
				for _, name := range s.Names {
					idents = append(idents, name.Name)
				}
			}
		}
		return idents
	default:
		return nil
	}
}

func splitExprIdent(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return splitExprIdent(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.IndexExpr:
		return splitExprIdent(e.X)
	case *ast.IndexListExpr:
		return splitExprIdent(e.X)
	default:
		return ""
	}
}

func splitIdentType(ident string, types []splitType) (string, bool) {
	if ident == "" {
		return "", false
	}
	lower := strings.ToLower(ident)
	bestKey := ""
	bestScore := 0
	for _, t := range types {
		score := splitMatchScore(ident, lower, t)
		switch {
		case score == 0:
		case score > bestScore:
			bestScore = score
			bestKey = t.PartSuffix
		case score == bestScore && bestKey != t.PartSuffix:
			bestKey = ""
		}
	}
	if bestScore == 0 || bestKey == "" {
		return "", false
	}
	return bestKey, true
}

func splitMatchScore(ident, lower string, t splitType) int {
	switch {
	case splitHasPrefix(ident, t.Name):
		return 200 + len(t.Name)
	case splitHasPrefix(ident, t.Prefix):
		return 100 + len(t.Prefix)
	case splitHasPrefix(lower, t.LowerName):
		return 50 + len(t.LowerName)
	case splitHasPrefix(lower, t.LowerPref):
		return 25 + len(t.LowerPref)
	default:
		return 0
	}
}

func splitHasPrefix(ident, prefix string) bool {
	return splitHasBoundaryPrefix(ident, prefix) || splitHasBoundaryPrefix(ident, prefix+"s")
}

func splitHasBoundaryPrefix(ident, prefix string) bool {
	if prefix == "" || !strings.HasPrefix(ident, prefix) {
		return false
	}
	if len(ident) == len(prefix) {
		return true
	}
	next := ident[len(prefix)]
	switch {
	case next == '_':
		return true
	case next >= '0' && next <= '9':
		return true
	case next >= 'A' && next <= 'Z':
		return true
	default:
		return false
	}
}

func splitFileContent(prologue []byte, imports, decls [][]byte, split bool) []byte {
	size := len(prologue)
	if split {
		size += len(splitFileMarker) + 2
	}
	for _, imp := range imports {
		size += len(imp) + 2
	}
	for _, decl := range decls {
		size += len(decl) + 2
	}
	out := make([]byte, 0, size+1)
	out = append(out, prologue...)
	if split {
		out = append(out, splitFileMarker...)
		out = append(out, '\n', '\n')
	}
	for i, imp := range imports {
		if i > 0 {
			out = append(out, '\n', '\n')
		}
		out = append(out, imp...)
	}
	if len(imports) > 0 && len(decls) > 0 {
		out = append(out, '\n', '\n')
	}
	for i, decl := range decls {
		if i > 0 {
			out = append(out, '\n', '\n')
		}
		out = append(out, decl...)
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out
}

func positionOffset(fset *token.FileSet, pos token.Pos) int {
	if !pos.IsValid() {
		return -1
	}
	return fset.Position(pos).Offset
}

func (a assets) cleanupSplit() error {
	families := make(map[string]map[string]struct{})
	generated := make(map[string]struct{}, len(a.files))
	for path, file := range a.files {
		generated[path] = struct{}{}
		origin := file.meta.Origin
		if origin == "" {
			origin = path
		}
		if !strings.HasSuffix(origin, ".go") {
			continue
		}
		keep := families[origin]
		if keep == nil {
			keep = make(map[string]struct{})
			families[origin] = keep
		}
		keep[path] = struct{}{}
	}
	for origin, keep := range families {
		if err := cleanupSplitFamily(origin, keep, generated); err != nil {
			return err
		}
	}
	return nil
}

func cleanupSplitFamily(origin string, keep map[string]struct{}, generated map[string]struct{}) error {
	prefix := strings.TrimSuffix(origin, ".go")
	patterns := []string{
		origin,
		prefix + "_base.go",
		prefix + "_part*.go",
		prefix + "_*.go",
	}
	seen := make(map[string]struct{})
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("glob split cleanup pattern %q: %w", pattern, err)
		}
		for _, match := range matches {
			seen[match] = struct{}{}
		}
	}
	for stale := range seen {
		if _, ok := keep[stale]; ok {
			continue
		}
		if _, ok := generated[stale]; ok {
			continue
		}
		if !isSplitCleanupCandidate(origin, stale) {
			continue
		}
		if err := os.Remove(stale); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale split file %q: %w", stale, err)
		}
	}
	return nil
}

func isSplitCleanupCandidate(origin, path string) bool {
	if path == origin {
		return true
	}
	originBase := strings.TrimSuffix(filepath.Base(origin), ".go")
	name := filepath.Base(path)
	if !strings.HasPrefix(name, originBase+"_") || !strings.HasSuffix(name, ".go") {
		return false
	}
	if strings.HasSuffix(name, "_base.go") || isLegacySplitPartFile(originBase, name) {
		return true
	}
	return hasSplitFileMarker(path)
}

func isLegacySplitPartFile(originBase, name string) bool {
	base, ok := strings.CutSuffix(name, ".go")
	if !ok {
		return false
	}
	part, ok := strings.CutPrefix(base, originBase+"_part")
	if !ok || part == "" {
		return false
	}
	for _, r := range part {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func hasSplitFileMarker(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(content, []byte(splitFileMarker))
}

func isCoreGraphTemplate(tmpl GraphTemplate) bool {
	for _, t := range GraphTemplates {
		if t.Name == tmpl.Name && t.Format == tmpl.Format {
			return true
		}
	}
	for _, f := range allFeatures {
		for _, t := range f.GraphTemplates {
			if t.Name == tmpl.Name && t.Format == tmpl.Format {
				return true
			}
		}
	}
	return false
}
