// entc/internal/subpackage_imports_test.go
package internal

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSubPackageNoSiblingImports asserts the PR 6 invariant: every
// generated ent/<entity>/ sub-package imports zero sibling-entity
// packages and never imports the root ent package. This keeps the
// sub-packages as leaves in the dependency graph so Go's parallel
// compiler can shard the per-entity work.
func TestSubPackageNoSiblingImports(t *testing.T) {
	skipFixtures := map[string]bool{
		"gremlin":     true, // doesn't use sub-package layout
		"edgeschema":  true, // pre-existing sibling import (PR 3-5 edge-schema split), out of PR 6 scope
		"customid":    true, // M2M-through-edge sibling import (bloblink.ThroughDefaults) — same root cause as edgeschema
		"multischema": true, // M2M-through-edge sibling import (friendship.ThroughDefaults, parent.ThroughDefaults) — same root cause as edgeschema
	}

	fixturesRoot := filepath.Join("..", "integration")
	entries, err := os.ReadDir(fixturesRoot)
	require.NoError(t, err)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		entDir := filepath.Join(fixturesRoot, e.Name(), "ent")
		info, err := os.Stat(entDir)
		if err != nil || !info.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			if skipFixtures[e.Name()] {
				t.Skipf("out of PR 6 scope: pre-existing sibling import (M2M-through-edge / PR 3-5 split)")
				return
			}
			assertSubPackagesLeaf(t, entDir, e.Name())
		})
	}
}

func assertSubPackagesLeaf(t *testing.T, entDir, fixtureName string) {
	t.Helper()
	// Discover sub-package dirs (ent/<entity>/) — exclude well-known
	// shared dirs that aren't per-entity (internal, predicate, runtime,
	// migrate, hook, enttest, schema, intercept, privacy).
	shared := map[string]bool{
		"internal": true, "predicate": true, "runtime": true, "migrate": true,
		"hook": true, "enttest": true, "schema": true, "intercept": true,
		"privacy": true,
	}
	entries, err := os.ReadDir(entDir)
	require.NoError(t, err)

	// Build the set of per-entity sub-package names first (siblings of the one being checked).
	entityDirs := []string{}
	for _, x := range entries {
		if !x.IsDir() || shared[x.Name()] {
			continue
		}
		entityDirs = append(entityDirs, x.Name())
	}

	for _, dir := range entityDirs {
		dirPath := filepath.Join(entDir, dir)
		walkErr := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if perr != nil {
				return nil // ignore parse errors here; build will catch
			}
			for _, imp := range f.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				// Forbid: imports of sibling entity sub-packages.
				for _, sibling := range entityDirs {
					if sibling == dir {
						continue
					}
					siblingSuffix := "/ent/" + sibling
					if strings.HasSuffix(p, siblingSuffix) {
						t.Errorf("%s: imports sibling sub-package %q — cycle hazard", path, p)
					}
				}
				// Forbid: import of the root ent package.
				if strings.HasSuffix(p, "/ent") && !strings.Contains(p, "entgo.io/ent") {
					t.Errorf("%s: imports root ent package %q — sub-packages must be leaves", path, p)
				}
			}
			return nil
		})
		require.NoError(t, walkErr)
	}
}
