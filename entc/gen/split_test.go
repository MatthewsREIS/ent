// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"

	"github.com/stretchr/testify/require"
)

func TestConfigSplitGenerationMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		want    SplitMode
		wantErr string
	}{
		{
			name: "default legacy when split disabled",
			cfg:  Config{},
			want: SplitModeLegacy,
		},
		{
			name: "split enabled defaults to compat",
			cfg: Config{
				Features: []Feature{FeatureSplitPackages},
			},
			want: SplitModeCompat,
		},
		{
			name: "split enabled coerces legacy to compat",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeLegacy,
			},
			want: SplitModeCompat,
		},
		{
			name: "split enabled supports compat",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeCompat,
			},
			want: SplitModeCompat,
		},
		{
			name: "split enabled supports native",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeNative,
			},
			want: SplitModeNative,
		},
		{
			name: "split mode ignored when split disabled",
			cfg: Config{
				SplitMode: SplitModeNative,
			},
			want: SplitModeLegacy,
		},
		{
			name: "invalid split mode",
			cfg: Config{
				SplitMode: SplitMode("wat"),
			},
			wantErr: `invalid split mode "wat"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.cfg.SplitGenerationMode()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGraphGenerationTemplates(t *testing.T) {
	origLegacyType := legacyTypeTemplates
	origLegacyGraph := legacyGraphTemplates
	origCompatType := splitCompatTypeTemplates
	origCompatGraph := splitCompatGraphTemplates
	origNativeType := splitNativeTypeTemplates
	origNativeGraph := splitNativeGraphTemplates
	t.Cleanup(func() {
		legacyTypeTemplates = origLegacyType
		legacyGraphTemplates = origLegacyGraph
		splitCompatTypeTemplates = origCompatType
		splitCompatGraphTemplates = origCompatGraph
		splitNativeTypeTemplates = origNativeType
		splitNativeGraphTemplates = origNativeGraph
	})

	legacyTypeTemplates = []TypeTemplate{{Name: "legacy/type"}}
	legacyGraphTemplates = []GraphTemplate{{Name: "legacy/graph"}}
	splitCompatTypeTemplates = []TypeTemplate{{Name: "compat/type"}}
	splitCompatGraphTemplates = []GraphTemplate{{Name: "compat/graph"}}
	splitNativeTypeTemplates = []TypeTemplate{{Name: "native/type"}}
	splitNativeGraphTemplates = []GraphTemplate{{Name: "native/graph"}}

	tests := []struct {
		name      string
		cfg       Config
		wantType  string
		wantGraph string
		wantErr   string
	}{
		{
			name:      "legacy selection when split disabled",
			cfg:       Config{},
			wantType:  "legacy/type",
			wantGraph: "legacy/graph",
		},
		{
			name: "compat selection when split enabled",
			cfg: Config{
				Features: []Feature{FeatureSplitPackages},
			},
			wantType:  "compat/type",
			wantGraph: "compat/graph",
		},
		{
			name: "native selection when split mode native",
			cfg: Config{
				Features:  []Feature{FeatureSplitPackages},
				SplitMode: SplitModeNative,
			},
			wantType:  "native/type",
			wantGraph: "native/graph",
		},
		{
			name: "invalid mode returns error",
			cfg: Config{
				SplitMode: SplitMode("bad"),
			},
			wantErr: `invalid split mode "bad"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			g := &Graph{Config: &tt.cfg}
			ttmpl, gtmpl, err := g.generationTemplates()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantType, ttmpl[0].Name)
			require.Equal(t, tt.wantGraph, gtmpl[0].Name)
		})
	}
}

func TestGraphSplitTypePackages(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package:  "entc/gen",
		Target:   target,
		Storage:  drivers[0],
		IDType:   &field.TypeInfo{Type: field.TypeInt},
		Features: []Feature{FeatureSplitPackages},
	}, []*load.Schema{
		{
			Name: "User",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
		},
		{
			Name: "Pet",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
		},
	}...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	internalPrefix := pathpkg.Join(graph.Config.Package, "internal", "split", "type")
	files := []string{"model.go", "query.go", "create.go", "update.go", "delete.go", "mutation.go"}
	for _, n := range graph.Nodes {
		for _, file := range files {
			path := filepath.Join(target, "internal", "split", "type", n.PackageDir(), file)
			_, err := os.Stat(path)
			require.NoErrorf(t, err, "expected split internal file %s", path)

			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			require.NoError(t, err)
			for _, imp := range f.Imports {
				impPath, err := strconv.Unquote(imp.Path.Value)
				require.NoError(t, err)
				require.Falsef(
					t,
					strings.HasPrefix(impPath, internalPrefix),
					"unexpected split sibling import %q in %s",
					impPath,
					path,
				)
			}
		}
	}
}

func TestGraphSplitBridgeFacade(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package:  "entc/gen",
		Target:   target,
		Storage:  drivers[0],
		IDType:   &field.TypeInfo{Type: field.TypeInt},
		Features: []Feature{FeatureSplitPackages},
	}, []*load.Schema{
		{
			Name: "User",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
			Edges: []*load.Edge{
				{Name: "pets", Type: "Pet"},
			},
		},
		{
			Name: "Pet",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
			Edges: []*load.Edge{
				{Name: "owner", Type: "User", RefName: "pets", Inverse: true, Unique: true},
			},
		},
	}...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	bridgePath := filepath.Join(target, "internal", "split", "bridge", "bridge.go")
	bridgeCode, err := os.ReadFile(bridgePath)
	require.NoError(t, err)
	require.Contains(t, string(bridgeCode), "func SetNeighbors(")
	require.Contains(t, string(bridgeCode), "func Neighbors(")

	userQueryCode, err := os.ReadFile(filepath.Join(target, "user_query.go"))
	require.NoError(t, err)
	require.Contains(t, string(userQueryCode), "entbridge.SetNeighbors(")

	clientCode, err := os.ReadFile(filepath.Join(target, "client.go"))
	require.NoError(t, err)
	require.Contains(t, string(clientCode), "entbridge.Neighbors(")
}

func TestGraphSplitEntQLIsolation(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package: "entc/gen",
		Target:  target,
		Storage: drivers[0],
		IDType:  &field.TypeInfo{Type: field.TypeInt},
		SplitMode: SplitModeCompat,
		Features: []Feature{
			FeatureSplitPackages,
			FeatureEntQL,
		},
	}, []*load.Schema{
		{
			Name: "User",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
		},
	}...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	rootCode, err := os.ReadFile(filepath.Join(target, "entql.go"))
	require.NoError(t, err)
	require.Contains(t, string(rootCode), "entqlinternal.NewUserFilter(")
	require.Contains(t, string(rootCode), "type UserFilter = entqlinternal.UserFilter")
	require.NotContains(t, string(rootCode), "var schemaGraph =")

	internalCode, err := os.ReadFile(filepath.Join(target, "internal", "split", "entql", "entql.go"))
	require.NoError(t, err)
	require.Contains(t, string(internalCode), "var schemaGraph =")
	require.Contains(t, string(internalCode), "type UserFilter struct {")
}

func TestGraphSplitNativeNoBridgeFacade(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package:   "entc/gen",
		Target:    target,
		Storage:   drivers[0],
		IDType:    &field.TypeInfo{Type: field.TypeInt},
		SplitMode: SplitModeNative,
		Features: []Feature{
			FeatureSplitPackages,
		},
	}, splitTestSchemas()...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	bridgePath := filepath.Join(target, "internal", "split", "bridge", "bridge.go")
	_, err = os.Stat(bridgePath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	userQueryCode, err := os.ReadFile(filepath.Join(target, "user_query.go"))
	require.NoError(t, err)
	require.NotContains(t, string(userQueryCode), "entbridge.SetNeighbors(")
	require.Contains(t, string(userQueryCode), "sqlgraph.SetNeighbors(")

	clientCode, err := os.ReadFile(filepath.Join(target, "client.go"))
	require.NoError(t, err)
	require.NotContains(t, string(clientCode), "entbridge.Neighbors(")
	require.Contains(t, string(clientCode), "sqlgraph.Neighbors(")
}

func TestGraphSplitNativeEntQLInline(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package:   "entc/gen",
		Target:    target,
		Storage:   drivers[0],
		IDType:    &field.TypeInfo{Type: field.TypeInt},
		SplitMode: SplitModeNative,
		Features: []Feature{
			FeatureSplitPackages,
			FeatureEntQL,
		},
	}, []*load.Schema{
		{
			Name: "User",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
		},
	}...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	rootCode, err := os.ReadFile(filepath.Join(target, "entql.go"))
	require.NoError(t, err)
	require.Contains(t, string(rootCode), "var schemaGraph =")
	require.NotContains(t, string(rootCode), "entqlinternal.")

	internalPath := filepath.Join(target, "internal", "split", "entql", "entql.go")
	_, err = os.Stat(internalPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestSplitImportGraphAcyclic(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package: "entc/gen",
		Target:  target,
		Storage: drivers[0],
		IDType:  &field.TypeInfo{Type: field.TypeInt},
		Features: []Feature{
			FeatureSplitPackages,
			FeatureEntQL,
		},
	}, splitTestSchemas()...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	imports := generatedImportGraph(t, target, graph.Config.Package)
	cycle := findImportCycle(imports)
	require.Emptyf(t, cycle, "import cycle detected: %s", strings.Join(cycle, " -> "))
}

func TestSplitCompatPublicAPIsParity(t *testing.T) {
	t.Parallel()

	legacyTarget := filepath.Join(t.TempDir(), "legacy")
	compatTarget := filepath.Join(t.TempDir(), "compat")

	legacy, err := NewGraph(&Config{
		Package: "entc/gen",
		Target:  legacyTarget,
		Storage: drivers[0],
		IDType:  &field.TypeInfo{Type: field.TypeInt},
		Features: []Feature{
			FeatureEntQL,
		},
	}, splitTestSchemas()...)
	require.NoError(t, err)
	require.NoError(t, legacy.Gen())

	compat, err := NewGraph(&Config{
		Package: "entc/gen",
		Target:  compatTarget,
		Storage: drivers[0],
		IDType:  &field.TypeInfo{Type: field.TypeInt},
		Features: []Feature{
			FeatureSplitPackages,
			FeatureEntQL,
		},
	}, splitTestSchemas()...)
	require.NoError(t, err)
	require.NoError(t, compat.Gen())

	legacySymbols := exportedRootSymbols(t, legacyTarget)
	compatSymbols := exportedRootSymbols(t, compatTarget)
	require.Equal(t, legacySymbols, compatSymbols)
}

func TestSplitEntQLFacadeGuardrails(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package: "entc/gen",
		Target:  target,
		Storage: drivers[0],
		IDType:  &field.TypeInfo{Type: field.TypeInt},
		Features: []Feature{
			FeatureSplitPackages,
			FeatureEntQL,
		},
	}, splitTestSchemas()...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	code, err := os.ReadFile(filepath.Join(target, "entql.go"))
	require.NoError(t, err)
	const maxFacadeLines = 120
	require.LessOrEqual(t, strings.Count(string(code), "\n")+1, maxFacadeLines)
	require.NotContains(t, string(code), "var schemaGraph =")
	require.NotContains(t, string(code), "schemaGraph.EvalP(")
}

func TestSplitNativeMigrationMapGenerated(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package:   "entc/gen",
		Target:    target,
		Storage:   drivers[0],
		IDType:    &field.TypeInfo{Type: field.TypeInt},
		SplitMode: SplitModeNative,
		Features: []Feature{
			FeatureSplitPackages,
			FeatureEntQL,
		},
	}, splitTestSchemas()...)
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	mappingCode, err := os.ReadFile(filepath.Join(target, "internal", "split", "native", "migration_map_v1.go"))
	require.NoError(t, err)
	require.Contains(t, string(mappingCode), `const MigrationMapVersion = "v1"`)
	require.Contains(t, string(mappingCode), `"entc/gen/entql.go":`)
	require.Contains(t, string(mappingCode), `"entc/gen/entql.go",`)
	require.Contains(t, string(mappingCode), `"entc/gen/user_query.go":`)
	require.Contains(t, string(mappingCode), `"entc/gen/internal/split/type/user/query.go"`)
	require.Contains(t, string(mappingCode), `"entc/gen/mutation.go#User":`)
	require.Contains(t, string(mappingCode), `"entc/gen/internal/split/type/user/mutation.go"`)
	require.NotContains(t, string(mappingCode), `"internal/split/bridge/bridge.go"`)
}

func splitTestSchemas() []*load.Schema {
	return []*load.Schema{
		{
			Name: "User",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
			Edges: []*load.Edge{
				{Name: "pets", Type: "Pet"},
			},
		},
		{
			Name: "Pet",
			Fields: []*load.Field{
				{Name: "name", Info: &field.TypeInfo{Type: field.TypeString}},
			},
			Edges: []*load.Edge{
				{Name: "owner", Type: "User", RefName: "pets", Inverse: true, Unique: true},
			},
		},
	}
}

func generatedImportGraph(t *testing.T, target, basePkg string) map[string][]string {
	t.Helper()
	graph := make(map[string][]string)
	require.NoError(t, filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(target, dir)
		if err != nil {
			return err
		}
		pkg := basePkg
		if rel != "." {
			pkg = pathpkg.Join(basePkg, filepath.ToSlash(rel))
		}
		if _, ok := graph[pkg]; !ok {
			graph[pkg] = nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			impPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(impPath, basePkg) || impPath == pkg {
				continue
			}
			graph[pkg] = append(graph[pkg], impPath)
		}
		return nil
	}))
	for pkg := range graph {
		graph[pkg] = dedupe(graph[pkg])
	}
	return graph
}

func findImportCycle(graph map[string][]string) []string {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(graph))
	stack := make([]string, 0, len(graph))
	var dfs func(string) []string
	dfs = func(node string) []string {
		state[node] = visiting
		stack = append(stack, node)
		for _, next := range graph[node] {
			switch state[next] {
			case unvisited:
				if cycle := dfs(next); len(cycle) > 0 {
					return cycle
				}
			case visiting:
				start := 0
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == next {
						start = i
						break
					}
				}
				cycle := append([]string{}, stack[start:]...)
				cycle = append(cycle, next)
				return cycle
			}
		}
		state[node] = visited
		stack = stack[:len(stack)-1]
		return nil
	}
	for node := range graph {
		if state[node] == unvisited {
			if cycle := dfs(node); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

func exportedRootSymbols(t *testing.T, dir string) map[string]struct{} {
	t.Helper()
	symbols := make(map[string]struct{})
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(spec.Name.Name) {
							symbols["type:"+spec.Name.Name] = struct{}{}
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if ast.IsExported(name.Name) {
								prefix := "var:"
								if decl.Tok == token.CONST {
									prefix = "const:"
								}
								symbols[prefix+name.Name] = struct{}{}
							}
						}
					}
				}
			case *ast.FuncDecl:
				if !ast.IsExported(decl.Name.Name) {
					continue
				}
				if decl.Recv == nil {
					symbols["func:"+decl.Name.Name] = struct{}{}
					continue
				}
				recv := receiverName(decl.Recv.List[0].Type)
				// In split-compat mode, filter methods are provided through type aliases
				// to internal implementations and therefore do not have local declarations.
				if strings.HasSuffix(recv, "Filter") {
					continue
				}
				symbols["method:"+recv+"."+decl.Name.Name] = struct{}{}
			}
		}
	}
	return symbols
}

func receiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		if id, ok := expr.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return "unknown"
}

func dedupe(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	return deduped
}
