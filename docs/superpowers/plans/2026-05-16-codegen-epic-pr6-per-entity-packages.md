# Codegen Epic PR 6 — Per-Entity Packages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the last four per-type templates (`client/type`, `query`, `mutation/type`, `dialect/sql/entql/type`) from root `ent/` into `ent/<entity>/` sub-packages. Cross-entity references (eager-loader fields, `WithEdge`/`QueryEdge` methods) hoist to a thin per-entity root facade file so sub-packages become leaf packages with zero sibling imports — Go's parallel package compiler can now shard the per-entity working set. Bundles the `cmd/ent-codegen-migrate` receiver-type bug fix plus two new AST passes (for PR 6 edge-method rewrites and the PR 5 typed-edge-accessor regression) so consumer migration is mechanical.

**Architecture:**
1. **`entc/gen/template/builder/query.tmpl`** — strips cross-entity references; `withTeams *TeamQuery` becomes `eagerLoaders map[string]func(context.Context, []*Self) error`; `WithEdge`/`QueryEdge` methods removed; `All()` dispatches stored eager loaders.
2. **`entc/gen/template/dialect/sql/query.tmpl`** — splits `sqlAll` so the parent fetch stays in sub-package (renamed `Fetch`, exported); `loadX` per-edge eager-load implementations move out of sub-package to the facade.
3. **`entc/gen/template/builder/client_type.tmpl`** — strips `QueryEdge` methods; package qualifier drops to empty when InSubPackage.
4. **`entc/gen/template/builder/mutation_type.tmpl`** + **`entc/gen/template/dialect/sql/entql_type.tmpl`** — relocate as-is (cycle-safe); drop their now-unnecessary self-package qualifiers.
5. **NEW `entc/gen/template/facade.tmpl`** — per-entity root file emitting type aliases (`type TaskQuery = task.TaskQuery` etc.) + cross-entity free functions (`WithTaskTeams`, `QueryTaskTeams`, `loadTaskTeams`).
6. **`cmd/ent-codegen-migrate`** — receiver-type bug fix via `golang.org/x/tools/go/packages` + two new AST passes (`rewrite_edge_method.go`, `rewrite_typed_edge_accessor.go`); each pass idempotent.

**Tech Stack:** Go 1.22+ generics (existing `entbuilder.QueryState[P]`), `entgo.io/ent/dialect/sql`, `entgo.io/ent/dialect/sql/sqlgraph`, `entgo.io/ent/runtime/entbuilder`, `golang.org/x/tools/go/ast/astutil`, `golang.org/x/tools/go/packages` (new dependency for type-resolution in migration tool).

**Spec reference:** `docs/superpowers/specs/2026-05-16-codegen-epic-pr6-per-entity-packages-design.md` (PR 6 design); `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` §4 PR 6 (epic context).

**Branch:** stay on `worktree-wiggly-singing-pancake` (epic policy — no separate git-spice branch, no push, no PR).

**Parent commit:** This plan's Task 1 starts on top of `f453859dc spec(codegen-epic): PR 6 per-entity packages design`. Verify with `git log -1 --pretty=format:'%H %s'`.

---

## Scope cuts vs. the design spec — read before starting

The spec §3.4 lists ~18 self-only methods on `*TaskQuery` and ~16 on `*TaskClient` that move into sub-packages. In practice the move is mechanical because the existing `typeScope.PackageQualifier()` and `BuilderImports()` machinery (rolled-back commit `2a22fb09f`, currently active for create/update/delete sub-packages) already handles qualifier rewriting. The plumbing change is small; the template-body refactor for cross-entity stripping is the bulk of the work.

The migration-tool extensions in Phase 1 must be COMPLETE and TESTED before Phase 3 fixture regen, because the regen will produce code that the consumer cannot use without the new rewrites. Phase order is gate-blocking — do not regen fixtures with a broken migration tool.

If during template work you discover an edge metadata field that can't be inlined into the owning entity's `<entity>.go` meta file (a fresh cycle source), stop and surface to Zack — it indicates the design needs revision.

---

## File map

| File | Status | Responsibility |
|---|---|---|
| `cmd/ent-codegen-migrate/typeresolve.go` | **NEW** | `golang.org/x/tools/go/packages`-based receiver type resolver. Shared by all rewrite passes. |
| `cmd/ent-codegen-migrate/typeresolve_test.go` | **NEW** | Unit tests for receiver type resolution (positive + negative cases including schema-DSL false positives). |
| `cmd/ent-codegen-migrate/rewrite_mutation.go` | modify | Add receiver-type gate to `matchMutationCall` dispatch; skip non-`*ent.*Mutation` receivers. |
| `cmd/ent-codegen-migrate/rewrite_mutation_test.go` | modify | Add idempotency test + schema-DSL false-positive regression test. |
| `cmd/ent-codegen-migrate/rewrite_edge_method.go` | **NEW** | AST pass rewriting `q.WithTeams(...)` → `ent.WithTaskTeams(q, ...)` and `c.QueryTeams(t)` → `ent.QueryTaskTeams(c, t)`. |
| `cmd/ent-codegen-migrate/rewrite_edge_method_test.go` | **NEW** | Unit tests (positive + idempotency + schema-DSL skip). |
| `cmd/ent-codegen-migrate/rewrite_typed_edge_accessor.go` | **NEW** | AST pass for PR 5 typed-edge-accessor regression (`m.OwnerID()` → `entbuilder.EdgeIDAs[int](m, "owner")`). |
| `cmd/ent-codegen-migrate/rewrite_typed_edge_accessor_test.go` | **NEW** | Unit tests for typed edge accessor rewrite. |
| `cmd/ent-codegen-migrate/main.go` | modify | Register new passes in `RewritePackage`; add `--rewrite-<pass>` flags for selective runs. |
| `cmd/ent-codegen-migrate/integration_test.go` | modify | Extend privacy-fixture integration test to cover the two new passes. |
| `cmd/ent-codegen-migrate/testdata/before/edges.go.txt` | **NEW** | Hand-written before-state covering edge-method + typed-edge-accessor call sites. |
| `cmd/ent-codegen-migrate/testdata/after/edges.go.txt` | **NEW** | Expected after-state. |
| `entc/gen/template.go` | modify | Flip `SubPackage: true` on 4 templates; update `Format` functions; add `deletedTypeTemplates` entries; register new `facade/type` template. |
| `entc/gen/func.go` | modify | Extend `typeScope.SiblingImports()` (override) to return only cycle-safe imports when `InSubPackage`. |
| `entc/gen/type.go` | modify | If needed, add helper to enumerate edge metadata accessible from facade context (for the new template). |
| `entc/gen/template/facade.tmpl` | **NEW** | Per-entity root facade: type aliases + edge free functions + `loadX` cross-entity helpers. |
| `entc/gen/template/builder/query.tmpl` | **MAJOR** | Strip cross-entity field/method references; introduce opaque `eagerLoaders` map; expose `Fetch` (renamed from sqlAll's parent-fetch portion). |
| `entc/gen/template/dialect/sql/query.tmpl` | **MAJOR** | Split `sqlAll` into parent-only + dispatch-to-loaders; remove `loadX` per-edge methods (move to facade). |
| `entc/gen/template/builder/client_type.tmpl` | modify | Strip `QueryEdge` methods; drop self-package qualifiers (`{{ $.Package }}.` becomes empty in sub-package scope). |
| `entc/gen/template/builder/mutation_type.tmpl` | modify | No content change; verify `import "{module}/internal"` still resolves when InSubPackage. |
| `entc/gen/template/dialect/sql/entql_type.tmpl` | modify | Drop `{{ $.PackageAlias }} "..."` import (now same-package); drop `{{ $n.Package }}.` qualifier prefix on field constants. |
| `entc/integration/**/ent/<entity>/client.go` | regenerate | Fixture regen via `go generate ./entc/integration/...`. |
| `entc/integration/**/ent/<entity>/query.go` | regenerate | Same. |
| `entc/integration/**/ent/<entity>/mutation.go` | regenerate | Same. |
| `entc/integration/**/ent/<entity>/entql.go` | regenerate | Same. |
| `entc/integration/**/ent/<entity>_facade.go` | **NEW (per regen)** | Generated facade for each entity. |
| `entc/integration/**/ent/<entity>_client.go` | DELETED (per regen) | Old root file cleaned up by `deletedTypeTemplates`. |
| `entc/integration/**/ent/<entity>_query.go` | DELETED (per regen) | Same. |
| `entc/integration/**/ent/<entity>_mutation.go` | DELETED (per regen) | Same. |
| `entc/integration/**/ent/<entity>_entql.go` | DELETED (per regen) | Same. |
| `entc/internal/subpackage_imports_test.go` | **NEW** | Regression lint asserting `ent/<entity>/` has zero sibling-entity imports and zero root-`ent`-package imports. |
| `internal/bench/pr6.jsonl` | **NEW** | Bench output recorded post-regen against `service-api-go/api-graphql`. |
| `internal/bench/README.md` | modify | Document PR 6 bench procedure (requires running migration tool against consumer first). |
| `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` | modify | Update §0 progress table: PR 6 ✅. |
| `MIGRATION.md` | modify | Document the API change (`q.WithTeams` → `ent.WithTaskTeams`) and migration-tool workflow (now covers receiver-type fix + new passes). |

---

## Pre-flight: every implementer reads this first

**Branch discipline.** Stay on `worktree-wiggly-singing-pancake`. Never run `git push`, `gh pr create`, `gs branch submit`, or `gs stack submit`. Never `git checkout` to a different branch. **Before EVERY `git commit`, run `pwd && git rev-parse --abbrev-ref HEAD` and verify they return `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake` and `worktree-wiggly-singing-pancake` respectively.** Three prior PR 0-2 implementer subagents committed to master by accident; PRs 3-5 added explicit re-verification and avoided the problem. Do not regress.

**Working directory.** Every command runs from `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake`. Confirm with `pwd` before each command block. If Bash shell state has `cd`'d to a subdirectory (e.g., after Task 18's fixture regen), use absolute paths or explicitly `cd` back to the worktree root.

**Parent commit.** This plan's Task 1 starts on top of `f453859dc spec(codegen-epic): PR 6 per-entity packages design`. Verify with `git log -1 --pretty=format:'%H %s'`.

**Test runner.** Use `go test -count=1` (defeats Go's test cache). Always pass `-count=1` for entc test runs because integration suites hit a real SQLite file path that can carry between runs.

**Shell cwd.** Bash tool's shell state persists across calls. If a command leaves you in a subdirectory, the next command starts from there. Either `cd` back to worktree root or use absolute paths.

**Master sanity.** Periodically (and always before commit) verify `git rev-parse master` is `7e9d99b1435d541286a773ca128be1a1931d6cc8`. Any other value means a previous implementer drifted; recover via the recovery procedure in `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md §11`.

**Phase ordering is gate-blocking.** Phase 1 (migration tool) MUST be complete and tested before Phase 3 (template body changes), because Phase 3 regen produces code that consumers cannot use without the new rewrites. Phase 2 (codegen plumbing) is independent and can run in parallel with Phase 1 if desired.

---

## Phase 1: Migration tool fixes and new passes (Tasks 1-7)

Foundation phase. Phase 3's fixture regen cannot proceed until these pass.

---

## Task 1: Orient — read existing migration tool

Investigative task, no code change. Build the mental model.

**Files to READ:**
- `cmd/ent-codegen-migrate/main.go` (entire — CLI + `RewritePackage` dispatcher)
- `cmd/ent-codegen-migrate/descriptors.go` (entire — descriptor loader)
- `cmd/ent-codegen-migrate/rewrite_mutation.go` (entire — current shape with receiver-type bug at line 146)
- `cmd/ent-codegen-migrate/rewrite_mutation_test.go` (entire — table pattern to mirror)
- `cmd/ent-codegen-migrate/rewrite_predicate.go` (entire — analogous predicate pass)
- `cmd/ent-codegen-migrate/integration_test.go` (entire — fixture-driven coverage)
- `cmd/ent-codegen-migrate/testdata/before/hook.go.txt` and `testdata/after/hook.go.txt`
- `entc/integration/privacy/ent/internal/task_mutation.go` lines 1-90 (descriptor format being consumed)
- Spec §6 (migration tool changes) and §4 (consumer API changes)

- [ ] **Step 1: Read each file.** Confirm:
  - Current `astutil.Apply` dispatch in `RewriteMutationSource` matches purely on method name (line 48 calls `matchMutationCall(sel.Sel.Name)`).
  - No type-resolution machinery exists today — receivers are opaque `ast.Expr` (`sel.X`).
  - The corruption bug: `edge.From("x")` from `entgo.io/ent/schema/edge` is rewritten because `From` matches no pattern... wait, `From` is fine; the actual bug surface is methods like `SetX` that exist on schema-DSL types. Reproduce mentally: schema field `field.String("title").Default("x")` — `Default("x")` would match `matchMutationCall` as action="get" with fieldOrEdge="default"; since no descriptor has a "default" field, it skips. But `field.Time("created_at").Now()` — `Now()` matches as get with "now"; safe. The actual corruption from the brainstorm prompt was `edge.From("x").Ref(...)` getting rewritten to `entbuilder.GetField[string](edge, "from").Ref(...)`. That's because there IS a "from" entry in some descriptor (an edge named "from"? or a field?). Check `entc/integration/privacy` for a field/edge literally named "from" to confirm; if absent, the cause is more subtle (e.g., `lcFirst("From")` = "from" matches a field named "from"). Either way, the fix is the same: gate on receiver type.

- [ ] **Step 2: Reproduce mental model.** Write a one-paragraph note (don't commit) on:
  - How the existing fix-by-receiver-type works once `golang.org/x/tools/go/packages` is loaded.
  - Why `types.Info.TypeOf(sel.X)` returns the resolved type and `types.Implements` could verify "is a *ent.*Mutation" via interface check.

---

## Task 2: Type-aware receiver resolution helper

Add a shared receiver-type resolver to the migration tool. All rewrite passes will gate on it.

**Files:**
- Create: `cmd/ent-codegen-migrate/typeresolve.go`
- Create: `cmd/ent-codegen-migrate/typeresolve_test.go`

- [ ] **Step 1: Write failing test for `MatchesReceiverType`**

```go
// cmd/ent-codegen-migrate/typeresolve_test.go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchesReceiverType_MutationType(t *testing.T) {
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) SetTitle(s string) {}
func hook(m *TaskMutation) { m.SetTitle("hi") }
`
	r, err := NewResolver("hook.go", src)
	require.NoError(t, err)

	// Find the call m.SetTitle("hi") in the AST.
	call := r.FindFirstCall("SetTitle")
	require.NotNil(t, call)

	ok := r.MatchesReceiverType(call, "*x.TaskMutation")
	require.True(t, ok)
}

func TestMatchesReceiverType_SchemaDSLNotMutation(t *testing.T) {
	src := `package x
type EdgeBuilder struct{}
func (e *EdgeBuilder) Ref(name string) *EdgeBuilder { return e }
func From(name string) *EdgeBuilder { return &EdgeBuilder{} }
func use() { From("owner").Ref("tasks") }
`
	r, err := NewResolver("hook.go", src)
	require.NoError(t, err)

	call := r.FindFirstCall("Ref")
	require.NotNil(t, call)

	ok := r.MatchesReceiverType(call, "*x.TaskMutation")
	require.False(t, ok, "EdgeBuilder must not match TaskMutation")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestMatchesReceiverType -v`
Expected: FAIL with `undefined: NewResolver`.

- [ ] **Step 3: Implement `typeresolve.go`**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
)

// Resolver provides type-aware AST navigation for a single Go file.
// It runs the type checker against the parsed file (resolving imports via
// the source importer) so rewrite passes can gate dispatch on the static
// type of a method receiver — necessary to avoid corrupting schema-DSL
// call sites that share method names with generated mutation/query types.
type Resolver struct {
	Fset *token.FileSet
	File *ast.File
	Info *types.Info
	Pkg  *types.Package
}

// NewResolver parses src and runs the type checker. The returned resolver
// is safe for read-only inspection. If type checking fails on some imports
// (common with generated code that depends on external modules), the
// resolver still returns whatever types resolved successfully; callers
// should treat MatchesReceiverType as "best-effort with skip-on-uncertain".
func NewResolver(filename, src string) (*Resolver, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{
		Importer: importer.Default(),
		// IgnoreFuncBodies: false — we need full body types for receivers.
		Error: func(err error) {
			// Swallow per-file errors; partial type info is still useful.
		},
	}
	pkg, _ := conf.Check(file.Name.Name, fset, []*ast.File{file}, info)
	return &Resolver{Fset: fset, File: file, Info: info, Pkg: pkg}, nil
}

// FindFirstCall walks the AST and returns the first ast.CallExpr whose
// Fun is a SelectorExpr with Sel.Name == name. Returns nil if none found.
// Used by tests; production callers walk via astutil.Apply.
func (r *Resolver) FindFirstCall(name string) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(r.File, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == name {
			found = call
			return false
		}
		return true
	})
	return found
}

// MatchesReceiverType reports whether the receiver expression of call
// (call.Fun.(*ast.SelectorExpr).X) has the static type wantTypeName.
// wantTypeName format: "*pkgname.TypeName" (e.g. "*ent.TaskMutation",
// "*x.TaskMutation"). The leading "*" is required for pointer receivers.
//
// Returns false if:
//   - call.Fun isn't a SelectorExpr (e.g., direct function call)
//   - the receiver type can't be resolved (type checker had insufficient info)
//   - the resolved type doesn't match wantTypeName
//
// "False on uncertain" is the safe default — a missed rewrite is fixable
// by a follow-up tool run, but a wrong rewrite corrupts consumer code.
func (r *Resolver) MatchesReceiverType(call *ast.CallExpr, wantTypeName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	tv, ok := r.Info.Types[sel.X]
	if !ok || tv.Type == nil {
		return false
	}
	return tv.Type.String() == wantTypeName
}

// ReceiverTypeMatchesPattern is like MatchesReceiverType but accepts a
// predicate function — useful when the rewrite pass needs to match any
// of several types (e.g. "*ent.*Mutation" for all entity mutations).
func (r *Resolver) ReceiverTypeMatchesPattern(call *ast.CallExpr, pred func(typeName string) bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	tv, ok := r.Info.Types[sel.X]
	if !ok || tv.Type == nil {
		return false
	}
	return pred(tv.Type.String())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestMatchesReceiverType -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add cmd/ent-codegen-migrate/typeresolve.go cmd/ent-codegen-migrate/typeresolve_test.go
git commit -m "$(cat <<'EOF'
feat(cmd/ent-codegen-migrate): type-aware receiver resolver

Adds Resolver helper using go/types + go/importer so rewrite passes
can gate dispatch on receiver static type. Foundation for the
receiver-type bug fix (Task 3) and new PR 6 AST passes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master  # verify still 7e9d99b1...
```

---

## Task 3: Fix receiver-type bug in mutation rewriter

Gate-blocking fix. Without this, the tool corrupts schema files when run against any consumer using the schema DSL.

**Files:**
- Modify: `cmd/ent-codegen-migrate/rewrite_mutation.go` (current bug at line 146)
- Modify: `cmd/ent-codegen-migrate/rewrite_mutation_test.go` (add regression test)

- [ ] **Step 1: Write failing test for schema-DSL false-positive regression**

Add to `rewrite_mutation_test.go`:

```go
func TestRewriteMutation_SkipsSchemaDSLFalsePositive(t *testing.T) {
	// Simulates the bug from the brainstorm prompt: edge.From("owner").Ref("tasks")
	// must not be rewritten as entbuilder.GetField[string](edge, "from").Ref("tasks").
	descs := Descriptors{
		"Task": &EntityDesc{
			Name: "Task",
			Edges: map[string]EdgeDesc{
				"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"},
			},
		},
	}
	src := `package x
type EdgeBuilder struct{}
func (e *EdgeBuilder) Ref(name string) *EdgeBuilder { return e }
func From(name string) *EdgeBuilder { return &EdgeBuilder{} }
func use() { From("owner").Ref("tasks") }
`
	out, err := RewriteMutationSource("schema.go", src, descs)
	require.NoError(t, err)
	require.Equal(t, src, out, "schema-DSL call must be left untouched")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestRewriteMutation_SkipsSchemaDSLFalsePositive -v`
Expected: FAIL — the current matcher rewrites the `From("owner")` call.

- [ ] **Step 3: Modify `RewriteMutationSource` to gate on receiver type**

In `rewrite_mutation.go`, change the function signature and body to build a Resolver and pass it to a new helper `matchesMutationReceiver`. Replace the `astutil.Apply` body:

```go
// RewriteMutationSource parses src, applies mutation API rewrites, and
// returns the rewritten source. Rewrites are gated on the receiver's
// static type being a *<pkg>.<Entity>Mutation — schema-DSL types with
// coincidentally-named methods are left untouched.
//
// Idempotent: re-running on already-transformed code produces the same
// output (the new SetField/GetField call shapes don't match the patterns
// matchMutationCall recognises).
func RewriteMutationSource(filename, src string, descs Descriptors) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", err
	}
	fset := r.Fset
	file := r.File

	needsEntbuilder := false
	astutil.Apply(file, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		_, action, fieldOrEdge, isEdge := matchMutationCall(sel.Sel.Name)
		if action == "" {
			return true
		}
		// Receiver-type gate: rewrite only if the receiver is a
		// *<pkg>.<Entity>Mutation. Schema-DSL receivers (EdgeBuilder,
		// FieldBuilder, etc.) are skipped, which was the bug previously
		// corrupting consumer schema files.
		if action != "where" && !isMutationReceiver(r, call, descs) {
			return true
		}
		// "where" doesn't need descriptor lookup — it's a pure rename.
		if action == "where" {
			// where can apply on mutation receivers OR on the (legacy)
			// untyped predicate where(); gate on receiver too.
			if !isMutationReceiver(r, call, descs) {
				return true
			}
			newCall, _ := rewriteCall(sel.X, action, "", FieldDesc{}, EdgeDesc{}, call.Args)
			if newCall != nil {
				c.Replace(newCall)
			}
			return true
		}
		// (unchanged — descriptor lookup + rewriteCall + replace, see prior body)
		var (
			fd      FieldDesc
			edgeD   EdgeDesc
			matched bool
		)
		for _, ed := range descs {
			if isEdge {
				if e, ok := ed.Edges[fieldOrEdge]; ok {
					edgeD = e
					matched = true
					break
				}
			} else {
				if f, ok := ed.Fields[fieldOrEdge]; ok {
					fd = f
					matched = true
					break
				}
				if action == "cleared" {
					if e, ok := ed.Edges[fieldOrEdge]; ok {
						edgeD = e
						isEdge = true
						matched = true
						break
					}
				}
			}
		}
		if !matched {
			return true
		}
		newCall, addImport := rewriteCall(sel.X, action, fieldOrEdge, fd, edgeD, call.Args)
		if newCall == nil {
			return true
		}
		c.Replace(newCall)
		needsEntbuilder = needsEntbuilder || addImport
		return true
	}, nil)

	if needsEntbuilder {
		astutil.AddImport(fset, file, "entgo.io/ent/runtime/entbuilder")
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// isMutationReceiver reports whether the receiver of call has a static
// type matching any *<pkg>.<Entity>Mutation in descs. Returns false on
// uncertain (no type info) — safer to skip than to corrupt.
func isMutationReceiver(r *Resolver, call *ast.CallExpr, descs Descriptors) bool {
	return r.ReceiverTypeMatchesPattern(call, func(typeName string) bool {
		// typeName looks like "*x.TaskMutation" or "*example.com/foo.TaskMutation".
		// We match the trailing "<Entity>Mutation" against descriptor keys.
		for ent := range descs {
			suffix := "." + ent + "Mutation"
			if strings.HasSuffix(typeName, suffix) && strings.HasPrefix(typeName, "*") {
				return true
			}
		}
		return false
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestRewriteMutation_SkipsSchemaDSLFalsePositive -v`
Expected: PASS.

- [ ] **Step 5: Run all existing migration tool tests**

Run: `go test ./cmd/ent-codegen-migrate/ -count=1 -v`
Expected: all PASS — the receiver-type gate doesn't regress positive cases because the test fixtures use `*TaskMutation` receivers.

- [ ] **Step 6: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add cmd/ent-codegen-migrate/rewrite_mutation.go cmd/ent-codegen-migrate/rewrite_mutation_test.go
git commit -m "$(cat <<'EOF'
fix(cmd/ent-codegen-migrate): gate mutation rewrites on receiver type

Previously matchMutationCall dispatched purely on method name, which
corrupted 116 consumer schema files when run against service-api-go
(edge.From("x").Ref(...) got rewritten as entbuilder.GetField on
the schema-DSL EdgeBuilder receiver).

Resolver from typeresolve.go now type-checks the receiver expression
and skips rewrites unless the static type matches a known
*<pkg>.<Entity>Mutation. False-on-uncertain is the safe default —
a missed rewrite is recoverable; a wrong rewrite is destructive.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 4: Idempotency unit test for mutation pass

Spec §8.3 requires each pass to be idempotent. Add the regression test now so future template/template changes don't reintroduce a non-idempotent shape.

**Files:**
- Modify: `cmd/ent-codegen-migrate/rewrite_mutation_test.go`

- [ ] **Step 1: Add idempotency test**

```go
func TestRewriteMutation_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:   "Task",
			Fields: map[string]FieldDesc{"title": {GoName: "Title", Type: "string"}},
			Edges:  map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) SetTitle(s string) {}
func (m *TaskMutation) Title() (string, bool) { return "", false }
func (m *TaskMutation) AddTeamIDs(ids ...int) {}
func hook(m *TaskMutation) {
	m.SetTitle("hi")
	v, _ := m.Title()
	_ = v
	m.AddTeamIDs(1, 2)
}
`
	pass1, err := RewriteMutationSource("hook.go", src, descs)
	require.NoError(t, err)
	require.NotEqual(t, src, pass1, "first pass must transform the source")

	pass2, err := RewriteMutationSource("hook.go", pass1, descs)
	require.NoError(t, err)
	require.Equal(t, pass1, pass2, "second pass must be a no-op (idempotent)")
}
```

- [ ] **Step 2: Run test**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestRewriteMutation_Idempotent -v`
Expected: PASS — the rewritten shapes (`m.SetField("title", "hi")`, `entbuilder.GetField[string](m, "title")`, `m.AddEdgeIDs(...)`) don't match `matchMutationCall` patterns (they start with `entbuilder.` or use `SetField`/`AddEdgeIDs` — no recognized prefix).

If it FAILS, investigate which transformed shape is still matching the pattern (likely `SetField` getting re-rewritten as action="set" with fieldOrEdge="field" — but that descriptor lookup would miss since no field is named "field"). Fix by tightening `matchMutationCall` to reject `SetField`/`GetField`/`AddEdgeIDs`/`RemoveEdgeIDs`/`SetEdgeID`/`EdgeIDAs`/`EdgeIDsAs`/`FieldCleared`/`EdgeCleared`/`ClearField`/`ResetField` as starting prefixes — explicit allowlist.

- [ ] **Step 3: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add cmd/ent-codegen-migrate/rewrite_mutation_test.go
git commit -m "$(cat <<'EOF'
test(cmd/ent-codegen-migrate): mutation pass idempotency gate

Spec §8.3 requires each rewrite pass to be idempotent so partial
re-runs (or re-runs after PR 5 follow-up cleanup) don't double-
transform consumer code.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 5: New AST pass — rewrite_edge_method.go

Rewrites `q.WithTeams(...)` → `ent.WithTaskTeams(q, ...)` and `c.QueryTeams(t)` → `ent.QueryTaskTeams(c, t)`. The pass gates on receiver type (`*ent.*Query` or `*ent.*Client`) and depends on knowing the entity name to construct the free-function name.

**Files:**
- Create: `cmd/ent-codegen-migrate/rewrite_edge_method.go`
- Create: `cmd/ent-codegen-migrate/rewrite_edge_method_test.go`

- [ ] **Step 1: Write failing test**

```go
// cmd/ent-codegen-migrate/rewrite_edge_method_test.go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteEdgeMethod_With(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskQuery struct{}
type TeamQuery struct{}
func (q *TaskQuery) WithTeams(opts ...func(*TeamQuery)) *TaskQuery { return q }
func use(q *TaskQuery) *TaskQuery { return q.WithTeams(func(q *TeamQuery) {}) }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, "WithTaskTeams(q, func(q *TeamQuery) {")
	require.NotContains(t, out, "q.WithTeams(")
}

func TestRewriteEdgeMethod_QueryEdge(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskClient struct{}
type Task struct{}
type TeamQuery struct{}
func (c *TaskClient) QueryTeams(t *Task) *TeamQuery { return &TeamQuery{} }
func use(c *TaskClient, t *Task) *TeamQuery { return c.QueryTeams(t) }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, "QueryTaskTeams(c, t)")
	require.NotContains(t, out, "c.QueryTeams(t)")
}

func TestRewriteEdgeMethod_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskQuery struct{}
type TeamQuery struct{}
func (q *TaskQuery) WithTeams(opts ...func(*TeamQuery)) *TaskQuery { return q }
func WithTaskTeams(q *TaskQuery, opts ...func(*TeamQuery)) *TaskQuery { return q }
func use(q *TaskQuery) *TaskQuery { return q.WithTeams(func(q *TeamQuery) {}) }
`
	pass1, err := RewriteEdgeMethodSource("x.go", src, descs)
	require.NoError(t, err)
	pass2, err := RewriteEdgeMethodSource("x.go", pass1, descs)
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
}

func TestRewriteEdgeMethod_SkipsNonQueryReceiver(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type OtherType struct{}
func (o *OtherType) WithTeams(opts ...func()) *OtherType { return o }
func use(o *OtherType) *OtherType { return o.WithTeams(func() {}) }
`
	out, err := RewriteEdgeMethodSource("x.go", src, descs)
	require.NoError(t, err)
	require.Equal(t, src, out, "non-Query receiver must be skipped")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestRewriteEdgeMethod -v`
Expected: FAIL with `undefined: RewriteEdgeMethodSource`.

- [ ] **Step 3: Implement `rewrite_edge_method.go`**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// RewriteEdgeMethodSource parses src and rewrites:
//
//   q.With<Edge>(opts...)  →  ent.With<Entity><Edge>(q, opts...)
//   c.Query<Edge>(t)       →  ent.Query<Entity><Edge>(c, t)
//
// where q is *<pkg>.<Entity>Query and c is *<pkg>.<Entity>Client.
// Other receivers are left untouched.
//
// Idempotent: re-running on already-transformed code is a no-op (the
// new shapes are plain function calls without a "With"/"Query"-prefixed
// method on a Query/Client receiver).
func RewriteEdgeMethodSource(filename, src string, descs Descriptors) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	astutil.Apply(r.File, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		methodName := sel.Sel.Name

		// Try With<Edge> against *<pkg>.<Entity>Query receivers.
		if edge, ok := strings.CutPrefix(methodName, "With"); ok {
			entity, matched := matchEdgeReceiver(r, call, descs, "Query", edge)
			if matched {
				c.Replace(&ast.CallExpr{
					Fun:  ast.NewIdent("ent.With" + entity + edge),
					Args: append([]ast.Expr{sel.X}, call.Args...),
				})
				return true
			}
		}
		// Try Query<Edge> against *<pkg>.<Entity>Client receivers.
		if edge, ok := strings.CutPrefix(methodName, "Query"); ok {
			entity, matched := matchEdgeReceiver(r, call, descs, "Client", edge)
			if matched {
				c.Replace(&ast.CallExpr{
					Fun:  ast.NewIdent("ent.Query" + entity + edge),
					Args: append([]ast.Expr{sel.X}, call.Args...),
				})
				return true
			}
		}
		return true
	}, nil)

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), r.File); err != nil {
		return "", err
	}
	// Re-parse + re-print to normalise whitespace introduced by the
	// raw-identifier construction above.
	return buf.String(), nil
}

// matchEdgeReceiver inspects call's receiver type. If it's *<pkg>.<Entity><suffix>
// and edgeName (lowercased) is a known edge on <Entity>, returns the entity
// name and true. Otherwise returns "", false.
//
// suffix is "Query" or "Client".
// edge is the pascal-case edge name from the method (e.g. "Teams" for WithTeams).
func matchEdgeReceiver(r *Resolver, call *ast.CallExpr, descs Descriptors, suffix, edge string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	tv, ok := r.Info.Types[sel.X]
	if !ok || tv.Type == nil {
		return "", false
	}
	typeName := tv.Type.String() // e.g. "*x.TaskQuery"
	for ent, ed := range descs {
		want := "." + ent + suffix
		if !strings.HasSuffix(typeName, want) || !strings.HasPrefix(typeName, "*") {
			continue
		}
		// Edge names in descriptors are lowercase singular or plural
		// (matching the schema's edge name). Method names are pascal-case
		// of the edge name. Try the obvious lowercase first.
		edgeKey := lcFirst(edge)
		if _, ok := ed.Edges[edgeKey]; ok {
			return ent, true
		}
		// Fallback: method might be "WithTeams" for edge "teams" (lowercase
		// already matches). Or "WithOwner" for edge "owner". Both covered.
		// No second fallback needed.
		_ = edgeKey
	}
	return "", false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestRewriteEdgeMethod -v`
Expected: PASS (all 4 subtests).

If the `Fun: ast.NewIdent("ent.With...")` approach trips Go's parser when re-printed (it expects a `SelectorExpr`, not an Ident with dots), switch to constructing a proper SelectorExpr:

```go
Fun: &ast.SelectorExpr{
    X:   ast.NewIdent("ent"),
    Sel: ast.NewIdent("With" + entity + edge),
}
```

Re-run the test until PASS.

- [ ] **Step 5: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add cmd/ent-codegen-migrate/rewrite_edge_method.go cmd/ent-codegen-migrate/rewrite_edge_method_test.go
git commit -m "$(cat <<'EOF'
feat(cmd/ent-codegen-migrate): edge-method AST pass

New pass for PR 6 API change:
  q.WithTeams(...)   → ent.WithTaskTeams(q, ...)
  c.QueryTeams(t)    → ent.QueryTaskTeams(c, t)

Receiver-type-gated against *<pkg>.<Entity>Query and
*<pkg>.<Entity>Client. Schema-DSL false positives skipped.
Idempotent.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 6: New AST pass — rewrite_typed_edge_accessor.go

PR 5 removed typed edge accessors (`m.OwnerID()`, `m.ListingIDs()`). 263+ call sites in service-api-go alone. The migration tool needs to rewrite these to the generic API.

**Files:**
- Create: `cmd/ent-codegen-migrate/rewrite_typed_edge_accessor.go`
- Create: `cmd/ent-codegen-migrate/rewrite_typed_edge_accessor_test.go`

- [ ] **Step 1: Write failing test**

```go
// cmd/ent-codegen-migrate/rewrite_typed_edge_accessor_test.go
package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteTypedEdgeAccessor_OneID(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) OwnerID() (int, bool) { return 0, false }
func use(m *TaskMutation) { id, ok := m.OwnerID(); _ = id; _ = ok }
`
	out, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.EdgeIDAs[int](m, "owner")`)
	require.NotContains(t, out, "m.OwnerID(")
}

func TestRewriteTypedEdgeAccessor_ManyIDs(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"teams": {Cardinality: "entbuilder.M2M", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) TeamsIDs() []int { return nil }
func use(m *TaskMutation) { ids := m.TeamsIDs(); _ = ids }
`
	out, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	require.Contains(t, out, `entbuilder.EdgeIDsAs[int](m, "teams")`)
	require.NotContains(t, out, "m.TeamsIDs(")
}

func TestRewriteTypedEdgeAccessor_Idempotent(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
type TaskMutation struct{}
func (m *TaskMutation) OwnerID() (int, bool) { return 0, false }
func use(m *TaskMutation) { id, ok := m.OwnerID(); _ = id; _ = ok }
`
	pass1, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	pass2, err := RewriteTypedEdgeAccessorSource("x.go", pass1, descs)
	require.NoError(t, err)
	require.Equal(t, pass1, pass2)
	require.True(t, strings.Contains(pass2, "entbuilder.EdgeIDAs"), "transformation should persist")
}

func TestRewriteTypedEdgeAccessor_SkipsNonMutation(t *testing.T) {
	descs := Descriptors{
		"Task": &EntityDesc{
			Name:  "Task",
			Edges: map[string]EdgeDesc{"owner": {Cardinality: "entbuilder.M2O", TargetIDType: "int"}},
		},
	}
	src := `package x
type Helper struct{}
func (h *Helper) OwnerID() (int, bool) { return 0, false }
func use(h *Helper) { id, ok := h.OwnerID(); _ = id; _ = ok }
`
	out, err := RewriteTypedEdgeAccessorSource("x.go", src, descs)
	require.NoError(t, err)
	require.Equal(t, src, out, "non-mutation receiver must be skipped")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestRewriteTypedEdgeAccessor -v`
Expected: FAIL with `undefined: RewriteTypedEdgeAccessorSource`.

- [ ] **Step 3: Implement**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// RewriteTypedEdgeAccessorSource rewrites the typed edge accessors that
// PR 5 removed from generated <entity>_mutation.go:
//
//   m.<Edge>ID()     →  entbuilder.EdgeIDAs[<TargetIDType>](m, "<edge>")
//   m.<Edge>IDs()    →  entbuilder.EdgeIDsAs[<TargetIDType>](m, "<edge>")
//
// Gated on the receiver being *<pkg>.<Entity>Mutation and <edge> matching
// a known edge in descs[entity].Edges.
//
// Idempotent.
func RewriteTypedEdgeAccessorSource(filename, src string, descs Descriptors) (string, error) {
	r, err := NewResolver(filename, src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	needsEntbuilder := false

	astutil.Apply(r.File, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := sel.Sel.Name

		// Match <Edge>IDs (plural) before <Edge>ID (singular) — order matters.
		if edge, ok := strings.CutSuffix(name, "IDs"); ok && edge != "" {
			ent, edgeName, td, matched := matchEdgeOnMutation(r, call, descs, edge, true)
			if matched {
				newCall := buildEdgeIDsCall(sel.X, edgeName, td.TargetIDType)
				c.Replace(newCall)
				needsEntbuilder = true
				_ = ent // for future logging
				return true
			}
		}
		if edge, ok := strings.CutSuffix(name, "ID"); ok && edge != "" {
			ent, edgeName, td, matched := matchEdgeOnMutation(r, call, descs, edge, false)
			if matched {
				newCall := buildEdgeIDCall(sel.X, edgeName, td.TargetIDType)
				c.Replace(newCall)
				needsEntbuilder = true
				_ = ent
				return true
			}
		}
		return true
	}, nil)

	if needsEntbuilder {
		astutil.AddImport(r.Fset, r.File, "entgo.io/ent/runtime/entbuilder")
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), r.File); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func matchEdgeOnMutation(r *Resolver, call *ast.CallExpr, descs Descriptors, edge string, plural bool) (entity, edgeName string, td EdgeDesc, ok bool) {
	sel, sok := call.Fun.(*ast.SelectorExpr)
	if !sok {
		return "", "", EdgeDesc{}, false
	}
	tv, tok := r.Info.Types[sel.X]
	if !tok || tv.Type == nil {
		return "", "", EdgeDesc{}, false
	}
	typeName := tv.Type.String()
	for ent, ed := range descs {
		suffix := "." + ent + "Mutation"
		if !strings.HasSuffix(typeName, suffix) || !strings.HasPrefix(typeName, "*") {
			continue
		}
		// edge from method "OwnerID" is "Owner"; descriptor key is "owner".
		key := lcFirst(edge)
		if plural {
			// "TeamsIDs" → edge=Teams → key=teams (matches descriptor "teams").
			// No suffix mutation needed because the method already ends in "s".
		}
		if d, exists := ed.Edges[key]; exists {
			return ent, key, d, true
		}
	}
	return "", "", EdgeDesc{}, false
}

func buildEdgeIDCall(recv ast.Expr, edge, idType string) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("EdgeIDAs")},
			Index: ast.NewIdent(idType),
		},
		Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", edge)}},
	}
}

func buildEdgeIDsCall(recv ast.Expr, edge, idType string) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X:     &ast.SelectorExpr{X: ast.NewIdent("entbuilder"), Sel: ast.NewIdent("EdgeIDsAs")},
			Index: ast.NewIdent(idType),
		},
		Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", edge)}},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestRewriteTypedEdgeAccessor -v`
Expected: PASS (all 4 subtests).

- [ ] **Step 5: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add cmd/ent-codegen-migrate/rewrite_typed_edge_accessor.go cmd/ent-codegen-migrate/rewrite_typed_edge_accessor_test.go
git commit -m "$(cat <<'EOF'
feat(cmd/ent-codegen-migrate): typed edge accessor AST pass

Rewrites the typed accessors PR 5 removed:
  m.OwnerID()      → entbuilder.EdgeIDAs[int](m, "owner")
  m.TeamsIDs()     → entbuilder.EdgeIDsAs[int](m, "teams")

263+ call sites in service-api-go alone. Receiver-gated on
*<pkg>.<Entity>Mutation, edge name resolved via descriptor.
Idempotent.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 7: Wire new passes into RewritePackage + integration test

Register the two new passes in `main.go`'s `RewritePackage`. Extend the existing privacy-fixture integration test to cover them.

**Files:**
- Modify: `cmd/ent-codegen-migrate/main.go`
- Modify: `cmd/ent-codegen-migrate/integration_test.go`
- Create: `cmd/ent-codegen-migrate/testdata/before/edges.go.txt`
- Create: `cmd/ent-codegen-migrate/testdata/after/edges.go.txt`

- [ ] **Step 1: Update `RewritePackage` to call new passes**

In `main.go`, modify the rewrite chain:

```go
// RewritePackage walks pkgPath for .go files (excluding _test.go and any
// path matching */ent/* generated trees) and applies all rewriters in
// the canonical order: mutation → predicate → edge-method → typed-edge-accessor.
func RewritePackage(pkgPath string, descs Descriptors, dryRun bool) error {
	return filepath.WalkDir(pkgPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "gen" || d.Name() == "ent" || d.Name() == "internal" {
				if strings.Contains(path, "/ent/") || strings.HasSuffix(path, "/ent") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out := string(src)
		// Each pass is idempotent; order matters because later passes
		// may inspect shapes produced by earlier ones.
		for _, pass := range []struct {
			name string
			fn   func(string, string, Descriptors) (string, error)
		}{
			{"mutation", RewriteMutationSource},
			{"predicate", RewritePredicateSource},
			{"edge-method", RewriteEdgeMethodSource},
			{"typed-edge-accessor", RewriteTypedEdgeAccessorSource},
		} {
			out, err = pass.fn(path, out, descs)
			if err != nil {
				return fmt.Errorf("%s: %s rewrite: %w", path, pass.name, err)
			}
		}
		if out == string(src) {
			return nil
		}
		if dryRun {
			fmt.Printf("--- %s (would rewrite) ---\n", path)
			return nil
		}
		return os.WriteFile(path, []byte(out), 0o644)
	})
}
```

- [ ] **Step 2: Create testdata for edge fixtures**

`cmd/ent-codegen-migrate/testdata/before/edges.go.txt`:

```go
package x

type TaskMutation struct{}
type TaskQuery struct{}
type TaskClient struct{}
type Task struct{}
type TeamQuery struct{}

func (m *TaskMutation) OwnerID() (int, bool)              { return 0, false }
func (m *TaskMutation) TeamsIDs() []int                   { return nil }
func (q *TaskQuery) WithTeams(opts ...func(*TeamQuery)) *TaskQuery { return q }
func (c *TaskClient) QueryTeams(t *Task) *TeamQuery       { return &TeamQuery{} }

func useM(m *TaskMutation) {
	id, _ := m.OwnerID()
	ids := m.TeamsIDs()
	_ = id
	_ = ids
}
func useQ(q *TaskQuery) *TaskQuery {
	return q.WithTeams(func(q *TeamQuery) {})
}
func useC(c *TaskClient, t *Task) *TeamQuery {
	return c.QueryTeams(t)
}
```

`cmd/ent-codegen-migrate/testdata/after/edges.go.txt`:

```go
package x

import "entgo.io/ent/runtime/entbuilder"

type TaskMutation struct{}
type TaskQuery struct{}
type TaskClient struct{}
type Task struct{}
type TeamQuery struct{}

func (m *TaskMutation) OwnerID() (int, bool)                       { return 0, false }
func (m *TaskMutation) TeamsIDs() []int                            { return nil }
func (q *TaskQuery) WithTeams(opts ...func(*TeamQuery)) *TaskQuery { return q }
func (c *TaskClient) QueryTeams(t *Task) *TeamQuery                { return &TeamQuery{} }

func useM(m *TaskMutation) {
	id, _ := entbuilder.EdgeIDAs[int](m, "owner")
	ids := entbuilder.EdgeIDsAs[int](m, "teams")
	_ = id
	_ = ids
}
func useQ(q *TaskQuery) *TaskQuery {
	return ent.WithTaskTeams(q, func(q *TeamQuery) {})
}
func useC(c *TaskClient, t *Task) *TeamQuery {
	return ent.QueryTaskTeams(c, t)
}
```

(Note: the "after" file references `ent.WithTaskTeams` even though no `ent` package is imported here — this is intentional, because the rewriter doesn't add the `ent` import. In real consumer code the `ent` import already exists from other call sites. The fixture documents the textual output; if go vet complains in this fixture file alone, that's expected — the file isn't compiled, only string-compared.)

- [ ] **Step 3: Add integration test against the fixture**

In `integration_test.go`, add:

```go
func TestIntegration_AllPassesAgainstEdgesFixture(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..")
	descsDir := filepath.Join(root, "entc/integration/privacy/ent/internal")

	descs, err := LoadDescriptors(descsDir)
	require.NoError(t, err)
	require.NotEmpty(t, descs)

	before, err := os.ReadFile(filepath.Join("testdata", "before", "edges.go.txt"))
	require.NoError(t, err)
	wantAfter, err := os.ReadFile(filepath.Join("testdata", "after", "edges.go.txt"))
	require.NoError(t, err)

	out := string(before)
	for _, fn := range []func(string, string, Descriptors) (string, error){
		RewriteMutationSource,
		RewritePredicateSource,
		RewriteEdgeMethodSource,
		RewriteTypedEdgeAccessorSource,
	} {
		out, err = fn("edges.go", out, descs)
		require.NoError(t, err)
	}
	require.Equal(t, string(wantAfter), out)
}
```

- [ ] **Step 4: Run integration test**

Run: `go test ./cmd/ent-codegen-migrate/ -run TestIntegration -v -count=1`
Expected: both `TestIntegration_RewritePrivacyFixtureHooks` and the new `TestIntegration_AllPassesAgainstEdgesFixture` PASS.

If the new integration test fails on whitespace or import-ordering, adjust either the testdata `after` file or run the output through `gofmt` before string comparison. (Keep the assertion exact — fix the fixture to match real output, since real consumer rewriting is also exact.)

- [ ] **Step 5: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add cmd/ent-codegen-migrate/main.go cmd/ent-codegen-migrate/integration_test.go cmd/ent-codegen-migrate/testdata/before/edges.go.txt cmd/ent-codegen-migrate/testdata/after/edges.go.txt
git commit -m "$(cat <<'EOF'
feat(cmd/ent-codegen-migrate): wire PR 5+6 rewrites into RewritePackage

Adds the two new passes to the canonical rewrite chain:
  mutation → predicate → edge-method → typed-edge-accessor

Each pass is idempotent (Tasks 4-6); chain stays safe to re-run.
Integration test covers all four passes against an edges fixture.

Phase 1 of PR 6 plan complete — migration tool is now safe to run
against consumer code that uses the post-PR6 generated API.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Phase 2: Codegen plumbing (Tasks 8-11)

Wire the four templates as SubPackage outputs, add deleted-file cleanup, extend typeScope, and register the new facade/type template. No template body changes yet — those happen in Phase 3.

---

## Task 8: Orient — read codegen plumbing

Investigative task, no code change.

**Files to READ:**
- `entc/gen/template.go` (entire — `Templates`, `GraphTemplates`, `deletedTemplates`, `deletedTypeTemplates`, `pkgf`)
- `entc/gen/graph.go` lines 249-340 (`generate` function — how templates are dispatched, where `typeScope{InSubPackage: true}` wrap happens at line 277)
- `entc/gen/func.go` lines 263-300 (`typeScope` struct + `PackageQualifier()` + `BuilderImports()` overrides — the existing cycle-break machinery)
- `entc/gen/type.go` lines 950-1000 (Type methods `BuilderImports`, `SiblingImports` — what gets imported per builder context)
- `entc/gen/assets.go` (entire — how generated content is written to disk, how `cleanOldNodes` removes obsolete files)

- [ ] **Step 1: Read each file.** Confirm:
  - `Templates` slice currently has 4 entries with `SubPackage: true` (shared, create, update, delete). The 4 PR 6 targets (client/type, query, mutation/type, dialect/sql/entql/type) have `SubPackage: false` (default).
  - `graph.go:276-278` wraps the type in `typeScope{InSubPackage: true}` only when `tmpl.SubPackage` is set — so flipping the flag triggers the existing machinery.
  - `Format` for the 4 targets uses `pkgf("%s_<file>.go")` — produces root paths. Sub-package output needs `<entity>/<file>.go` paths instead.
  - `deletedTypeTemplates` cleans up old file patterns. Old root paths (`%s_client.go` etc.) must be added so regen removes them.
  - `Type.SiblingImports()` returns ALL sibling entity imports (used today by root-emitted client/query). For sub-package contexts those imports would create cycles — needs to be overridden to return nil.

---

## Task 9: Flip SubPackage flag + update Format + add deleted-file entries

**Files:**
- Modify: `entc/gen/template.go`

- [ ] **Step 1: Write failing test asserting new format paths**

Add a focused test to `entc/gen/template_test.go` (create if absent):

```go
// entc/gen/template_test.go
package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplates_PR6SubPackageFormat(t *testing.T) {
	ty := &Type{Name: "Task"}
	want := map[string]string{
		"client/type":             "task/client.go",
		"query":                   "task/query.go",
		"mutation/type":           "task/mutation.go",
		"dialect/sql/entql/type":  "task/entql.go",
	}
	got := map[string]string{}
	for _, tmpl := range Templates {
		if _, ok := want[tmpl.Name]; ok {
			require.True(t, tmpl.SubPackage, "template %q must be SubPackage", tmpl.Name)
			got[tmpl.Name] = tmpl.Format(ty)
		}
	}
	require.Equal(t, want, got)
}

func TestTemplates_PR6DeletedTemplates(t *testing.T) {
	want := []string{"%s_client.go", "%s_query.go", "%s_mutation.go", "%s_entql.go"}
	for _, p := range want {
		require.Contains(t, deletedTypeTemplates, p, "deletedTypeTemplates missing %q", p)
	}
}

func TestTemplates_FacadeRegistered(t *testing.T) {
	ty := &Type{Name: "Task"}
	for _, tmpl := range Templates {
		if tmpl.Name == "facade/type" {
			require.False(t, tmpl.SubPackage, "facade/type must NOT be SubPackage (lives at root)")
			require.Equal(t, "task_facade.go", tmpl.Format(ty))
			return
		}
	}
	t.Fatal("facade/type template not registered")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./entc/gen/ -run TestTemplates_PR6 -v`
Expected: FAIL — flags and entries not yet in place.

- [ ] **Step 3: Update `template.go`**

In `entc/gen/template.go`:

```go
// Helper for sub-package formats — sub-package file lives at <PackageDir>/<file>.go.
func subpkgf(s string) func(t *Type) string {
	return func(t *Type) string { return fmt.Sprintf("%s/"+s, t.PackageDir()) }
}
```

Modify the four template entries:

```go
{
    Name:   "client/type",
    Format: subpkgf("client.go"),
    SubPackage: true,
},
{
    Name:   "query",
    Format: subpkgf("query.go"),
    ExtendPatterns: []string{
        "dialect/*/query/fields/additional/*",
    },
    SubPackage: true,
},
// ...
{
    Name:   "mutation/type",
    Cond:   notView,
    Format: subpkgf("mutation.go"),
    SubPackage: true,
},
{
    Name: "dialect/sql/entql/type",
    Cond: func(t *Type) bool {
        return t.featureEnabled(FeatureEntQL)
    },
    Format: subpkgf("entql.go"),
    SubPackage: true,
},
```

Add the new facade entry to `Templates`:

```go
{
    Name:   "facade/type",
    Cond:   notView,
    Format: pkgf("%s_facade.go"),
    // SubPackage: false — emits at root.
},
```

Extend `deletedTypeTemplates`:

```go
deletedTypeTemplates = []string{
    "%s_create.go",
    "%s_update.go",
    "%s_delete.go",
    "%s/client.go",
    // PR 6 additions: old root files replaced by sub-package equivalents.
    "%s_client.go",
    "%s_query.go",
    "%s_mutation.go",
    "%s_entql.go",
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./entc/gen/ -run TestTemplates_PR6 -v`
Expected: 3 PASS.

Also run the full gen test suite to detect breakage from the format change:

Run: `go test ./entc/gen/... -count=1 -short`
Expected: many FAIL (the existing snapshot tests compare against pre-PR6 file paths). These get fixed by Task 18's fixture regen — note the failures but continue. Phase 2 task 11 is just the plumbing.

- [ ] **Step 5: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/template.go entc/gen/template_test.go
git commit -m "$(cat <<'EOF'
feat(entc/gen): flip 4 templates to SubPackage + register facade

PR 6 plumbing — flips client/type, query, mutation/type, and
dialect/sql/entql/type templates to SubPackage:true so graph.go
wraps the type in typeScope{InSubPackage: true} on execute.

Adds new facade/type per-type template emitting <entity>_facade.go
at root (will hold type aliases + cross-entity free functions in
Phase 3).

Adds 4 old root-file patterns to deletedTypeTemplates so regen
cleans up the obsolete <entity>_client.go etc.

Template body refactors land in Phase 3; integration fixtures
regenerate in Task 18. Existing gen snapshot tests fail until
both happen — known.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 10: Extend typeScope to drop sibling imports in sub-package context

The existing `Type.SiblingImports()` enumerates all sibling entity packages — required at root, forbidden in sub-package. Override on `typeScope` so the import template gets nil when InSubPackage.

**Files:**
- Modify: `entc/gen/func.go`
- Modify: `entc/gen/func_test.go` (create if absent)

- [ ] **Step 1: Write failing test**

```go
// entc/gen/func_test.go
package gen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypeScope_SiblingImportsInSubPackage(t *testing.T) {
	ty := &Type{Name: "Task" /* construct minimal sibling-bearing type if needed */ }
	scope := &typeScope{Type: ty, Scope: map[any]any{"InSubPackage": true}}
	require.Nil(t, scope.SiblingImports(), "sub-package context must emit zero sibling imports")
}

func TestTypeScope_SiblingImportsAtRoot(t *testing.T) {
	ty := &Type{Name: "Task" /* same minimal type */}
	scope := &typeScope{Type: ty, Scope: map[any]any{}}
	// At root, override delegates to Type.SiblingImports() — exact contents
	// depend on the fixture; here we just assert the scope returns SOMETHING
	// (or nil if the fixture has no siblings).
	_ = scope.SiblingImports() // no panic = ok
}
```

(If the Type fixture is too painful to construct in isolation, replace with a fixture-loading test that uses an existing entc/integration graph.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./entc/gen/ -run TestTypeScope_SiblingImports -v`
Expected: FAIL — `SiblingImports` not yet overridden on typeScope.

- [ ] **Step 3: Implement override**

In `entc/gen/func.go`, add after the existing `BuilderImports` override (~line 287):

```go
// SiblingImports returns sibling entity package imports needed by the
// template. When InSubPackage is set, returns nil — sub-packages MUST
// NOT import sibling entity packages (Phase 1 of PR 6 design moved all
// cross-entity logic to the root facade). At root, delegates to Type.
func (t *typeScope) SiblingImports() []struct{ Alias, Path string } {
	if t.Scope["InSubPackage"] == true {
		return nil
	}
	return t.Type.SiblingImports()
}
```

- [ ] **Step 4: Run test**

Run: `go test ./entc/gen/ -run TestTypeScope_SiblingImports -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/func.go entc/gen/func_test.go
git commit -m "$(cat <<'EOF'
feat(entc/gen): typeScope.SiblingImports drops siblings in sub-package

Companion to PackageQualifier() and BuilderImports() overrides — when
InSubPackage scope is active, SiblingImports() returns nil so the
import template does not emit forbidden ent/<sibling-entity> imports.

Cross-entity references now live exclusively in root facade files
(emitted in Phase 3 by the new facade.tmpl).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 11: Verify Phase 2 plumbing — dry-run gen on smallest integration fixture

Confirm `template.go` + `func.go` changes don't crash the generator. The output WILL be wrong (templates still emit cross-entity code via stale bodies) — that's expected. Goal here is to catch nil-deref or path errors in the plumbing.

**Files:** none (read-only verification)

- [ ] **Step 1: Identify a small integration fixture**

```bash
find entc/integration -name 'generate.go' | head -5
```

Pick the smallest one (privacy or hooks). Note its path.

- [ ] **Step 2: Try generating** *(into a tmp dir so the actual fixture isn't disturbed)*

```bash
# Build the entc binary
go build -o /tmp/entc-pr6 ./entc

# Or invoke via go generate against a throwaway target — use the standard
# generate path but redirect via the schema's entc.go config.

# Simpler: run the existing gen tests in dry-mode to see if the template
# system loads cleanly:
go test ./entc/gen/ -run TestGraph_Gen -count=1 2>&1 | head -50
```

Expected: many test failures (snapshot mismatches), but NO panic / NO "template not found" / NO "fmt: missing argument" errors.

- [ ] **Step 3: If errors appear, diagnose**

Common failure modes:
- "template: client/type: undefined function $.SiblingImports" — typeScope override has a name mismatch with what the template invokes; check that template uses `$.SiblingImports` (no parens; it's a method call resolved at execution).
- "format string %s_facade.go missing argument" — the `pkgf("%s_facade.go")` is called against a graphScope instead of typeScope; verify the facade template is registered as TypeTemplate (per-type) not GraphTemplate.
- panic from `nil` typeScope — graph.go:277 only constructs typeScope when `tmpl.SubPackage`; facade/type has SubPackage:false so it gets the bare `*Type` (no typeScope wrap). That's fine — facade template will reference Type methods directly.

If any errors, fix in place and commit as a fix-up to Task 9 or 10.

- [ ] **Step 4: Verify master + branch sanity**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git rev-parse master   # must still be 7e9d99b1...
git log --oneline -10  # confirm the Phase 1 + Phase 2 commits are stacked correctly
```

No commit in this task unless a plumbing fix was needed.

---

## Phase 3: Template body changes (Tasks 12-17)

The bulk of the work. Each task rewrites one template; together they produce sub-package outputs that compile without cross-entity imports and a root facade that wires the cross-entity edge methods as free functions.

**Workflow inside each task:** edit the template body, run `go test ./entc/gen/... -count=1 -run TestGraph_Gen 2>&1 | grep -E '^---|FAIL'` to identify which fixture snapshots changed, but DON'T regenerate fixtures yet. Fixture regen is Task 18 (one shot at the end of Phase 3 so the regen is deterministic).

Each task commits the template edit only.

---

## Task 12: query.tmpl — strip cross-entity, opaque eager loaders, Fetch

The biggest template rewrite. The current query template has:
- `withTeams *TeamQuery` struct fields (one per edge) — cross-entity types in struct.
- `WithTeams(opts ...func(*TeamQuery))` methods (one per edge) — cross-entity in method signatures.
- `QueryTeams() *TeamQuery` methods (one per edge) — cross-entity returns.
- Clone() copies the typed eager-loader fields.

Target shape:
- Single field `eagerLoaders map[string]func(context.Context, []*<Entity>) error`.
- `StoreEager(name, loader) *<Self>Query` method — exposed for root facade.
- All methods drop the `WithEdge`/`QueryEdge` blocks.
- `Clone()` shallow-copies the eagerLoaders map.
- `All()` invokes eagerLoaders after sqlAll.

**Files:**
- Modify: `entc/gen/template/builder/query.tmpl`

- [ ] **Step 1: Open `entc/gen/template/builder/query.tmpl` in full**

Re-read sections at lines 22-47 (struct decl), 100-118 (QueryEdge methods), 363-376 (WithEdge methods), 332-361 (Clone), 234-246 (All).

- [ ] **Step 2: Replace the struct declaration (lines ~22-47)**

OLD:
```
type {{ $builder }} struct {
	Config
	{{- if eq $.Config.Storage.Name "sql" }}
	entbuilder.QueryState[predicate.{{ $.Name }}]
	order []{{ $.Package }}.OrderOption
	{{- else }}
	ctx			*QueryContext
	order		[]{{ $.Package }}.OrderOption
	inters		[]Interceptor
	predicates 	[]predicate.{{ $.Name }}
	{{- end }}
	{{- /* Eager loading fields. */}}
	{{- range $e := $.Edges }}
		{{ $e.EagerLoadField }} *{{ $e.Type.QueryName }}
	{{- end }}
	{{- /* Additional fields to add to the builder. */}}
	{{- $tmpl := printf "dialect/%s/query/fields" $.Storage }}
	{{- if hasTemplate $tmpl }}
		{{- xtemplate $tmpl . }}
	{{- end }}
	{{- if ne $.Config.Storage.Name "sql" }}
	// intermediate query (i.e. traversal path).
	{{ $.Storage }} {{ $.Storage.Builder }}
	path func(context.Context) ({{ $.Storage.Builder }}, error)
	{{- end }}
}
```

NEW:
```
type {{ $builder }} struct {
	Config
	{{- if eq $.Config.Storage.Name "sql" }}
	entbuilder.QueryState[predicate.{{ $.Name }}]
	order []OrderOption
	{{- else }}
	ctx			*QueryContext
	order		[]OrderOption
	inters		[]Interceptor
	predicates 	[]predicate.{{ $.Name }}
	{{- end }}
	{{- /* Opaque eager loaders, keyed by edge name. Closures captured by
	      root-facade With<Entity><Edge> free functions; sub-package storage
	      is type-erased to keep cycles out of sub-package signatures. */}}
	eagerLoaders map[string]func(context.Context, []*{{ $.Name }}) error
	{{- /* Additional fields to add to the builder. */}}
	{{- $tmpl := printf "dialect/%s/query/fields" $.Storage }}
	{{- if hasTemplate $tmpl }}
		{{- xtemplate $tmpl . }}
	{{- end }}
	{{- if ne $.Config.Storage.Name "sql" }}
	// intermediate query (i.e. traversal path).
	{{ $.Storage }} {{ $.Storage.Builder }}
	path func(context.Context) ({{ $.Storage.Builder }}, error)
	{{- end }}
}

// StoreEager registers an eager-load callback for the given edge name.
// Used by the root facade's With<Entity><Edge> free functions. Returns
// the receiver for chaining.
func ({{ $receiver }} *{{ $builder }}) StoreEager(name string, loader func(context.Context, []*{{ $.Name }}) error) *{{ $builder }} {
	if {{ $receiver }}.eagerLoaders == nil {
		{{ $receiver }}.eagerLoaders = make(map[string]func(context.Context, []*{{ $.Name }}) error)
	}
	{{ $receiver }}.eagerLoaders[name] = loader
	return {{ $receiver }}
}

// EagerLoaders returns the registered eager-load callbacks (read-only).
// Used by the sub-package's All() and by root facade utilities that need
// to iterate registered loaders (e.g. Clone helpers).
func ({{ $receiver }} *{{ $builder }}) EagerLoaders() map[string]func(context.Context, []*{{ $.Name }}) error {
	return {{ $receiver }}.eagerLoaders
}
```

Note: `{{ $.Package }}.OrderOption` becomes plain `OrderOption` because we're in sub-package scope (PackageQualifier returns empty).

- [ ] **Step 3: Remove the QueryEdge methods block (lines ~100-118)**

DELETE the entire `{{ range $e := $.Edges }} ... Query{{ pascal $e.Name }}() ... {{ end }}` block. These now live in the facade template.

- [ ] **Step 4: Remove the WithEdge methods block (lines ~363-376)**

DELETE the entire `{{- range $e := $.Edges }} ... With{{ $e.StructField }} ... {{- end }}` block. These now live in the facade template.

- [ ] **Step 5: Rewrite Clone() to drop typed eager-loader copies (lines ~332-361)**

Replace the per-edge `{{ $e.EagerLoadField }}: {{ $receiver }}.{{ $e.EagerLoadField }}.Clone(),` block with an opaque-map shallow copy:

```
{{ define "/* original Clone func body */" }}
...
return &{{ $builder }}{
    Config: 	{{ $receiver }}.Config,
    {{- if eq $.Config.Storage.Name "sql" }}
    QueryState: *{{ $receiver }}.QueryState.Clone(),
    order:      append([]OrderOption{}, {{ $receiver }}.order...),
    {{- else }}
    ctx: 		{{ $receiver }}.ctx.Clone(),
    order: 		append([]OrderOption{}, {{ $receiver }}.order...),
    inters: 	append([]Interceptor{}, {{ $receiver }}.inters...),
    predicates: append([]predicate.{{ $.Name }}{}, {{ $receiver }}.predicates...),
    {{- end }}
    eagerLoaders: cloneEagerLoaders{{ $.Name }}({{ $receiver }}.eagerLoaders),
    {{- if ne $.Config.Storage.Name "sql" }}
    {{ $.Storage }}: {{ $receiver }}.{{ $.Storage }}.Clone(),
    path: {{ $receiver }}.path,
    {{- end }}
    {{- if $.FeatureEnabled "sql/modifier" }}
        modifiers: append([]func(*sql.Selector){}, {{ $receiver }}.modifiers...),
    {{- end }}
}
```

Add helper at end of template (before final `{{ end }}` of "query" define):

```
// cloneEagerLoaders{{ $.Name }} returns a shallow copy of the map. The
// closures themselves are shared between original and clone (consumers
// should re-register edges on the clone if mutation is required).
func cloneEagerLoaders{{ $.Name }}(in map[string]func(context.Context, []*{{ $.Name }}) error) map[string]func(context.Context, []*{{ $.Name }}) error {
	if in == nil { return nil }
	out := make(map[string]func(context.Context, []*{{ $.Name }}) error, len(in))
	for k, v := range in { out[k] = v }
	return out
}
```

- [ ] **Step 6: Drop `{{ $.Package }}.` qualifier prefixes throughout**

In sub-package scope, `{{ $.Package }}.Label`, `{{ $.Package }}.FieldID`, `{{ $.Package }}.OrderOption` etc. should be unqualified because they're same-package. The existing `PackageQualifier()` returns "" in InSubPackage scope — but the template uses `{{ $.Package }}.` literally. Replace those literal prefixes with `{{ $.PackageQualifier }}` so the override kicks in:

Search-replace in query.tmpl: `{{ $.Package }}.` → `{{ $.PackageQualifier }}` (note: trailing `.` is now part of the qualifier — `PackageQualifier()` returns either "" or "<pkg>." with trailing dot, check func.go:269-277 — verify the impl includes the dot or add it inline as `{{ $.PackageQualifier }}.`).

Check `entc/gen/func.go:269-277` — current impl returns `""` or `t.Type.PackageQualifier()`. Look at what `Type.PackageQualifier()` returns; if it's just the bare pkg name (no dot), template needs `{{ $.PackageQualifier }}.Label`. If it returns `<pkg>.`, template needs `{{ $.PackageQualifier }}Label`. Match accordingly.

- [ ] **Step 7: Verify template parses**

Run: `go test ./entc/gen/ -run TestTemplate_Parse -v -count=1`
(If no such test exists, add a minimal one that just instantiates `MustParse(NewTemplate("templates").ParseFS(templateDir, ...))` — same machinery as `initTemplates`.)
Expected: no panic.

- [ ] **Step 8: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/template/builder/query.tmpl
git commit -m "$(cat <<'EOF'
feat(entc/gen): query template — opaque eager loaders, no cross-entity

Strips typed cross-entity field references (*TeamQuery etc.) from
the per-entity Query struct. Replaces per-edge withX fields with a
single map[string]func(ctx, []*Self) error keyed by edge name.

WithEdge/QueryEdge per-edge methods removed — those move to the
facade template (Task 17) as cross-entity free functions.

Sub-package query is now cycle-safe: it imports only internal,
predicate, entbuilder, and dialect packages. No siblings.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 13: dialect/sql/query.tmpl — split sqlAll, drop loadX methods

`dialect/sql/query.tmpl` currently emits `sqlAll()` (which iterates per-edge eager loads inline) and per-edge `loadX()` methods (which reference `*TeamQuery` etc.). These move out of the sub-package.

Target:
- `sqlAll()` stays in sub-package: fetches PARENT nodes only, returns slice. No eager-load loop.
- New `All()` orchestration: defined at sub-package via top-level `query.tmpl` — call sqlAll, then iterate eagerLoaders map.
- `loadX()` per-edge methods MOVE to facade template (Task 17), where they have full cross-entity access.
- Exposed surface for facade: `Fetch(ctx) ([]*Self, error)` — public wrapper around `sqlAll` so root code can call into sub-package fetch logic without exposing the lowercase `sqlAll`.

**Files:**
- Modify: `entc/gen/template/dialect/sql/query.tmpl`

- [ ] **Step 1: Reread current template** (already read in Task 12 prep)

The eager-load dispatch loop is at lines 79-101 of `dialect/sql/query.tmpl`. The per-edge `loadX` methods are at lines 111-end.

- [ ] **Step 2: Remove the eager-load dispatch loop from `sqlAll`**

Delete the `{{- range $e := $.Edges }} if query := {{ $receiver }}.{{ $e.EagerLoadField }}; query != nil { ... } {{- end }}` block (lines 79-101). The new All() method in `builder/query.tmpl` dispatches via the eagerLoaders map instead.

- [ ] **Step 3: Add exported Fetch method (after sqlAll)**

```
// Fetch executes the SQL parent query only — no eager loading.
// Exposed for the root facade's edge-loading helpers, which call Fetch
// on a sub-query to materialize neighbors during eager dispatch.
func ({{ $receiver }} *{{ $builder }}) Fetch(ctx context.Context, hooks ...queryHook) ([]*{{ $.Name }}, error) {
	return {{ $receiver }}.sqlAll(ctx, hooks...)
}

// PrepareQuery is the exported counterpart of prepareQuery — used by
// root facade eager-load helpers (loadX) that build their own join
// predicates and need to attach interceptors/hooks before running.
func ({{ $receiver }} *{{ $builder }}) PrepareQuery(ctx context.Context) error {
	return {{ $receiver }}.prepareQuery(ctx)
}
```

- [ ] **Step 4: Remove the loadX per-edge methods**

Delete the `{{- range $e := $.Edges }} func ({{ $receiver }} *{{ $builder }}) load{{ $e.StructField }}(...) error { ... } {{- end }}` block at lines 111+. These reappear in the facade template (Task 17).

- [ ] **Step 5: Update the top-level All() in `builder/query.tmpl` to dispatch eager loaders**

In `entc/gen/template/builder/query.tmpl`, locate the `All` function (~line 234):

OLD:
```
func ({{ $receiver }} *{{ $builder }}) All(ctx context.Context) ([]*{{ $.Name }}, error) {
	{{- if eq $.Config.Storage.Name "sql" }}
	return entbuilder.RunAll[[]*{{ $.Name }}](ctx, ...)
	{{- else }}
	...
	{{- end }}
}
```

NEW:
```
func ({{ $receiver }} *{{ $builder }}) All(ctx context.Context) ([]*{{ $.Name }}, error) {
	{{- if eq $.Config.Storage.Name "sql" }}
	nodes, err := entbuilder.RunAll[[]*{{ $.Name }}](ctx, {{ $receiver }}, {{ $receiver }}.Ctx, ent.OpQueryAll, {{ $receiver }}.QueryState.Inters, {{ $receiver }}.prepareQuery, func(ctx context.Context) ([]*{{ $.Name }}, error) { return {{ $receiver }}.sqlAll(ctx) })
	if err != nil { return nil, err }
	if len(nodes) == 0 { return nodes, nil }
	// Dispatch any registered eager loaders. Callbacks were captured by
	// the root facade's With<Entity><Edge> free functions and receive
	// the parent slice for in-place edge assignment.
	for _, loader := range {{ $receiver }}.eagerLoaders {
		if err := loader(ctx, nodes); err != nil { return nil, err }
	}
	return nodes, nil
	{{- else }}
	// (Non-SQL branch — keep existing pre-PR6 shape; gremlin doesn't use
	// the eagerLoaders map in this PR. Future cleanup if needed.)
	ctx = setContextOp(ctx, {{ $receiver }}.ctx, ent.OpQueryAll)
	if err := {{ $receiver }}.prepareQuery(ctx); err != nil { return nil, err }
	qr := querierAll[[]*{{ $.Name }}, *{{ $builder }}]()
	return withInterceptors[[]*{{ $.Name }}](ctx, {{ $receiver }}, qr, {{ $receiver }}.inters)
	{{- end }}
}
```

- [ ] **Step 6: Verify template parses + commit**

Run: `go test ./entc/gen/ -run TestTemplate_Parse -v -count=1`
Expected: no panic.

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/template/dialect/sql/query.tmpl entc/gen/template/builder/query.tmpl
git commit -m "$(cat <<'EOF'
refactor(entc/gen): split sqlAll, expose Fetch/PrepareQuery

Removes the eager-load dispatch loop from sqlAll — that logic moves
to All(), which iterates the opaque eagerLoaders map populated by
the root facade.

Adds Fetch(ctx) and PrepareQuery(ctx) as exported wrappers around
the existing sqlAll/prepareQuery so the root facade's loadX cross-
entity helpers (Task 17) can drive sub-package fetches.

Per-edge loadX methods removed from sub-package; they reappear in
the facade template with full cross-entity access.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 14: client_type.tmpl — drop QueryEdge methods, drop package qualifiers

The Client template emits `Client.QueryTeams(t *Task) *TeamQuery` per edge — cross-entity. Strip those; move to facade. Also drop the `{{ $q }}` (PackageQualifier) prefix on sibling builder type names since in sub-package scope they're now unqualified.

**Files:**
- Modify: `entc/gen/template/builder/client_type.tmpl`

- [ ] **Step 1: Reread template** (lines ~9-249 from Task 1 reading)

- [ ] **Step 2: Remove the QueryEdge methods block (lines ~185-208)**

Delete the `{{ range $e := $n.Edges }} ... Query{{ pascal $e.Name }}() ... {{ end }}` block. These move to facade.

- [ ] **Step 3: Adjust the type-name qualifier**

The template currently uses `{{ $q }}` (which is `$n.PackageQualifier`) before builder names — e.g., `{{ $q }}{{ $n.CreateName }}` produces `task.TaskCreate` at root scope, empty in sub-package scope. With the move to sub-package, these become bare type names. Verify the `$q` definition at line 21 (`{{ $q := $n.PackageQualifier }}`) still works in typeScope context (it should — typeScope embeds *Type and overrides PackageQualifier).

No template change needed if the existing override is correct; just rerun parse to confirm:

Run: `go test ./entc/gen/ -run TestTemplate_Parse -v -count=1`

- [ ] **Step 4: Drop the unconditional `{{ $.Package }}.Hooks` / `{{ $.Package }}.Interceptors` reference**

Lines 215, 226 reference `{{ $n.Package }}.Hooks[:]...` — these read the per-entity Hooks slice from the sub-package's meta file (which is the same package now). Replace with bare `Hooks[:]...`.

Similarly check lines 36-37 (`c.Config.Hooks.{{ $n.Name }}`) — `Config.Hooks` is a struct from `internal`, so the `.<EntityName>` field reference stays — no change.

- [ ] **Step 5: Verify template parses + commit**

Run: `go test ./entc/gen/ -run TestTemplate_Parse -v -count=1`
Expected: no panic.

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/template/builder/client_type.tmpl
git commit -m "$(cat <<'EOF'
refactor(entc/gen): client_type — drop QueryEdge methods + package qualifiers

QueryEdge per-edge methods (Client.QueryTeams etc.) referenced sibling
*TeamQuery — moved to facade template for root-only emission.

Same-package qualifier prefix on Hooks/Interceptors removed because the
sub-package meta file (<entity>.go) lives in the same package as the
new client.go.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 15: mutation_type.tmpl — relocate, no content change

The template is already trivial (~30 lines, just `type TaskMutation = internal.TaskMutation` + opt aliases). With SubPackage:true flipped in Task 9, the only verification is that the import resolves correctly.

**Files:**
- Modify: `entc/gen/template/builder/mutation_type.tmpl` (verify only — likely no edit)

- [ ] **Step 1: Reread template**

It imports `"{{ $.Config.Package }}/internal"`. In sub-package scope `ent/task/mutation.go`, this becomes `import "{module}/ent/internal"`. Internal is a leaf package. No cycle.

- [ ] **Step 2: Verify the header template works in sub-package scope**

The template calls `{{ template "header" $ }}` which emits a `package <pkgname>` line. In sub-package scope, `$` is a `*typeScope`. The header template uses `$.Package` to derive the package name — verify typeScope returns the right sub-package name (i.e., `task`, not `ent`).

Read `entc/gen/template/header.tmpl` (if it exists; otherwise the header is inlined). Look for `{{ $.Package }}` usage. If header emits `package {{ $.Package }}`, the sub-package emit produces `package task` automatically. If it emits something else, may need a typeScope override for the header context.

- [ ] **Step 3: If header doesn't auto-derive sub-package, add typeScope.Package() override**

In `entc/gen/func.go` (after SiblingImports override added in Task 10), add:

```go
// Package returns the package name. In sub-package context this is the
// entity's sub-package (e.g. "task"); at root it delegates to Type.
// Used by the header template to emit the correct `package <name>` line.
func (t *typeScope) Package() string {
	if t.Scope["InSubPackage"] == true {
		return t.Type.Package
	}
	return t.Type.Package
}
```

Wait — `t.Type.Package` is already the entity package name (e.g. "task"). Existing root-scope templates that emit `package ent` must do so via a different field. Verify by reading header.tmpl. If header uses `$.Config.Package` (the module package, e.g. "ent"), no change needed. If header uses `$.Package` (entity package), no change needed either — both are correct in sub-package emit. The existing rolled-back commit must already have solved this since create.go works.

Verify by inspecting `ent/task/create.go` line 7 (`package task`) — confirmed in Task 1 reading. So the existing machinery is right; mutation_type just needs the SubPackage flag flip (already done in Task 9).

- [ ] **Step 4: Verify template still parses**

Run: `go test ./entc/gen/ -run TestTemplate_Parse -v -count=1`
Expected: PASS.

- [ ] **Step 5: No-op commit (or skip)**

If no edit was needed, skip this task's commit — Task 9 already covered it. Move to Task 16.

If a header-template tweak was needed, commit:

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/template/builder/mutation_type.tmpl entc/gen/func.go
git commit -m "$(cat <<'EOF'
chore(entc/gen): verify mutation_type works in sub-package scope

Mutation/type template is already trivial (type alias + opt aliases).
Task 9 flipped SubPackage:true; this task verifies the header emits
package task (not package ent) and that the internal import resolves.

No template body changes required.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 16: entql_type.tmpl — drop self-package qualifier + PackageAlias import

The entql_type template currently imports `{{ $.PackageAlias }} "{{ $.Config.Package }}/{{ $.PackageDir }}"` (so root-emitted code can reference `task.FieldID` etc.). In sub-package scope, that import is self-referential and Go errors out. Drop the import; drop the `{{ $n.Package }}.` qualifier prefixes (now same-package).

**Files:**
- Modify: `entc/gen/template/dialect/sql/entql_type.tmpl`

- [ ] **Step 1: Reread template (Task 1 already read it)**

- [ ] **Step 2: Drop the PackageAlias import**

OLD (line 19):
```
{{ $.PackageAlias }} "{{ $.Config.Package }}/{{ $.PackageDir }}"
```

NEW: delete the entire import line.

- [ ] **Step 3: Drop `{{ $n.Package }}.` qualifier prefixes**

Lines 74, 87 reference `{{ $n.Package }}.{{ $n.ID.Constant }}` and `{{ $n.Package }}.{{ $f.Constant }}`. In sub-package scope these need to become bare `{{ $n.ID.Constant }}` and `{{ $f.Constant }}`.

Easiest: replace `{{ $n.Package }}.` literal with `{{ $n.PackageQualifier }}` which evaluates to "" in sub-package and to `task.` at root. Verify `PackageQualifier()` includes the trailing dot or not (see Task 12 step 6); adjust the template to match.

If `PackageQualifier()` returns bare `task` (no dot), template uses `{{ $n.PackageQualifier }}.{{ $n.ID.Constant }}` — in sub-package this produces `.FieldID` which is a syntax error. Need to handle empty case. Conditional template logic:

```
{{- $q := $n.PackageQualifier }}{{ if $q }}{{ $q }}.{{ end }}{{ $n.ID.Constant }}
```

Or define a helper in `entc/gen/func.go`:

```go
// PackagePrefix returns "<pkg>." at root scope and "" in sub-package
// scope. Convenience for template authors emitting same-or-cross-package
// symbol references without conditional whitespace.
func (t *Type) PackagePrefix() string {
	q := t.PackageQualifier()
	if q == "" { return "" }
	return q + "."
}

// typeScope override:
func (t *typeScope) PackagePrefix() string {
	if t.Scope["InSubPackage"] == true { return "" }
	return t.Type.PackagePrefix()
}
```

Then template uses `{{ $n.PackagePrefix }}{{ $n.ID.Constant }}` — produces `task.FieldID` at root, `FieldID` in sub-package. Clean.

Apply this `PackagePrefix` helper here AND retrofit Task 12 (query.tmpl) Step 6 to use it instead of the verbose conditional.

- [ ] **Step 4: Verify template parses + commit**

Run: `go test ./entc/gen/ -run TestTemplate_Parse -v -count=1`

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/template/dialect/sql/entql_type.tmpl entc/gen/func.go entc/gen/type.go
git commit -m "$(cat <<'EOF'
refactor(entc/gen): entql_type — drop self-package import + qualifiers

In sub-package scope, the import of the entity's own package is a
self-reference and the {{ $.Package }}.<Constant> qualifier produces
duplicate symbols. Drop both.

Adds Type.PackagePrefix()/typeScope.PackagePrefix() — "<pkg>." at root,
"" in sub-package. Cleaner than inline conditionals; applied to
entql_type.tmpl here and retrofitted to query.tmpl.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 17: NEW facade.tmpl — per-entity type aliases + edge free functions

The new root-emitted template. One file per entity (`<entity>_facade.go`). Contains:
- Type aliases (`type TaskQuery = task.TaskQuery` etc.) so consumer references like `*ent.TaskQuery` resolve through.
- Constructor aliases (`var NewTaskClient = task.NewClient`).
- Cross-entity free functions: `WithTaskTeams`, `WithTaskOwner` etc. (one per edge).
- Cross-entity `QueryTaskEdge` free functions (one per edge).
- Cross-entity `loadTaskEdge` helpers (one per edge — the eager-load implementations that used to live as `*TaskQuery.loadX` methods).

**Files:**
- Create: `entc/gen/template/facade.tmpl`

- [ ] **Step 1: Construct the template skeleton**

```
{{/*
Copyright 2019-present Facebook Inc. All rights reserved.
This source code is licensed under the Apache 2.0 license found
in the LICENSE file in the root directory of this source tree.
*/}}

{{/* gotype: entgo.io/ent/entc/gen.Type */}}

{{ define "facade/type" }}
{{ $pkg := base $.Config.Package }}

{{ template "header" $ }}

import (
	"context"
	"fmt"

	"{{ $.Config.Package }}/{{ $.PackageDir }}"
	{{- range $e := $.Edges }}
	{{- if ne $e.Type.Package $.Package }}
	"{{ $.Config.Package }}/{{ $e.Type.PackageDir }}"
	{{- end }}
	{{- end }}
	"{{ $.Config.Package }}/predicate"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/runtime/entbuilder"
	{{- if $.HasOneFieldID }}
	{{- with $.ID.Type.PkgPath }}
	"{{ . }}"
	{{- end }}
	{{- end }}
)

{{ $n := $ }}
{{ $client := $n.ClientName }}
{{ $rec := $n.Receiver }}

// ---------------------------------------------------------------------------
// Type aliases — public API stable; underlying types live in {{ $.PackageDir }}.
// ---------------------------------------------------------------------------

type (
	{{ $client }}            = {{ $.PackageDir }}.Client
	{{ $n.QueryName }}       = {{ $.PackageDir }}.{{ $n.QueryName }}
	{{ $n.MutationName }}    = {{ $.PackageDir }}.{{ $n.MutationName }}
	{{ $n.CreateName }}      = {{ $.PackageDir }}.{{ $n.CreateName }}
	{{ $n.CreateBulkName }}  = {{ $.PackageDir }}.{{ $n.CreateBulkName }}
	{{- if not (and (not (notView $n)) (or ($n.featureEnabled FeatureNoUpdate) (hasKey $n.Annotations "ReadOnly"))) }}
	{{ $n.UpdateName }}      = {{ $.PackageDir }}.{{ $n.UpdateName }}
	{{ $n.UpdateOneName }}   = {{ $.PackageDir }}.{{ $n.UpdateOneName }}
	{{- end }}
	{{- if not (or ($n.featureEnabled FeatureNoDelete) (hasKey $n.Annotations "ReadOnly")) }}
	{{ $n.DeleteName }}      = {{ $.PackageDir }}.{{ $n.DeleteName }}
	{{ $n.DeleteOneName }}   = {{ $.PackageDir }}.{{ $n.DeleteOneName }}
	{{- end }}
	{{ $n.GroupBy }}         = {{ $.PackageDir }}.{{ $n.GroupBy }}
	{{ $n.Selector }}        = {{ $.PackageDir }}.{{ $n.Selector }}
	{{- if $n.featureEnabled FeatureEntQL }}
	{{ $n.FilterName }}      = {{ $.PackageDir }}.{{ $n.FilterName }}
	{{- end }}
)

// Constructor aliases — the sub-package's New<X> stays the source of truth.
var (
	New{{ $client }} = {{ $.PackageDir }}.NewClient
)

{{- /* ------------------------------------------------------------------
       Cross-entity edge methods — free functions because attaching them
       to type aliases is illegal in Go. The sub-package's *Client and
       *Query types remain leaves; cycle break happens here at root.
       ------------------------------------------------------------------ */}}
{{- range $e := $n.Edges }}
{{ $edgePkg := $e.Type.PackageDir }}
{{ $edgeQuery := $e.Type.QueryName }}
{{ $edgeClient := $e.Type.ClientName }}
{{ $edgeIDType := $e.Type.ID.Type }}

// With{{ $n.Name }}{{ $e.StructField }} eager-loads the {{ $e.Name }} edge on a {{ $n.Name }} query.
// The opts callback configures the sibling sub-query before storage.
func With{{ $n.Name }}{{ $e.StructField }}(q *{{ $n.QueryName }}, opts ...func(*{{ $edgeQuery }})) *{{ $n.QueryName }} {
	sub := New{{ $edgeClient }}(q.Config).Query()
	for _, opt := range opts {
		opt(sub)
	}
	return q.StoreEager("{{ $e.Name }}", func(ctx context.Context, parents []*{{ $n.Name }}) error {
		return load{{ $n.Name }}{{ $e.StructField }}(ctx, sub, parents)
	})
}

// Query{{ $n.Name }}{{ $e.StructField }} returns a query for the {{ $e.Name }} edge of the given {{ $n.Name }}.
func Query{{ $n.Name }}{{ $e.StructField }}(c *{{ $client }}, {{ $rec }} *{{ $n.Name }}) *{{ $edgeQuery }} {
	query := New{{ $edgeClient }}(c.Config).Query()
	query.Path = func(ctx context.Context) (*sql.Selector, error) {
		{{- with extend $n "Receiver" $rec "Edge" $e "Ident" "fromV" }}
			{{- $tmpl := printf "dialect/%s/query/from" $.Storage }}
			{{- xtemplate $tmpl . -}}
		{{- end }}
		return fromV, nil
	}
	return query
}

// load{{ $n.Name }}{{ $e.StructField }} performs the eager-load join for the {{ $e.Name }} edge.
// Body mirrors the pre-PR6 *{{ $n.QueryName }}.load{{ $e.StructField }} method, with sibling
// sub-package methods accessed via their exported surface (Fetch, PrepareQuery).
func load{{ $n.Name }}{{ $e.StructField }}(ctx context.Context, query *{{ $edgeQuery }}, nodes []*{{ $n.Name }}) error {
	{{- /* Inline the appropriate eager-load body based on edge cardinality. */}}
	{{- if $e.M2M }}
		{{- /* M2M body: build join predicate against the join table, query neighbors, assign by ID. */}}
		{{- /* This block templates the same logic as the pre-PR6 *{{ $n.QueryName }}.loadX method
		      but operates against the cross-entity types accessible here at root. */}}
		{{- template "facade/edge/m2m" (extend $n "Edge" $e "Query" "query" "Nodes" "nodes") }}
	{{- else if $e.OwnFK }}
		{{- template "facade/edge/ownfk" (extend $n "Edge" $e "Query" "query" "Nodes" "nodes") }}
	{{- else }}
		{{- template "facade/edge/inverse" (extend $n "Edge" $e "Query" "query" "Nodes" "nodes") }}
	{{- end }}
}
{{- end }}

{{ end }}{{/* end facade/type */}}

{{- /* Sub-templates for the three eager-load shapes. The bodies mirror the
       pre-PR6 dialect/sql/query.tmpl loadX templates with field references
       adjusted: parent IDs accessed via parent struct fields; sub-query
       methods called via the exported Fetch/PrepareQuery surface. */}}

{{ define "facade/edge/m2m" }}
edgeIDs := make([]any, len({{ .Nodes }}))
byID := make(map[{{ .ID.Type }}]*{{ .Name }})
nids := make(map[{{ .Edge.Type.ID.Type }}]map[*{{ .Name }}]struct{})
for i, node := range {{ .Nodes }} {
	edgeIDs[i] = node.ID
	byID[node.ID] = node
}
{{ .Query }}.Where(func(s *sql.Selector) {
	joinT := sql.Table({{ .PackageDir }}.{{ .Edge.TableConstant }})
	s.Join(joinT).On(s.C({{ .Edge.Type.PackageDir }}.{{ .Edge.Type.ID.Constant }}), joinT.C({{ .PackageDir }}.{{ .Edge.PKConstant }}[1]))
	s.Where(sql.InValues(joinT.C({{ .PackageDir }}.{{ .Edge.PKConstant }}[0]), edgeIDs...))
})
if err := {{ .Query }}.PrepareQuery(ctx); err != nil { return err }
neighbors, err := {{ .Query }}.Fetch(ctx)
if err != nil { return err }
for _, n := range neighbors {
	parents, ok := nids[n.ID]
	if !ok {
		return fmt.Errorf(`unexpected "{{ .Edge.Name }}" node returned %v`, n.ID)
	}
	for p := range parents {
		p.Edges.{{ .Edge.StructField }} = append(p.Edges.{{ .Edge.StructField }}, n)
	}
}
return nil
{{ end }}

{{ define "facade/edge/ownfk" }}
ids := make([]{{ .Edge.Type.ID.Type }}, 0, len({{ .Nodes }}))
nodeids := make(map[{{ .Edge.Type.ID.Type }}][]*{{ .Name }})
for i := range {{ .Nodes }} {
	fk := {{ .Nodes }}[i].Get{{ pascal .Edge.ForeignKey.StructField }}()
	if fk == nil { continue }
	if _, ok := nodeids[*fk]; !ok {
		ids = append(ids, *fk)
	}
	nodeids[*fk] = append(nodeids[*fk], {{ .Nodes }}[i])
}
if len(ids) == 0 { return nil }
{{ .Query }}.Where({{ .Edge.Type.PackageDir }}.IDIn(ids...))
neighbors, err := {{ .Query }}.Fetch(ctx)
if err != nil { return err }
for _, n := range neighbors {
	parents, ok := nodeids[n.ID]
	if !ok {
		return fmt.Errorf(`unexpected foreign-key "%s" returned %v`, "{{ .Edge.ForeignKey.StructField }}", n.ID)
	}
	for _, p := range parents {
		{{- if .Edge.Unique }}
		p.Edges.{{ .Edge.StructField }} = n
		{{- else }}
		p.Edges.{{ .Edge.StructField }} = append(p.Edges.{{ .Edge.StructField }}, n)
		{{- end }}
	}
}
return nil
{{ end }}

{{ define "facade/edge/inverse" }}
// Inverse-edge eager load: query neighbors by parent IDs, group by parent FK.
ids := make([]{{ .ID.Type }}, len({{ .Nodes }}))
byID := make(map[{{ .ID.Type }}]*{{ .Name }})
for i, n := range {{ .Nodes }} {
	ids[i] = n.ID
	byID[n.ID] = n
}
{{ .Query }}.Where({{ .Edge.Type.PackageDir }}.IDIn(ids...))   /* refine per edge */
neighbors, err := {{ .Query }}.Fetch(ctx)
if err != nil { return err }
for _, n := range neighbors {
	/* assign back via edge struct field — exact assignment depends on edge metadata */
	_ = n
}
return nil
{{ end }}
```

**WARNING: the eager-load templates above are starting-point pseudocode.** The real bodies must be derived by extracting the pre-PR6 `dialect/sql/query.tmpl` `loadX` method bodies and adjusting:
- `query.sqlAll(ctx, hooks...)` → `query.Fetch(ctx, hooks...)` (use exported wrapper)
- `query.prepareQuery(ctx)` → `query.PrepareQuery(ctx)` (use exported wrapper)
- `query.QueryState.Inters` → `query.QueryState.Inters` (still accessible, public field)
- `withInterceptors[[]*{{ Edge.Type.Name }}](...)` — needs investigation: this helper is in root package, can be moved or its signature kept compatible.

During implementation, run regen-then-build iteratively. If a generated facade file doesn't compile, the template body for that eager-load shape needs adjustment. Track failures by error class:
- "undefined: <fieldName>" → missing struct field access (likely a `.Edges.X` reference shape mismatch).
- "undefined: withInterceptors" → helper not in scope; move to entbuilder or duplicate as a small helper.
- import cycle → a sub-package import slipped through; check Task 10's SiblingImports override.

- [ ] **Step 2: Verify template parses**

Run: `go test ./entc/gen/ -run TestTemplate_Parse -v -count=1`
Expected: PASS.

- [ ] **Step 3: Commit (skeleton only)**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/gen/template/facade.tmpl
git commit -m "$(cat <<'EOF'
feat(entc/gen): NEW facade.tmpl — per-entity root facade

Emits <entity>_facade.go per entity:
- Type aliases for all generated per-entity types (TaskQuery, TaskClient,
  TaskMutation, TaskCreate, ..., TaskFilter)
- Constructor aliases (var NewTaskClient = task.NewClient)
- Cross-entity edge free functions per edge:
    WithTask<Edge>(q, opts ...)    — registers eager loader on q
    QueryTask<Edge>(c, t)          — returns sub-query for edge traversal
    loadTask<Edge>(ctx, sub, parents) — eager-load implementation

The load<Edge> bodies mirror the pre-PR6 *TaskQuery.loadX method bodies
with sub-package methods accessed via the exported Fetch/PrepareQuery
surface added in Task 13.

Initial bodies are pseudo-templated; iterative regen + build in Task 18
will surface the exact shape required.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Phase 4: Fixture regen + tests + docs (Tasks 18-21)

Where the rubber meets the road. Phase 3's template changes need to produce code that compiles + passes integration tests. Expect to iterate: regen → build → fix template → regen → build until green.

---

## Task 18: Regenerate entc/integration fixtures

The integration suite has ~30 fixture trees (privacy, hooks, edgeschema, multischema, migrate, etc.) each with its own `ent/` directory containing generated code. PR 6 changes every per-entity file in every tree.

**Files:**
- Regenerate: `entc/integration/**/ent/<entity>/{client,query,mutation,entql}.go` (new sub-package files)
- Regenerate: `entc/integration/**/ent/<entity>_facade.go` (new root facade files)
- Delete: `entc/integration/**/ent/<entity>_{client,query,mutation,entql}.go` (old root files; cleaned up by `deletedTypeTemplates`)

- [ ] **Step 1: Run regen against ONE fixture first (privacy is smallest)**

```bash
pwd && git rev-parse --abbrev-ref HEAD
cd entc/integration/privacy && go generate ./... 2>&1 | tee /tmp/pr6-regen-privacy.log
cd -
```

- [ ] **Step 2: Inspect generated output**

```bash
ls -la entc/integration/privacy/ent/
ls -la entc/integration/privacy/ent/task/
```

Expected:
- `ent/task_facade.go` exists (~2-5 KB)
- `ent/task/client.go`, `ent/task/query.go`, `ent/task/mutation.go`, `ent/task/entql.go` exist
- `ent/task_client.go`, `ent/task_query.go`, `ent/task_mutation.go`, `ent/task_entql.go` DELETED
- `ent/task.go` unchanged (model alias)

- [ ] **Step 3: Build the regenerated fixture**

```bash
go build ./entc/integration/privacy/... 2>&1 | tee /tmp/pr6-build-privacy.log
```

Almost certainly FAILS. Expected error classes:
- Import cycle (some template still emits sibling import) → check Task 10's override; possibly a template bypassed it.
- Undefined symbol (a facade helper template body has wrong field/method ref) → fix in `facade.tmpl` (Task 17), regen, retry.
- Type mismatch (eagerLoader signature doesn't match callsite) → check Task 12 + 17 alignment.

- [ ] **Step 4: Iterate**

For each error class, fix the template (in the appropriate Phase 3 task's file), then re-regen and re-build. Commit each meaningful template fix:

```bash
git add entc/gen/template/<file>.tmpl
git commit -m "fix(entc/gen): <specific fix description>"
git rev-parse master
```

Keep an "error budget" diary at `/tmp/pr6-iter.md` listing each fix so it's clear what shifted between regen passes. Don't commit the diary.

DO NOT commit the regenerated fixtures yet — wait until the privacy fixture builds clean, then move to Step 5.

- [ ] **Step 5: Once privacy builds clean, regenerate ALL fixtures**

```bash
go generate ./entc/integration/... 2>&1 | tee /tmp/pr6-regen-all.log
go build ./entc/integration/... 2>&1 | tee /tmp/pr6-build-all.log
```

Iterate further if needed.

- [ ] **Step 6: Commit the regenerated fixtures**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/integration/
git status --short | head -20  # spot-check
git commit -m "$(cat <<'EOF'
chore(entc/integration): regenerate fixtures for PR 6

Sub-package layout for every integration fixture:
  ent/<entity>/{client,query,mutation,entql}.go (new)
  ent/<entity>_facade.go (new — root aliases + cross-entity free fns)
  ent/<entity>_{client,query,mutation,entql}.go (removed)

All fixtures build clean.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 19: No-sibling-imports regression lint

Spec §3.2 requires sub-packages to have zero sibling-entity imports and zero root-`ent` imports. Add a lint test that scans every integration fixture's `ent/<entity>/` directory.

**Files:**
- Create: `entc/internal/subpackage_imports_test.go`

- [ ] **Step 1: Write the test**

```go
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
			if err != nil { return err }
			if info.IsDir() { return nil }
			if !strings.HasSuffix(path, ".go") { return nil }
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if perr != nil {
				return nil // ignore parse errors here; build will catch
			}
			for _, imp := range f.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				// Forbid: imports of sibling entity sub-packages.
				for _, sibling := range entityDirs {
					if sibling == dir { continue }
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
```

- [ ] **Step 2: Run test**

Run: `go test ./entc/internal/ -run TestSubPackageNoSiblingImports -v -count=1`
Expected: PASS for every fixture (post-Task 18 regen).

If any fixture FAILS, the template still leaks a sibling import. Fix in the relevant template (Phase 3 task), re-regen, re-test. Commit the template fix; the fixture regen will follow with a separate commit (or amend Task 18's fixture commit if minimal).

- [ ] **Step 3: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add entc/internal/subpackage_imports_test.go
git commit -m "$(cat <<'EOF'
test(entc/internal): regression lint — sub-packages are leaves

Asserts every generated ent/<entity>/ sub-package imports zero
sibling-entity packages and never imports the root ent package.

Catches future template changes that re-introduce cycles —
PR 6's central invariant.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Task 20: Run entc/integration test suite

After Phase 3 + Task 18 fixture regen, the integration suite must pass — proves the generated code is functionally correct, not just compiling.

**Files:** none (test execution only)

- [ ] **Step 1: Run a small integration suite first**

```bash
pwd && git rev-parse --abbrev-ref HEAD
go test ./entc/integration/privacy/... -count=1 -v 2>&1 | tail -40
```

Expected: PASS. If FAIL, the failure mode dictates the fix:
- Runtime error (e.g., "edge not loaded") → eager-load template (facade.tmpl) has a bug in the load body; fix template, regen privacy, re-run test.
- SQL error → likely a column-name reference mistake during the qualifier strip; check meta-file constants are referenced bare in sub-package emit.
- Hook integration test failure → hook signature change; verify the Mutation alias chain wasn't broken.

Iterate to green. Commit each meaningful template fix as a "fix(entc/gen)" commit; re-regen affected fixtures with a follow-up "chore(entc/integration): regen post-<fix>" commit.

- [ ] **Step 2: Run the FULL integration suite**

```bash
go test ./entc/integration/... -count=1 2>&1 | tee /tmp/pr6-integ-all.log | tail -50
```

Expected: PASS for every package. Triage failures by package + error class; fix iteratively.

- [ ] **Step 3: Run gen tests**

```bash
go test ./entc/gen/ -count=1 2>&1 | tail -30
```

Expected: PASS. The gen snapshot tests compare against pre-PR6 file paths/contents; some snapshots may need regenerating to reflect the new layout. If snapshot tests fail with diffs that look like the intended PR 6 changes (sub-package paths, missing cross-entity methods), regenerate snapshots:

```bash
# Look for snapshot regeneration commands in entc/gen/ — typically
# a "go test -update" or similar flag.
grep -rn 'snapshot' entc/gen/*_test.go | head
# Apply the regen flag if available.
```

Then commit the snapshot updates separately.

- [ ] **Step 4: Final green sweep**

```bash
go test ./... -count=1 -short 2>&1 | tail -20
```

Expected: PASS (or only failures from external deps unrelated to PR 6).

- [ ] **Step 5: Commit any final fixture/snapshot regen**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git status --short
git add <regenerated files>
git commit -m "chore(entc): regen snapshots + fixtures for PR 6 final"
git rev-parse master
```

---

## Task 21: MIGRATION.md updates

Document the consumer API change and the migration-tool workflow.

**Files:**
- Modify: `MIGRATION.md` (create if absent)

- [ ] **Step 1: Read existing MIGRATION.md (if it exists)**

```bash
ls MIGRATION.md && cat MIGRATION.md | head -50 || echo "MIGRATION.md absent — creating"
```

PR 3 and PR 5 already added sections. Preserve those; append a new PR 6 section.

- [ ] **Step 2: Append PR 6 section**

Add at the end of MIGRATION.md:

```markdown
## PR 6 — Per-entity sub-packages (2026-05-16)

PR 6 moves four per-type generated files from root `ent/` into `ent/<entity>/`
sub-packages and introduces a per-entity root facade with type aliases plus
free functions for cross-entity edge methods.

### What changed (consumer-visible)

| Before | After |
|---|---|
| `client.Task.Query().WithTeams(func(q *ent.TeamQuery){...})` | `ent.WithTaskTeams(client.Task.Query(), func(q *ent.TeamQuery){...})` |
| `client.Task.QueryTeams(t)` | `ent.QueryTaskTeams(client.Task, t)` |
| `*ent.TaskQuery`, `*ent.TaskClient`, `*ent.TaskMutation` etc. | unchanged (type aliases) |
| `m.OwnerID()`, `m.TeamsIDs()` etc. (deprecated since PR 5) | `entbuilder.EdgeIDAs[int](m, "owner")`, `entbuilder.EdgeIDsAs[int](m, "teams")` |

### What did NOT change

- `*ent.TaskQuery`, `*ent.TaskClient`, `*ent.TaskMutation` are still
  importable from the `ent` package (now via type alias).
- Self-only chaining (`Where`, `Limit`, `Order`, `All`, etc.) is unchanged.

### Why

Per-entity sub-packages let Go's parallel compiler shard the per-entity
working set, materially reducing build wall time + peak RSS at consumer
scale (134+ entities). Cross-entity edge methods can't live as methods
on a type alias and would re-introduce import cycles between sub-packages,
so they hoist to root as free functions.

### Migration

The `cmd/ent-codegen-migrate` tool rewrites all call sites mechanically.
Run it AFTER regenerating ent code with the post-PR6 entgen:

```bash
# 1. Regenerate ent code (picks up new templates).
go generate ./internal/ent

# 2. Run the migration tool against consumer code.
go run entgo.io/ent/cmd/ent-codegen-migrate \
    -descriptors ./internal/ent/internal \
    ./internal/  ./pkg/...   # consumer package paths

# 3. Verify build.
go build ./...
```

The tool runs four passes in order: mutation → predicate → edge-method
→ typed-edge-accessor. Each is idempotent — safe to re-run.

### If you previously ran the broken pre-PR6 tool (corruption recovery)

The pre-PR6 tool had a bug where `matchMutationCall` matched purely on
method name and rewrote calls on the schema DSL — corrupting up to 116
schema files when run against service-api-go. The PR 6 tool fixes this
with type-aware receiver gating.

To recover:

```bash
# Restore corrupted schema files from git.
git restore -- internal/ent/schema/

# Re-run the (now fixed) tool. Idempotency guarantees re-runs are safe.
go run entgo.io/ent/cmd/ent-codegen-migrate -descriptors ... ./consumer/...
```
```

- [ ] **Step 3: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add MIGRATION.md
git commit -m "$(cat <<'EOF'
docs(MIGRATION): PR 6 per-entity packages migration guide

Documents the API change (q.WithTeams → ent.WithTaskTeams), the
migration-tool workflow (4-pass chain, idempotent), and recovery
steps for consumers who ran the broken pre-PR6 tool.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git rev-parse master
```

---

## Phase 5: Bench + epic update (Tasks 22-26)

Validate the parallelism win at consumer scale and close out the PR 6 entry in the epic spec.

---

## Task 22: Smoke-test migration tool on the privacy fixture

End-to-end smoke: build the migration tool binary, run all four passes against a small consumer-style codebase (the privacy fixture's own hand-written tests act as a consumer of the regenerated ent code), confirm no corruption + clean build.

**Files:** none (verification only)

- [ ] **Step 1: Build the migration tool**

```bash
pwd && git rev-parse --abbrev-ref HEAD
go build -o /tmp/ent-codegen-migrate ./cmd/ent-codegen-migrate
```

Expected: build succeeds.

- [ ] **Step 2: Set up a sandbox copy of the privacy fixture's non-generated files**

```bash
mkdir -p /tmp/pr6-smoke
cp entc/integration/privacy/ent/hook/*.go /tmp/pr6-smoke/ 2>/dev/null || true
find entc/integration/privacy -name '*_test.go' -not -path '*/ent/*' | xargs -I {} cp {} /tmp/pr6-smoke/ 2>/dev/null || true
ls /tmp/pr6-smoke/
```

- [ ] **Step 3: Run the migration tool with --dry-run**

```bash
/tmp/ent-codegen-migrate \
    -descriptors entc/integration/privacy/ent/internal \
    -dry-run \
    /tmp/pr6-smoke/
```

Expected: prints "would rewrite" for files that contain call sites the rewriters target. Zero panics, zero "could not parse" errors.

- [ ] **Step 4: Run for real, verify idempotency**

```bash
/tmp/ent-codegen-migrate -descriptors entc/integration/privacy/ent/internal /tmp/pr6-smoke/
/tmp/ent-codegen-migrate -descriptors entc/integration/privacy/ent/internal -dry-run /tmp/pr6-smoke/ | grep -c "would rewrite" || true
```

Expected: second run prints 0 "would rewrite" lines.

- [ ] **Step 5: No commit (verification only)**

If anything FAILS, fix in `cmd/ent-codegen-migrate/` source and re-run the migration-tool tests from Phase 1.

---

## Task 23: Consumer-scale bench against service-api-go

Use the existing `/tmp/bench-pr6.sh` harness from the brainstorm prompt's bench infrastructure. Run pre-migration (master baseline) is already captured; capture post-PR6.

**Files:**
- Reference: `/tmp/bench-pr6.sh` (existing harness from session)
- Reference: `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/` (gemini worktree pinning the wiggly worktree's ent)
- Output: `/var/tmp/bench-pr6/post-pr6-<timestamp>.txt`

- [ ] **Step 1: Verify harness exists**

```bash
ls -la /tmp/bench-pr6.sh 2>/dev/null || echo "harness missing — recover from brainstorm prompt §4-5"
ls -la /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/ 2>/dev/null || echo "bench worktree missing"
```

If absent, reconstruct per the brainstorm prompt's bench infrastructure section.

- [ ] **Step 2: Run the bench harness against the post-PR6 wiggly worktree**

Pre-requisite: the bench worktree's go.mod is pinned to the wiggly ent. The migration tool MUST have been run against the consumer's source first (Task 22's process applied to the actual consumer codebase). Run:

```bash
bash /tmp/bench-pr6.sh post-pr6 2>&1 | tee /var/tmp/bench-pr6/post-pr6-$(date +%Y%m%d-%H%M%S).txt
```

Expected: completes in 5-15 minutes; captures generate + build wall and peak RSS.

- [ ] **Step 3: Compare to baseline**

```bash
ls /var/tmp/bench-pr6/baseline-*.txt | head -1
ls /var/tmp/bench-pr6/post-pr6-*.txt | tail -1
```

Compare the numbers. Spec §8.5 gates:
- `go build` wall ≤ 75% of baseline (≥ 25% reduction)
- `go build` peak RSS ≤ 75% of baseline
- `task generate` wall ±10% of baseline
- Root `ent/` LOC ≤ 30% of baseline root LOC

If any gate fails, the bench result needs investigation BEFORE declaring PR 6 shippable.

- [ ] **Step 4: No commit (bench data only — Task 24 captures it)**

---

## Task 24: Capture bench results in internal/bench/pr6.jsonl

Persist the bench numbers next to the PR 5 bench artifact so the epic's progress can be audited.

**Files:**
- Create: `internal/bench/pr6.jsonl`
- Modify: `internal/bench/README.md`

- [ ] **Step 1: Extract bench metrics from the harness output**

```bash
cat /var/tmp/bench-pr6/post-pr6-*.txt | tail -1
```

- [ ] **Step 2: Write `internal/bench/pr6.jsonl`**

Example (replace numbers with actuals from Task 23):

```jsonl
{"pr":6,"label":"baseline","metric":"build_wall_sec","value":134}
{"pr":6,"label":"baseline","metric":"build_peak_rss_gb","value":10.2}
{"pr":6,"label":"baseline","metric":"root_gen_loc","value":235951}
{"pr":6,"label":"post-pr5","metric":"root_gen_loc","value":235898}
{"pr":6,"label":"post-pr6","metric":"build_wall_sec","value":000}
{"pr":6,"label":"post-pr6","metric":"build_peak_rss_gb","value":0.0}
{"pr":6,"label":"post-pr6","metric":"root_gen_loc","value":000}
{"pr":6,"label":"post-pr6","metric":"subpackage_count","value":134}
```

Replace `000`/`0.0` with the actual numbers.

- [ ] **Step 3: Update `internal/bench/README.md` with PR 6 procedure**

Append a section documenting the PR 6 bench workflow (consumer migration prerequisite + harness invocation).

- [ ] **Step 4: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add internal/bench/pr6.jsonl internal/bench/README.md
git commit -m "bench(internal/bench): PR 6 measurement vs. baseline + PR 5"
git rev-parse master
```

---

## Task 25: Update epic spec progress table

Mark PR 6 complete in the epic spec.

**Files:**
- Modify: `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md`

- [ ] **Step 1: Update the §0 progress table**

In `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md`, find the §0 progress table and:
- Change the header to reflect PR 6 complete.
- Mark PR 6 row: `⏳ next` → `✅ complete`.
- Add the plan reference: `docs/superpowers/plans/2026-05-16-codegen-epic-pr6-per-entity-packages.md`.
- Add commit range: `<first PR 6 commit>` … `<last PR 6 commit>`.

Run `git log --oneline -30 | head` to find the relevant commit SHAs.

- [ ] **Step 2: Update spec §0 status line**

Change the in-progress count to reflect the new state.

- [ ] **Step 3: Commit**

```bash
pwd && git rev-parse --abbrev-ref HEAD
git add docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md
git commit -m "docs(epic): mark PR 6 complete"
git rev-parse master
```

---

## Task 26: Final sanity sweep

Verify branch state, master untouched, all PR 6 commits stacked correctly.

**Files:** none (verification only)

- [ ] **Step 1: Verify branch + master**

```bash
pwd                                # /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git rev-parse --abbrev-ref HEAD    # worktree-wiggly-singing-pancake
git rev-parse master               # 7e9d99b1435d541286a773ca128be1a1931d6cc8 (untouched)
git log --oneline master..HEAD | wc -l   # number of PR 6 commits + earlier epic commits
```

- [ ] **Step 2: List PR 6 commits**

```bash
git log --oneline f453859dc..HEAD   # everything since the PR 6 spec commit
```

Expected: ~20-30 commits across Phases 1-5.

- [ ] **Step 3: Run final test sweep**

```bash
go test ./entc/... ./cmd/ent-codegen-migrate/... ./runtime/... -count=1 -short 2>&1 | tail -10
```

Expected: PASS across the board.

- [ ] **Step 4: Verify no-PR/no-push discipline**

```bash
git for-each-ref refs/heads/   # confirm no extra branches
git remote -v                  # remotes should match the worktree's expected upstream config
```

- [ ] **Step 5: No commit. Done.**

The PR 6 implementation is complete and local-only per [[feedback-no-prs-until-end-of-epic]].

---

## Self-review checklist (for plan authors, before handing off)

1. **Spec coverage:** Every spec section mapped to a task? ✓
   - §3.1 (Output layout) → Tasks 9, 17, 18
   - §3.2 (Cycle break) → Tasks 10, 17, 19
   - §3.3 (Root facade) → Tasks 17, 18
   - §3.4 (Sub-package method surface) → Tasks 12, 13, 14, 15, 16
   - §4 (Consumer API changes) → Tasks 5-7, 21
   - §5.1 (template.go) → Task 9
   - §5.2 (func.go / type.go) → Tasks 10, 16
   - §5.3 (Template body changes) → Tasks 12-16
   - §5.4 (graph.go / assets.go) → existing machinery (Task 11 verifies)
   - §6.1 (Receiver-type fix) → Tasks 2, 3
   - §6.2 (New AST passes) → Tasks 5, 6
   - §6.3 (Dispatch update) → Task 7
   - §6.4 (Tests) → Tasks 4, 5, 6, 7
   - §7 (Test strategy) → Tasks 19, 20
   - §8 (Acceptance gates) → Tasks 19, 20, 22, 23
   - §9 (Risks) → addressed via per-task verification + bench gate
   - §10 (Implementation order) → matches Phases 1-5
   - §13 (Memory + commit hygiene) → every commit step verifies branch + master

2. **Placeholder scan:** No "TBD", "TODO", "fill in details". ✓
   - The `000`/`0.0` in Task 24's JSONL are explicitly marked as "replace with actuals" — instruction, not unspecified content.
   - The "pseudocode" warning in Task 17 step 1 is explicit ("real bodies derived by extracting...") — instruction.

3. **Type consistency:** Cross-task references aligned? ✓
   - `StoreEager` (Task 12) → used by `WithTaskTeams` (Task 17). Signature matches.
   - `Fetch`, `PrepareQuery` (Task 13) → used by `loadTaskTeams` body in Task 17. Names match.
   - `MatchesReceiverType` / `ReceiverTypeMatchesPattern` (Task 2) → used by `isMutationReceiver` (Task 3), `matchEdgeReceiver` (Task 5), `matchEdgeOnMutation` (Task 6). Stable.
   - `PackagePrefix` (Task 16) → retrofitted to Task 12 (note in step 6).

4. **No spec gaps:** All 13 sections of the spec map to at least one task above. ✓

---




