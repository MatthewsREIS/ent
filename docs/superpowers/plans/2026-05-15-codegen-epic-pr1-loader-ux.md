# Codegen Epic PR 1 — Loader UX / Bootstrap Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** End the "comment-out-hook, regenerate, uncomment" workflow paper-cut. When a hook references a generated mutation method that doesn't exist yet, codegen should succeed (via a new bootstrap mode that AST-strips `Hooks` / `Policy` / `Interceptors` method bodies in a temp copy of the schema package), so the generated method gets created and the user's real hooks compile on the next build.

**Architecture:** Two additive components:
1. Extend `entc/internal/snapshot.go:IsBuildError` to recognize type-checker errors (`undefined:`, `has no field or method`, `undeclared name`, `cannot use`), so the existing snapshot-recovery path triggers in more cases.
2. New `entc/internal/bootstrap.go` package: copies the schema directory to a tmpdir, AST-strips method bodies of `Hooks()` / `Policy()` / `Interceptors()` on every Schema-implementing struct (replacing each body with `return nil`), removes any imports made unused by the stripping, then returns the tmpdir path. The loader runs against this stripped copy.

Bootstrap is triggered two ways: explicit opt-in via the new `entc.SkipHookCompilation()` option (and `--skip-hook-compilation` CLI flag), or auto-fallback from `mayRecover` when snapshot restore fails.

**Tech Stack:** Go stdlib (`go/parser`, `go/ast`, `go/format`, `go/token`), `golang.org/x/tools/go/ast/astutil` (already a transitive dep used in `entc/load/load.go`), `github.com/stretchr/testify/require` (already a dep).

**Spec reference:** `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` §4 PR 1.

**Branch:** stay on `worktree-wiggly-singing-pancake` (no separate git-spice branch yet, per epic policy — local commits only, end-of-epic stack submission later).

---

## File map

| File | Status | Responsibility |
|---|---|---|
| `entc/internal/snapshot.go` | modify | Extend `IsBuildError` substring list |
| `entc/internal/snapshot_buildtag_test.go` | modify | Add `TestIsBuildError_TypeCheckerErrors` |
| `entc/internal/bootstrap.go` | **NEW** | AST stripping + tmpdir staging |
| `entc/internal/bootstrap_test.go` | **NEW** | Unit tests for the stripper |
| `entc/internal/regression_bootstrap_test.go` | **NEW** | End-to-end: real schema with broken hook, codegen via bootstrap succeeds |
| `entc/entc.go` | modify | Add `SkipHookCompilation()` option + `gen.Config.SkipHookCompilation` field; integrate bootstrap in `generate()` and `mayRecover()` |
| `entc/gen/config.go` | modify | Add `SkipHookCompilation bool` field on `Config` |
| `cmd/internal/base/base.go` | modify | Add `--skip-hook-compilation` flag |
| `doc/md/devx-bootstrap.md` | **NEW** | When to use bootstrap mode, limitations |

---

## Pre-flight: every implementer reads this first

**Branch discipline.** Stay on `worktree-wiggly-singing-pancake`. Never run `git push`, `gh pr create`, `gs branch submit`, `gs stack submit`, or `git checkout <other>`. Before EVERY commit, run `git rev-parse --abbrev-ref HEAD` and verify the output is `worktree-wiggly-singing-pancake`. **A previous implementer accidentally committed to master — do not repeat that.**

**Working directory.** Every command runs from `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake`. Confirm with `pwd` before doing anything else.

**Parent commit.** This plan's Task 1 builds on PR 0's final commit (`f3471df35 docs(bench): README for in-repo and consumer-scale measurement`). Verify with `git log -1 --pretty=format:'%H %s'` before starting.

---

## Task 1: Extend `IsBuildError` to recognize type-checker errors

The current `IsBuildError` (entc/internal/snapshot.go:218-234) matches `syntax error`, `previous declaration`, `invalid character`, `could not import`, `found '<<'`. It does NOT match the most common error class that hits users: "your hook references a method that doesn't exist on the generated mutation type". Extend the substring list so the recovery path triggers in those cases.

**Files:**
- Modify: `entc/internal/snapshot.go:218-234`
- Modify: `entc/internal/snapshot_buildtag_test.go` (append new test function)

- [ ] **Step 1: Write the failing test**

Append the following to `entc/internal/snapshot_buildtag_test.go` (after the existing `TestSnapshotRestoreIsTextBased` function):

```go
// TestIsBuildError_TypeCheckerErrors verifies that the type-checker error
// classes hit users most often are matched as build errors, so the existing
// mayRecover/snapshot-restore path triggers on them.
func TestIsBuildError_TypeCheckerErrors(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"undefined symbol", "main.go:10:5: undefined: ent.UserMutation", true},
		{"missing field or method", "main.go:11:7: m.SetNewField undefined (type *ent.UserMutation has no field or method SetNewField)", true},
		{"undeclared name (older Go)", "main.go:12:3: undeclared name: Helper", true},
		{"cannot use", "main.go:13:9: cannot use \"foo\" (untyped string constant) as int value in argument to m.SetAge", true},
		{"existing syntax error still matched", "main.go:1:1: syntax error", true},
		{"existing could not import still matched", "main.go:1:1: could not import x/y (foo)", true},
		{"unrelated runtime error not matched", "connection refused", false},
		{"empty error string not matched", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsBuildError(errors.New(tc.msg))
			require.Equal(t, tc.want, got, "IsBuildError(%q) = %v, want %v", tc.msg, got, tc.want)
		})
	}
}
```

Then add `"errors"` to the imports of `snapshot_buildtag_test.go` if not already present.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 -run TestIsBuildError_TypeCheckerErrors ./entc/internal/...`

Expected: 4 subtests fail (the 4 new substring patterns). The "existing" cases pass. The "unrelated" cases pass (correctly returning false).

- [ ] **Step 3: Extend `IsBuildError` in `entc/internal/snapshot.go`**

Find the existing function at `entc/internal/snapshot.go:218-234`:

```go
// IsBuildError reports if the given error is an error from the Go command (e.g. syntax error).
func IsBuildError(err error) bool {
	if strings.HasPrefix(err.Error(), "entc/load: #") {
		return true
	}
	for _, s := range []string{
		"syntax error",
		"previous declaration",
		"invalid character",
		"could not import",
		"found '<<'",
	} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}
```

Replace the substring slice to add four new entries:

```go
// IsBuildError reports if the given error is an error from the Go command (e.g. syntax error).
func IsBuildError(err error) bool {
	if strings.HasPrefix(err.Error(), "entc/load: #") {
		return true
	}
	for _, s := range []string{
		// Original substrings (syntax-level errors).
		"syntax error",
		"previous declaration",
		"invalid character",
		"could not import",
		"found '<<'",
		// Type-checker errors emitted when schema-side code references
		// symbols that don't yet exist in the generated ent package.
		// These hit users most often during normal schema iteration:
		// add a new field, then a hook that calls SetNewField, then
		// regenerate -- without this match, mayRecover never triggers.
		"undefined:",
		"has no field or method",
		"undeclared name",
		"cannot use",
	} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -run TestIsBuildError_TypeCheckerErrors ./entc/internal/...`

Expected: all 8 subtests PASS.

- [ ] **Step 5: Run the full entc/internal test suite to verify no regression**

Run: `go test -count=1 ./entc/internal/...`

Expected: every test passes; no failures.

- [ ] **Step 6: Commit**

**Re-verify branch first.** Run: `git rev-parse --abbrev-ref HEAD` — must be `worktree-wiggly-singing-pancake`.

```bash
git add entc/internal/snapshot.go entc/internal/snapshot_buildtag_test.go
git commit -m "feat(entc/internal): IsBuildError matches type-checker errors

Adds four substring matches (undefined:, has no field or method,
undeclared name, cannot use) so the existing mayRecover/snapshot-
restore path triggers when a schema hook references a generated
mutation method that does not yet exist. This is the most common
class of error that breaks codegen during normal schema iteration.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 2: AST-strip helper

The bootstrap mode needs a pure function that takes Go source bytes and returns Go source bytes with the bodies of `Hooks()`, `Policy()`, and `Interceptors()` methods replaced with `return nil`. Unused imports must be removed (Go fails to compile files with unused imports).

This task is the pure stripper. Task 3 wraps it in a "process a whole directory" helper.

**Files:**
- Create: `entc/internal/bootstrap.go`
- Create: `entc/internal/bootstrap_test.go`

- [ ] **Step 1: Write the failing test**

Create `entc/internal/bootstrap_test.go` with the following content:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package internal

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripHookBodies_BasicHooks(t *testing.T) {
	src := `package schema

import (
	"context"

	"entgo.io/ent"
	"example.com/x/ent/gen"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return nil
}

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if um, ok := m.(*gen.UserMutation); ok {
					um.SetName("processed")
				}
				return next.Mutate(ctx, m)
			})
		},
	}
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (User) Hooks() []ent.Hook {")
	require.Contains(t, s, "return nil\n}", "Hooks body must be replaced with `return nil`")
	require.NotContains(t, s, "SetName", "the original hook body must be gone")
	require.NotContains(t, s, "MutateFunc", "the original hook body must be gone")
	require.NotContains(t, s, `"context"`, "context import must be dropped (now unused)")
	require.NotContains(t, s, `"example.com/x/ent/gen"`, "generated-pkg import must be dropped (now unused)")
	require.Contains(t, s, `"entgo.io/ent"`, "ent import must stay (still used in method signature)")
	require.Contains(t, s, "func (User) Fields() []ent.Field {", "Fields method must be preserved")
}

func TestStripHookBodies_PolicyAndInterceptors(t *testing.T) {
	src := `package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/privacy"
	"example.com/x/ent/gen/rule"
)

type Tenant struct {
	ent.Schema
}

func (Tenant) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			rule.DenyIfNoTenant(),
		},
	}
}

func (Tenant) Interceptors() []ent.Interceptor {
	return []ent.Interceptor{
		rule.FilterTenant(),
	}
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (Tenant) Policy() ent.Policy {")
	require.Contains(t, s, "func (Tenant) Interceptors() []ent.Interceptor {")
	occurrences := strings.Count(s, "return nil\n}")
	require.GreaterOrEqual(t, occurrences, 2, "both Policy and Interceptors bodies must be replaced")
	require.NotContains(t, s, "DenyIfNoTenant", "Policy body must be gone")
	require.NotContains(t, s, "FilterTenant", "Interceptors body must be gone")
	require.NotContains(t, s, `"entgo.io/ent/privacy"`, "privacy import must be dropped (now unused)")
	require.NotContains(t, s, `"example.com/x/ent/gen/rule"`, "rule import must be dropped (now unused)")
	require.Contains(t, s, `"entgo.io/ent"`, "ent import must stay")
}

func TestStripHookBodies_PreservesUnrelatedFunctions(t *testing.T) {
	src := `package schema

import "entgo.io/ent"

type Comment struct {
	ent.Schema
}

func (Comment) Fields() []ent.Field {
	return nil
}

func (Comment) Edges() []ent.Edge {
	return nil
}

func someHelper() int {
	return 42
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func someHelper() int {", "top-level helpers must be preserved (only method receivers are stripped)")
	require.Contains(t, s, "return 42", "helper body must be preserved")
	require.Contains(t, s, "func (Comment) Fields() []ent.Field {", "Fields preserved")
	require.Contains(t, s, "func (Comment) Edges() []ent.Edge {", "Edges preserved")
}

func TestStripHookBodies_NoHooksMethod(t *testing.T) {
	src := `package schema

import "entgo.io/ent"

type Group struct {
	ent.Schema
}

func (Group) Fields() []ent.Field {
	return nil
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "func (Group) Fields() []ent.Field {")
	require.NotContains(t, s, "Hooks", "no Hooks method in source -> no Hooks method in output")
}

func TestStripHookBodies_TopLevelFuncCalledHooksNotStripped(t *testing.T) {
	// A top-level (non-method) function named Hooks must NOT be stripped --
	// the stripper only touches methods (FuncDecl.Recv != nil).
	src := `package schema

import "entgo.io/ent"

func Hooks() []ent.Hook {
	return []ent.Hook{nil}
}
`
	out, err := StripHookBodies([]byte(src))
	require.NoError(t, err)
	s := string(out)
	require.Contains(t, s, "[]ent.Hook{nil}", "top-level Hooks function must be preserved")
}

func TestStripHookBodies_SyntaxErrorReturnsError(t *testing.T) {
	src := `package schema

func ((((`
	_, err := StripHookBodies([]byte(src))
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails (no `StripHookBodies` yet)**

Run: `go test -count=1 -run TestStripHookBodies ./entc/internal/...`

Expected: build failure — `undefined: StripHookBodies`.

- [ ] **Step 3: Implement `StripHookBodies`**

Create `entc/internal/bootstrap.go` with the following content:

```go
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
		fn.Body = &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent("nil")},
				},
			},
		}
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -run TestStripHookBodies ./entc/internal/...`

Expected: all 6 subtests PASS.

- [ ] **Step 5: Commit**

**Re-verify branch.**

```bash
git add entc/internal/bootstrap.go entc/internal/bootstrap_test.go
git commit -m "feat(entc/internal): StripHookBodies AST stripper for bootstrap mode

Pure function that takes Go source bytes and returns the same source
with Hooks() / Policy() / Interceptors() method bodies replaced with
'return nil'. Unused imports (those only referenced from the stripped
bodies) are removed. Top-level functions with matching names are NOT
touched -- only methods with receivers.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 3: Bootstrap directory staging

`StripHookBodies` handles one file at a time. Now we need the orchestrator that takes a schema directory, copies it to a tmpdir, strips every `.go` file, and returns the tmpdir path.

**Files:**
- Modify: `entc/internal/bootstrap.go` (add `StageStrippedSchema` function)
- Modify: `entc/internal/bootstrap_test.go` (add tests)

- [ ] **Step 1: Write the failing test**

Append to `entc/internal/bootstrap_test.go`:

```go
func TestStageStrippedSchema_CopiesAndStrips(t *testing.T) {
	src := t.TempDir()

	// Write a schema file with a Hooks method that references a "generated" symbol.
	userSrc := `package schema

import (
	"context"

	"entgo.io/ent"
	"badpackage" // intentionally bad path -- proves the stripped output doesn't need it
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field { return nil }

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				_ = badpackage.GoSomething(m)
				return next.Mutate(ctx, m)
			})
		},
	}
}
`
	require.NoError(t, os.WriteFile(filepath.Join(src, "user.go"), []byte(userSrc), 0o644))

	// Also write a non-.go file that should be copied verbatim.
	require.NoError(t, os.WriteFile(filepath.Join(src, "README"), []byte("hello"), 0o644))

	// And a subdirectory with another .go file.
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	subSrc := `package schema

import "entgo.io/ent"

type Comment struct{ ent.Schema }

func (Comment) Fields() []ent.Field { return nil }
`
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "comment.go"), []byte(subSrc), 0o644))

	dst, err := StageStrippedSchema(src)
	require.NoError(t, err)
	defer os.RemoveAll(dst)

	// user.go should exist with Hooks body stripped and bad imports removed.
	userOut, err := os.ReadFile(filepath.Join(dst, "user.go"))
	require.NoError(t, err)
	require.NotContains(t, string(userOut), "badpackage", "bad import must be gone from stripped file")
	require.NotContains(t, string(userOut), "GoSomething", "stripped body content must be gone")
	require.Contains(t, string(userOut), "func (User) Hooks() []ent.Hook {")

	// README should be copied verbatim.
	readme, err := os.ReadFile(filepath.Join(dst, "README"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(readme))

	// sub/comment.go should exist (no hooks to strip; should be unchanged in semantics).
	commentOut, err := os.ReadFile(filepath.Join(dst, "sub", "comment.go"))
	require.NoError(t, err)
	require.Contains(t, string(commentOut), "type Comment struct")

	// Source directory must be untouched.
	srcUser, err := os.ReadFile(filepath.Join(src, "user.go"))
	require.NoError(t, err)
	require.Contains(t, string(srcUser), "GoSomething", "original source must NOT be modified")
}

func TestStageStrippedSchema_NonExistentSrc(t *testing.T) {
	_, err := StageStrippedSchema("/this/path/does/not/exist")
	require.Error(t, err)
}
```

Add to the existing imports of `bootstrap_test.go`:
```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 -run TestStageStrippedSchema ./entc/internal/...`

Expected: build failure — `undefined: StageStrippedSchema`.

- [ ] **Step 3: Implement `StageStrippedSchema`**

Append the following to `entc/internal/bootstrap.go`:

```go
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
	dst, err := os.MkdirTemp("", "ent-bootstrap-*")
	if err != nil {
		return "", fmt.Errorf("bootstrap: tempdir: %w", err)
	}
	if err := stripAndCopyTree(srcDir, dst); err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	return dst, nil
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
```

Add to the imports of `bootstrap.go`:
```go
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -run TestStageStrippedSchema ./entc/internal/...`

Expected: both tests PASS.

- [ ] **Step 5: Run the full entc/internal suite to confirm no regressions**

Run: `go test -count=1 ./entc/internal/...`

Expected: every test passes.

- [ ] **Step 6: Commit**

**Re-verify branch.**

```bash
git add entc/internal/bootstrap.go entc/internal/bootstrap_test.go
git commit -m "feat(entc/internal): StageStrippedSchema copies + strips a schema dir

Walks srcDir, copies every file into a fresh tmpdir, and AST-strips
Hooks/Policy/Interceptors method bodies in .go files. Non-.go files
copied verbatim. Source dir untouched. Caller owns the returned dir.

This is the entrypoint bootstrap mode uses before invoking the loader.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 4: Add `gen.Config.SkipHookCompilation` field

Before wiring bootstrap into `entc.Generate`, the config needs a field to control it.

**Files:**
- Modify: `entc/gen/config.go`

- [ ] **Step 1: Inspect the existing Config struct**

Run: `grep -n '^type Config struct\|^}' entc/gen/config.go | head -10`

Note the existing field layout. You'll add `SkipHookCompilation` next to the other boolean config fields (e.g., near `Storage`, `Features`, etc.).

- [ ] **Step 2: Add the field**

Read `entc/gen/config.go` and locate the `Config struct`. Add the following field, grouped with other configuration toggles (likely near the bottom of the struct):

```go
	// SkipHookCompilation, when true, instructs the loader to AST-strip the
	// bodies of Hooks() / Policy() / Interceptors() methods on every
	// Schema-implementing type before type-checking the schema package.
	// This lets codegen complete when a hook references a generated symbol
	// that does not yet exist. See entc.SkipHookCompilation() for the
	// option setter, and doc/md/devx-bootstrap.md for usage.
	SkipHookCompilation bool
```

Edit the file by reading it first to find the right insertion point. Pick a location adjacent to other bool fields (e.g., right after a related field). If unsure, append it as the last field of the struct.

- [ ] **Step 3: Verify the package builds**

Run: `go build ./entc/gen/...`

Expected: clean build.

- [ ] **Step 4: Commit**

**Re-verify branch.**

```bash
git add entc/gen/config.go
git commit -m "feat(entc/gen): add Config.SkipHookCompilation field

Controls the bootstrap mode introduced by codegen-epic PR 1: when true,
the loader AST-strips Hooks/Policy/Interceptors method bodies before
type-checking the schema package. Default false preserves current
behavior.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 5: Add `entc.SkipHookCompilation()` option and wire bootstrap into `generate()`

Add the option setter and integrate bootstrap into the `generate` orchestrator. When `cfg.SkipHookCompilation` is true, the schema path is rewritten to a staged-stripped tmpdir before `LoadGraph` is called.

**Files:**
- Modify: `entc/entc.go`

- [ ] **Step 1: Read the existing option setters and `generate()` flow**

Look at how other options are defined in `entc/entc.go` (search for `func .*Option`). The `SnapshotDir` option around line 168 is a representative pattern:

Run: `grep -n 'func.*Option\|cfg.SnapshotDir\|cfg.SkipHookCompilation' entc/entc.go | head -10`

Note the surrounding pattern; add `SkipHookCompilation` next to it.

- [ ] **Step 2: Add the option setter**

In `entc/entc.go`, add this function near the other option setters (just before or after `SnapshotDir` is a good spot — they're related: SnapshotDir is about recovery, SkipHookCompilation is about bootstrapping past a hook-vs-codegen deadlock):

```go
// SkipHookCompilation enables bootstrap mode: before loading the schema
// package, entc copies the schema directory to a tmpdir and AST-strips the
// bodies of Hooks() / Policy() / Interceptors() methods on every
// Schema-implementing type. The loader runs against the stripped copy, so
// codegen succeeds even when hooks reference generated symbols that do not
// yet exist.
//
// Use this when you have added a new field/edge to a schema and an existing
// hook references the not-yet-generated method (e.g., m.SetNewField(v)). The
// user's real schema package is never modified.
//
// Known limitation: this only strips methods in the schema package itself.
// If a hook lives in a subpackage that also imports the generated ent
// package, the subpackage will still fail to type-check.
func SkipHookCompilation() Option {
	return func(cfg *gen.Config) error {
		cfg.SkipHookCompilation = true
		return nil
	}
}
```

- [ ] **Step 3: Wire bootstrap into the `generate()` function**

Find the `generate` function at entc/entc.go:404. The current body starts:

```go
func generate(schemaPath string, cfg *gen.Config) error {
	graph, err := LoadGraph(schemaPath, cfg)
	if err != nil {
		if err := mayRecover(err, schemaPath, cfg); err != nil {
			return err
		}
		if graph, err = LoadGraph(schemaPath, cfg); err != nil {
			return err
		}
	}
	...
}
```

Replace the first few lines (before the first `LoadGraph` call) with bootstrap handling. The full new `generate` function should look like this — replace the existing function entirely:

```go
// generate loads the given schema and run codegen.
func generate(schemaPath string, cfg *gen.Config) error {
	loadPath, cleanup, err := maybeStageBootstrap(schemaPath, cfg, false)
	if err != nil {
		return err
	}
	defer cleanup()

	graph, err := LoadGraph(loadPath, cfg)
	if err != nil {
		if rErr := mayRecover(err, schemaPath, cfg); rErr != nil {
			return rErr
		}
		// mayRecover may have either restored from snapshot OR succeeded via
		// bootstrap fallback. In the snapshot path, we still need to re-run
		// LoadGraph against the (now-presumed-fixed) schema. In the bootstrap
		// fallback path, mayRecover already invoked Generate, so we should
		// not re-LoadGraph.
		if cfg.recovered {
			return nil
		}
		loadPath, cleanup, err = maybeStageBootstrap(schemaPath, cfg, false)
		if err != nil {
			return err
		}
		defer cleanup()
		if graph, err = LoadGraph(loadPath, cfg); err != nil {
			return err
		}
	}
	if err := normalizePkg(cfg); err != nil {
		return err
	}
	// Save the original target before Gen (Split rewrites cfg.Target to a temp dir).
	origTarget := cfg.Target
	if err := graph.Gen(); err != nil {
		return err
	}
	// Copy snapshot to external directory if configured.
	if cfg.SnapshotDir != "" {
		if ok, _ := cfg.FeatureEnabled(gen.FeatureSnapshot.Name); ok {
			src := filepath.Join(origTarget, "internal", "schema.go")
			dst := filepath.Join(cfg.SnapshotDir, "schema.go")
			data, err := os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("reading snapshot for copy: %w", err)
			}
			if err := os.MkdirAll(cfg.SnapshotDir, 0o755); err != nil {
				return fmt.Errorf("creating snapshot dir: %w", err)
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return fmt.Errorf("writing snapshot copy: %w", err)
			}
		}
	}
	return nil
}

// maybeStageBootstrap returns the schema path to feed the loader. If
// cfg.SkipHookCompilation is true OR forceBootstrap is true, the schema dir
// is AST-stripped into a tmpdir and that tmpdir's path is returned. Otherwise
// the original schemaPath is returned unchanged. The returned cleanup func
// is always non-nil and safe to defer.
func maybeStageBootstrap(schemaPath string, cfg *gen.Config, forceBootstrap bool) (string, func(), error) {
	if !cfg.SkipHookCompilation && !forceBootstrap {
		return schemaPath, func() {}, nil
	}
	staged, err := internal.StageStrippedSchema(schemaPath)
	if err != nil {
		return "", func() {}, fmt.Errorf("bootstrap: %w", err)
	}
	return staged, func() { _ = os.RemoveAll(staged) }, nil
}
```

Note: the new code references `cfg.recovered`. We need to add this field to `gen.Config` too. To keep this task focused, add it now as a small extra step.

- [ ] **Step 4: Add the `recovered` flag to `gen.Config`**

In `entc/gen/config.go`, add this field near `SkipHookCompilation`:

```go
	// recovered is set by entc.mayRecover when it has already produced a
	// completed graph via the bootstrap fallback path, so the top-level
	// generate() should not attempt to re-load and re-generate.
	recovered bool
```

The field is unexported because it's an internal coordination flag, not a user-facing option.

- [ ] **Step 5: Build to confirm it compiles**

Run: `go build ./entc/... ./cmd/...`

Expected: clean build.

- [ ] **Step 6: Commit**

**Re-verify branch.**

```bash
git add entc/entc.go entc/gen/config.go
git commit -m "feat(entc): SkipHookCompilation option wires bootstrap into generate()

When the option is set, the schema directory is AST-stripped into a
tmpdir before LoadGraph is invoked. The user's source is never touched.
maybeStageBootstrap is the integration helper used by both the explicit
option path and (next commit) the mayRecover auto-fallback path.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 6: Wire bootstrap auto-fallback into `mayRecover`

Now make `mayRecover` (entc/entc.go:443) also try bootstrap as a second-chance recovery after snapshot restore. This is what makes the loader UX work without the user having to set any flag — common cases just work.

**Files:**
- Modify: `entc/entc.go`

- [ ] **Step 1: Read the existing `mayRecover`**

The current implementation (lines 443-469) returns the snapshot's `.Restore()` error. We want to add: if snapshot recovery fails AND the error class matches, try bootstrap.

- [ ] **Step 2: Replace `mayRecover` with the auto-fallback version**

Replace the entire `mayRecover` function in `entc/entc.go` with:

```go
func mayRecover(err error, schemaPath string, cfg *gen.Config) error {
	if !errors.As(err, &packages.Error{}) && !internal.IsBuildError(err) {
		return err
	}
	// If the build error comes from the schema package.
	if cErr := internal.CheckDir(schemaPath); cErr != nil {
		return fmt.Errorf("schema failure: %w", cErr)
	}

	snapshotOK, _ := cfg.FeatureEnabled(gen.FeatureSnapshot.Name)
	if snapshotOK {
		if ok, _ := cfg.FeatureEnabled(gen.FeatureGlobalID.Name); ok {
			if internal.CheckDir(filepath.Dir(gen.IncrementStartsFilePath(cfg.Target))) != nil {
				if err := gen.ResolveIncrementStartsConflict(cfg.Target); err != nil {
					return err
				}
			}
		}
		var target string
		if cfg.SnapshotDir != "" {
			target = filepath.Join(cfg.SnapshotDir, "schema.go")
		} else {
			target = filepath.Join(cfg.Target, "internal/schema.go")
		}
		if rErr := (&internal.Snapshot{Path: target, Config: cfg}).Restore(); rErr == nil {
			return nil
		}
		// Snapshot restore failed (file missing, malformed, or schema stale).
		// Fall through to bootstrap.
	}

	// Bootstrap fallback: AST-strip the current schema and try generating
	// against the stripped copy.
	loadPath, cleanup, sErr := maybeStageBootstrap(schemaPath, cfg, true)
	if sErr != nil {
		return fmt.Errorf("%w; bootstrap fallback also failed: %v", err, sErr)
	}
	defer cleanup()
	graph, lErr := LoadGraph(loadPath, cfg)
	if lErr != nil {
		return fmt.Errorf("%w; bootstrap fallback also failed: %v", err, lErr)
	}
	if nErr := normalizePkg(cfg); nErr != nil {
		return fmt.Errorf("%w; bootstrap fallback also failed: %v", err, nErr)
	}
	if gErr := graph.Gen(); gErr != nil {
		return fmt.Errorf("%w; bootstrap fallback also failed: %v", err, gErr)
	}
	cfg.recovered = true
	return nil
}
```

Notes on the change:
- The original `mayRecover` returned `err` if FeatureSnapshot was disabled. The new version drops that gate so bootstrap can run independently of FeatureSnapshot. That's an intentional expansion of recovery surface — bootstrap is useful whether or not the team uses snapshot.
- Snapshot recovery and bootstrap fallback are ordered: snapshot first (it's cheaper and handles the merge-conflict case), bootstrap second (catches the schema-references-not-yet-generated-code case).
- The bootstrap fallback path runs the FULL codegen pipeline (LoadGraph → normalizePkg → Gen), then sets `cfg.recovered = true` so the outer `generate()` knows not to re-do work.

- [ ] **Step 3: Build to confirm it compiles**

Run: `go build ./entc/...`

Expected: clean build.

- [ ] **Step 4: Run the existing snapshot tests to confirm no regression**

Run: `go test -count=1 ./entc/internal/... ./entc/...`

Expected: every test passes. Snapshot recovery tests (`TestSnapshot_Restore`, `TestSnapshotIsBuildTagged`, `TestSnapshotRestoreIsTextBased`) all still pass — bootstrap is additive, not replacing them.

- [ ] **Step 5: Commit**

**Re-verify branch.**

```bash
git add entc/entc.go
git commit -m "feat(entc): mayRecover falls back to bootstrap after snapshot restore

Adds an auto-fallback path: when LoadGraph fails with a build-error
class error (now broader thanks to IsBuildError extension), try the
existing snapshot restore first, then fall back to bootstrap mode
(AST-strip + retry against the current schema). If both fail, the
original error is surfaced with a note that bootstrap also failed.

Bootstrap is now triggered automatically -- no flag required -- for
the most common DX-breaking case (added a field, hook references the
new method, snapshot is stale).

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 7: End-to-end regression test for bootstrap with broken Hooks

Now the integration test: create a schema with a `Hooks()` method that references a not-yet-generated mutation method, run `entc.Generate` with bootstrap mode, and verify codegen succeeds.

**Files:**
- Create: `entc/internal/regression_bootstrap_test.go`

The test needs to live in `entc/internal/` rather than `entc/` because it exercises the bootstrap helpers without going through the full `entc.Generate` (which has a tighter package dependency chain). Alternative placement is `entc/bootstrap_test.go` — see Step 1 decision.

- [ ] **Step 1: Decide placement and write the test**

Place the test in `entc/bootstrap_test.go` (NEW file, package `entc`). This exercises the full `entc.Generate` path with `SkipHookCompilation` set.

Create `entc/bootstrap_test.go` with this content:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	"github.com/stretchr/testify/require"
)

// TestBootstrap_SkipHooksThatReferenceGeneratedMutation is the canonical
// regression test for codegen-epic PR 1: a schema declares a Hooks() method
// that references a not-yet-generated mutation method. Without bootstrap
// mode, the loader fails to type-check the schema package and codegen
// aborts. With SkipHookCompilation, the loader runs against an AST-stripped
// copy of the schema and codegen succeeds.
func TestBootstrap_SkipHooksThatReferenceGeneratedMutation(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	mod := writeBootstrapModule(t, "bootstraphooks")
	schemaPath := filepath.Join(mod, "schema")

	// Write a schema with a Hooks() body that calls a method
	// (`SetMagicValue`) that the generator wouldn't normally emit -- and
	// definitely doesn't exist before this codegen run.
	require.NoError(t, os.MkdirAll(schemaPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaPath, "user.go"), []byte(`package schema

import (
	"context"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"bootstraphooks/ent"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if um, ok := m.(*ent.UserMutation); ok {
					// SetMagicValue is not a real generated method; this
					// reference will fail to type-check unless bootstrap
					// strips the Hooks body before the loader runs.
					um.SetMagicValue("x")
				}
				return next.Mutate(ctx, m)
			})
		},
	}
}
`), 0o644))

	target := filepath.Join(mod, "ent")
	err := entc.Generate(schemaPath, &gen.Config{Target: target}, entc.SkipHookCompilation())
	require.NoError(t, err, "codegen with SkipHookCompilation should succeed even though the schema hook references a not-yet-generated method")

	// Verify the generated User entity exists (proves codegen actually ran,
	// not just that the loader silently noop'd).
	userGo := filepath.Join(target, "user.go")
	_, err = os.Stat(userGo)
	require.NoError(t, err, "generated user.go should exist after bootstrap codegen")
}

// writeBootstrapModule creates a temp Go module with go.mod pointing back at
// the local ent repo via a replace directive. Returns the module root.
func writeBootstrapModule(t *testing.T, module string) string {
	t.Helper()
	mod := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Clean(filepath.Join(wd, ".."))
	goMod := fmt.Sprintf(`module %s

go 1.23

require entgo.io/ent v0.0.0

replace entgo.io/ent => %s
`, module, repoRoot)
	require.NoError(t, os.WriteFile(filepath.Join(mod, "go.mod"), []byte(goMod), 0o644))
	return mod
}
```

Note: the test uses `package entc_test` (the external-test convention) so it can `import "entgo.io/ent/entc"` cleanly.

- [ ] **Step 2: Run the test to verify it passes**

Run: `go test -count=1 -timeout 120s -run TestBootstrap_SkipHooksThatReferenceGeneratedMutation ./entc/...`

Expected: PASS. The first run may be slow (cold caches); subsequent runs are faster.

If it FAILS with "package not found" or "missing go.sum entry", run `go mod download` first in the test's tmpdir. If the test framework can't manage this, the test setup may need a `go mod tidy` step similar to what `internal/bench/bench.go` does for its temp modules — refer to that for the pattern.

- [ ] **Step 3: Commit**

**Re-verify branch.**

```bash
git add entc/bootstrap_test.go
git commit -m "test(entc): bootstrap mode handles Hooks referencing missing methods

End-to-end regression: a schema declares Hooks() with a body that
references *ent.UserMutation.SetMagicValue, which does not exist in
the generated code. Without bootstrap, the loader fails to type-check
the schema package. With SkipHookCompilation, codegen succeeds.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 8: End-to-end regression test for bootstrap with broken Policy

Same shape as Task 7 but for `Policy()`. Confirms the stripper handles Policy methods too.

**Files:**
- Modify: `entc/bootstrap_test.go`

- [ ] **Step 1: Append the second test**

Append to `entc/bootstrap_test.go`:

```go
// TestBootstrap_SkipPolicyThatReferencesGeneratedSymbol mirrors the Hooks
// test for Policy(). Same chicken-and-egg pattern, different method name.
func TestBootstrap_SkipPolicyThatReferencesGeneratedSymbol(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	mod := writeBootstrapModule(t, "bootstrappolicy")
	schemaPath := filepath.Join(mod, "schema")

	require.NoError(t, os.MkdirAll(schemaPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemaPath, "tenant.go"), []byte(`package schema

import (
	"context"
	"errors"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"

	"bootstrappolicy/ent"
)

type Tenant struct {
	ent.Schema
}

func (Tenant) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}

func (Tenant) Policy() ent.Policy {
	return ent.PolicyFunc(func(ctx context.Context, m ent.Mutation) error {
		if tm, ok := m.(*ent.TenantMutation); ok {
			// PendingDeletion is not a real generated method.
			if tm.PendingDeletion() {
				return errors.New("cannot mutate a tenant pending deletion")
			}
		}
		return nil
	})
}
`), 0o644))

	target := filepath.Join(mod, "ent")
	err := entc.Generate(schemaPath, &gen.Config{Target: target}, entc.SkipHookCompilation())
	require.NoError(t, err, "codegen with SkipHookCompilation should succeed when Policy references a not-yet-generated method")

	_, err = os.Stat(filepath.Join(target, "tenant.go"))
	require.NoError(t, err, "generated tenant.go should exist after bootstrap codegen")
}
```

(The `ent.PolicyFunc` may need to be the actual function/type used in this fork — check `grep -rn 'type Policy\b\|PolicyFunc' .` from the repo root if `PolicyFunc` doesn't resolve. The test's job is to exercise the AST stripper for Policy; the precise body shape can be adjusted to compile against the real Policy interface, as long as the body references `*ent.TenantMutation.PendingDeletion()` to trigger the chicken-and-egg.)

- [ ] **Step 2: Run and verify**

Run: `go test -count=1 -timeout 120s -run TestBootstrap_SkipPolicy ./entc/...`

Expected: PASS.

If it fails because `ent.PolicyFunc` doesn't exist, replace the body with a simpler form that still references `*ent.TenantMutation.PendingDeletion()` — what matters for the test is the typed mutation reference, not the Policy shape itself.

- [ ] **Step 3: Commit**

**Re-verify branch.**

```bash
git add entc/bootstrap_test.go
git commit -m "test(entc): bootstrap mode handles Policy referencing missing methods

Mirrors the Hooks test for Policy(). Codegen succeeds when SkipHookCompilation
is set even though the schema's Policy body references a not-yet-generated
mutation method.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 9: CLI flag `--skip-hook-compilation`

Add the CLI surface so `ent generate --skip-hook-compilation ./ent/schema` works.

**Files:**
- Modify: `cmd/internal/base/base.go`

- [ ] **Step 1: Read the existing `GenerateCmd` flag block**

In `cmd/internal/base/base.go`, lines 144-210 define `GenerateCmd`. The flag block runs from line 200-208. The new flag goes there.

- [ ] **Step 2: Add the flag and option**

Find this section (around lines 145-210). Three changes:

1. In the `var (...)` block at the top of `GenerateCmd`, add `skipHookCompilation bool` to the variable declarations.

2. In the `Run:` function body, near where other options are appended to `opts`, add:
```go
if skipHookCompilation {
    opts = append(opts, entc.SkipHookCompilation())
}
```

3. In the flag-registration block (near line 204), add:
```go
cmd.Flags().BoolVar(&skipHookCompilation, "skip-hook-compilation", false, "AST-strip Hooks/Policy/Interceptors method bodies before loading the schema (use when a hook references a generated symbol that does not yet exist)")
```

The final `GenerateCmd` should still compile and behave identically when the flag is not set.

- [ ] **Step 3: Build and smoke-test**

Run: `go build ./cmd/ent`

Expected: clean build.

Smoke-test by running the binary's help:

Run: `go run ./cmd/ent generate --help 2>&1 | grep -A1 'skip-hook-compilation'`

Expected: the help text shows the new flag with its description.

- [ ] **Step 4: Commit**

**Re-verify branch.**

```bash
git add cmd/internal/base/base.go
git commit -m "feat(cmd/ent): --skip-hook-compilation flag for bootstrap mode

Wires entc.SkipHookCompilation() into the generate subcommand, so
'ent generate --skip-hook-compilation ./ent/schema' bypasses the
schema-package type-check deadlock when hooks reference not-yet-
generated mutation methods.

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 10: Documentation page

Add a doc page explaining when and why to use bootstrap mode.

**Files:**
- Create: `doc/md/devx-bootstrap.md`

- [ ] **Step 1: Write the doc**

Create `doc/md/devx-bootstrap.md`:

```markdown
---
id: devx-bootstrap
title: Bootstrap mode (hook chicken-and-egg)
---

## When you need it

You added a new field to a schema and an existing hook references the
not-yet-generated mutation method:

```go
// ent/schema/user.go
func (User) Hooks() []ent.Hook {
    return []ent.Hook{
        func(next ent.Mutator) ent.Mutator {
            return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
                if um, ok := m.(*ent.UserMutation); ok {
                    um.SetNewField("x") // doesn't exist yet -- you just added the field
                }
                return next.Mutate(ctx, m)
            })
        },
    }
}
```

When you try to regenerate:

```
$ go generate ./ent
ent/schema/user.go:13:8: um.SetNewField undefined (type *ent.UserMutation has no field or method SetNewField)
```

The loader needs to type-check the schema package, but the schema package
imports the generated `ent` package, which doesn't have `SetNewField` yet.
Without bootstrap mode, you have to comment out the hook, regenerate, then
uncomment.

## How bootstrap mode works

Bootstrap mode AST-strips the bodies of `Hooks()`, `Policy()`, and
`Interceptors()` methods on every Schema-implementing type in a **temp
copy** of your schema package. The original source is never touched. The
loader runs against the stripped copy, codegen produces the new methods
(including `SetNewField`), and your real schema package compiles cleanly
on the next build.

## Two ways to enable it

### Automatic (recommended)

As of codegen-epic PR 1, `entc.Generate` automatically falls back to
bootstrap mode when the loader fails with a type-checker error class
(`undefined:`, `has no field or method`, `undeclared name`, `cannot use`).
Snapshot restore is tried first; bootstrap kicks in only if snapshot
recovery also fails.

For most users, this is invisible — codegen just works.

### Explicit

Pass `entc.SkipHookCompilation()` in your generator main, or use the
`--skip-hook-compilation` flag on the CLI:

```go
// ent/generate.go
package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
)

func main() {
    if err := entc.Generate("./schema", &gen.Config{}, entc.SkipHookCompilation()); err != nil {
        log.Fatalf("ent codegen: %v", err)
    }
}
```

```bash
ent generate --skip-hook-compilation ./ent/schema
```

Use the explicit form when you know your schemas always reference
generated types from hooks and you don't want the auto-fallback overhead
of failing once before bootstrap kicks in.

## Limitations

**Hooks in subpackages.** Bootstrap only strips methods in the schema
package itself. If your hooks live in a subpackage that imports the
generated ent package, that subpackage will still fail to type-check.
The standard fix is to keep generated-type-touching hooks in a package
that is *not* imported by your schema package.

**Helpers in the schema package that reference generated types.** The
stripper only touches method bodies on Schema-implementing types. A
top-level helper function in the schema package — say,
`func myValidator(s string) error` — that references generated types
will still fail to compile. Move such helpers to a subpackage that is
not imported by your schema package.

**Schemas with computed Fields()/Edges().** If `Fields()` or `Edges()`
calls a helper that references generated types, bootstrap can't help —
the loader needs those methods to evaluate, and we can't strip them
without losing the schema definitions.
```

- [ ] **Step 2: Commit**

**Re-verify branch.**

```bash
git add doc/md/devx-bootstrap.md
git commit -m "docs: bootstrap mode user guide

Explains when to use bootstrap mode, how to enable it (auto-fallback +
explicit option + CLI flag), and the three documented limitations
(hooks-in-subpackages, schema-package helpers referencing generated
types, computed Fields/Edges).

Part of codegen-epic PR 1 (loader UX / bootstrap mode)."
```

---

## Task 11: Final acceptance check (NO push, NO PR)

Verify all PR 1 acceptance criteria; leave branch local per epic policy.

- [ ] **Step 1: Full entc test suite**

Run: `go test -count=1 ./entc/...`

Expected: every package passes, no `--- FAIL`.

- [ ] **Step 2: Run the two bootstrap regression tests explicitly**

Run: `go test -count=1 -v -run TestBootstrap_Skip ./entc/...`

Expected: both `TestBootstrap_SkipHooksThatReferenceGeneratedMutation` and `TestBootstrap_SkipPolicyThatReferencesGeneratedSymbol` PASS.

- [ ] **Step 3: Verify IsBuildError tests pass**

Run: `go test -count=1 -v -run TestIsBuildError ./entc/internal/...`

Expected: 8 subtests in `TestIsBuildError_TypeCheckerErrors` all PASS.

- [ ] **Step 4: Verify the CLI flag is wired**

Run: `go run ./cmd/ent generate --help 2>&1 | grep -E 'skip-hook-compilation'`

Expected: the flag appears in the help output.

- [ ] **Step 5: Verify the docs file exists**

Run: `ls doc/md/devx-bootstrap.md`

Expected: the file is listed.

- [ ] **Step 6: Verify the branch is still local**

Run: `git rev-parse --abbrev-ref HEAD && git rev-parse master && git config --get branch.worktree-wiggly-singing-pancake.remote || echo "no upstream configured (expected)"`

Expected:
- branch: `worktree-wiggly-singing-pancake`
- master: `7e9d99b1435d541286a773ca128be1a1931d6cc8` (origin/master, untouched)
- upstream: `no upstream configured (expected)`

- [ ] **Step 7: Branch summary**

Run: `git log --oneline master..HEAD | head -30`

Expected: the PR 0 commits (10 of them) PLUS the new PR 1 commits (around 10) — all with proper subjects.

- [ ] **Step 8: NO push, NO PR**

Per epic policy: do not run `git push`, `gh pr create`, `gs branch submit`, or `gs stack submit`. The branch stays local until the end-of-epic batch submission.

---

## Self-Review

### Spec coverage

| Spec §4 PR 1 requirement | Plan task |
|---|---|
| Extend IsBuildError to match `undefined:`, `has no field or method`, `undeclared name`, `cannot use` | Task 1 |
| Each new substring unit-tested | Task 1, Step 1 (TestIsBuildError_TypeCheckerErrors has all 4 + existing) |
| `entc.SkipHookCompilation()` option | Task 5 (option setter + wiring) |
| `--skip-hook-compilation` CLI flag | Task 9 |
| AST-strip Hooks/Policy/Interceptors method bodies into tmpdir | Tasks 2, 3 |
| Preserve method signatures (still implements ent.Schema) | Task 2 (body replaced with `return nil` typed correctly) |
| Loader runs against stripped copy | Task 5 (maybeStageBootstrap rewrites loadPath) |
| Auto-fallback ordering: snapshot first, then bootstrap, then surface original error | Task 6 (mayRecover rewrite) |
| Regression test: schema with Hooks referencing non-existent method | Task 7 |
| Regression test: schema with Policy referencing non-existent method | Task 8 |
| Documentation page | Task 10 |
| Final acceptance (no push, no PR per epic policy) | Task 11 |

All spec requirements have a task. ✓

### Placeholder scan

- No "TBD" / "TODO" / "implement later" — every step has concrete code or commands.
- Step in Task 8 notes "may need to adjust if ent.PolicyFunc doesn't exist" — this is a known runtime detail, not a placeholder; the test's success criterion (mutation reference triggers chicken-and-egg) is clear.
- Step in Task 4 says "If unsure, append it as the last field of the struct" — this is a fallback for placement, not the primary instruction; the primary instruction is "group with other bool fields".

### Type consistency

- `StripHookBodies(src []byte) ([]byte, error)` — used in Task 2 definition AND Task 3 (`StageStrippedSchema` calls it). Consistent.
- `StageStrippedSchema(srcDir string) (string, error)` — defined Task 3, called by Task 5 via `internal.StageStrippedSchema`. Consistent.
- `maybeStageBootstrap(schemaPath string, cfg *gen.Config, forceBootstrap bool) (string, func(), error)` — defined Task 5, called by Task 6. Consistent.
- `cfg.SkipHookCompilation` field — added Task 4, set by Task 5's option, read by Task 5's helper. Consistent.
- `cfg.recovered` field — added Task 5 (Step 4), set by Task 6's mayRecover, read by Task 5's generate. Consistent.
- `hookMethodNames = {"Hooks", "Policy", "Interceptors"}` — defined Task 2, used in Task 7's and Task 8's test setup. Consistent.
- CLI flag name `--skip-hook-compilation` — Task 9 registers it, Task 11 verifies it. Consistent.

No drift. The plan is internally consistent.
