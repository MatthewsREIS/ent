# Codegen Reduction Lever B-1 — Facade Edge Bodies → `gen/edges/` Subpackage

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `entc/gen/template/facade.tmpl` so the per-edge `load<Node><Edge>` / `loadNamed<Node><Edge>` / `With<Node><Edge>` / `WithNamed<Node><Edge>` / `Query<Node><Edge>` / `Query<Node><Edge>FromQuery` bodies emit into a new `gen/edges/<entity>.go` file, while `*_facade.go` shrinks to type aliases + `var X = edges.X` forwarders. Measured target on consumer bench: BUILD cold wall −5% to −10%, peak RSS −5%.

**Architecture:** New `edges/type` template in `entc/gen/template.go` Templates slice with `Format: "edges/<entity>.go"` — mirrors the existing `internal/model` pattern (flat directory, single package). New `typeScope` flag `InEdgesPackage` toggled by graph.go so the template prefixes current-entity refs with `<package>.` while edge-target refs continue qualifying via the existing `$e.Type.Package` mechanism. Root `facade.tmpl` replaces every edge function body with `var WithUserCreatedBy = edges.WithUserCreatedBy` style forwarders — works because root `UserQuery` is a type alias of `user.UserQuery`, so signatures match exactly.

**Tech Stack:** Go 1.25, ent codegen (`text/template`), existing testify+`go test ./entc/...` for in-repo validation, bench worktree at `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go` for consumer-scale measurement.

**Per [[feedback-no-prs-until-end-of-epic]]:** all commits stay local on `worktree-wiggly-singing-pancake`. No `git push`, no `gs submit`, no PRs at any step.

**Per [[feedback-bench-build-memory-limits]]:** every bench-worktree `go build` / `go run entc.go` invocation gets `GOMEMLIMIT=8GiB GOGC=25`. No `GOMAXPROCS` cap on the 32-core host for fair-comparison wall times.

---

## File Structure

**Templates (entc/gen/template/):**
- Modify: `facade.tmpl` — strip per-edge function bodies; keep aliases + new `var` forwarders
- Create: `edges.tmpl` — new template emitting `package edges` with full per-edge function bodies

**Generator wiring (entc/gen/):**
- Modify: `template.go` — append `edges/type` entry to `Templates` slice (Format `"edges/%s.go"`, no `SubPackage: true`)
- Modify: `func.go` — extend the `InSubPackage` scope-check pattern with a parallel `InEdgesPackage` branch in `PackageQualifier` and the import helpers
- Modify: `graph.go` — when executing the `edges/type` template, wrap the type in `&typeScope{Type: n, Scope: map[any]any{"InEdgesPackage": true}}`

**Tests (entc/gen/):**
- Modify: `template_test.go` — add `edges/type` expectation to `TestTemplates_PR6SubPackageFormat` (or sibling test if SubPackage-only)
- Modify: `func_test.go` — add `TestTypeScope_QualifiesInEdgesPackage` (parallel to existing `TestTypeScope_SiblingImportsInSubPackage`)

**Regenerated in-repo fixtures (entc/integration/*/ent/edges/):**
- New files: `edges/*.go` per entity in each integration fixture (hooks, edgeschema, privacy, etc.)
- Modified: `*_facade.go` per entity — shrunk to aliases + forwarders

**Bench artifacts:**
- New: `/var/tmp/bench-pr6/post-b1-<timestamp>.txt` — measurement output
- Modified: `project_codegen_reduction_post_bench_plan.md` (memory) — append B-1 measurement outcome

---

## Phase A — Template-mode infrastructure (ent core)

### Task 1: Add `InEdgesPackage` scope flag with unit tests

**Files:**
- Test: `entc/gen/func_test.go`
- Modify: `entc/gen/func.go:269-294`

- [ ] **Step 1: Write the failing test**

Append to `entc/gen/func_test.go`:

```go
func TestTypeScope_QualifiesInEdgesPackage(t *testing.T) {
	// In the new gen/edges/ package, the current entity must be qualified
	// by its sub-package name (e.g., *user.UserQuery, not *UserQuery).
	ty := &Type{
		Name:    "User",
		Package: "user",
	}
	scope := &typeScope{Type: ty, Scope: map[any]any{"InEdgesPackage": true}}
	require.Equal(t, "user.", scope.PackageQualifier(), "InEdgesPackage must prefix current-entity refs with <package>.")
}

func TestTypeScope_NoQualifierWhenNoEdgesPackage(t *testing.T) {
	ty := &Type{
		Name:    "User",
		Package: "user",
	}
	scope := &typeScope{Type: ty, Scope: map[any]any{}}
	require.Equal(t, "", scope.PackageQualifier(), "default (no scope flag) returns empty qualifier")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./entc/gen/ -run TestTypeScope_QualifiesInEdgesPackage -v`
Expected: FAIL — `PackageQualifier()` currently returns `""` when `InEdgesPackage` is set (no handling).

- [ ] **Step 3: Implement the InEdgesPackage branch in PackageQualifier**

Edit `entc/gen/func.go` around line 269. Existing code:

```go
// PackageQualifier returns the package qualifier. When InSubPackage is set in
// the typeScope's Scope, it returns "" — the sub-package's templates
// reference their own types directly without qualification.
func (t typeScope) PackageQualifier() string {
	if t.Scope["InSubPackage"] == true {
		return ""
	}
	return ""
}
```

Replace with:

```go
// PackageQualifier returns the package qualifier. When InSubPackage is set,
// returns "" (sub-package templates reference their own types directly). When
// InEdgesPackage is set, returns "<package>." so edges/<entity>.go (which
// lives in gen/edges/, not gen/<entity>/) qualifies references to the
// current entity's types through the entity sub-package.
func (t typeScope) PackageQualifier() string {
	if t.Scope["InSubPackage"] == true {
		return ""
	}
	if t.Scope["InEdgesPackage"] == true {
		return t.Type.Package + "."
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./entc/gen/ -run 'TestTypeScope_(QualifiesInEdgesPackage|NoQualifierWhenNoEdgesPackage)' -v`
Expected: PASS for both.

- [ ] **Step 5: Commit**

```bash
git add entc/gen/func.go entc/gen/func_test.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entc/gen/func): add InEdgesPackage scope qualifier

The new gen/edges/<entity>.go template lives in package "edges" — neither
root gen nor the per-entity sub-package — so it must qualify current-entity
type references with <package>. (e.g., *user.UserQuery, not *UserQuery).
Mirrors the existing InSubPackage scope flag's PackageQualifier branch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Register `edges/type` template

**Files:**
- Test: `entc/gen/template_test.go`
- Modify: `entc/gen/template.go:48-172` (Templates slice)

- [ ] **Step 1: Write the failing test**

Append to `entc/gen/template_test.go`:

```go
func TestTemplates_EdgesTypeRegistered(t *testing.T) {
	ty := &Type{Name: "Task"}
	want := "edges/task.go"
	var got string
	for _, tmpl := range Templates {
		if tmpl.Name == "edges/type" {
			got = tmpl.Format(ty)
			break
		}
	}
	require.Equal(t, want, got, "edges/type template must be registered with format edges/<package>.go")
}

func TestTemplates_EdgesTypeIsNotSubPackage(t *testing.T) {
	// edges/type goes to a single shared edges/ dir, not per-entity sub-package.
	// Should NOT have SubPackage: true (that's reserved for <entity>/file.go layouts).
	for _, tmpl := range Templates {
		if tmpl.Name == "edges/type" {
			require.False(t, tmpl.SubPackage, "edges/type lives in a shared dir, must NOT be SubPackage")
			return
		}
	}
	t.Fatal("edges/type template not registered")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./entc/gen/ -run 'TestTemplates_EdgesType' -v`
Expected: FAIL — `edges/type` not yet registered, so `got == ""` and the second test hits `t.Fatal`.

- [ ] **Step 3: Register the new template**

Edit `entc/gen/template.go`. Find the Templates slice (line 49), append the new entry **before** the closing `}` of the Templates slice (after the `facade/type` entry around line 171):

```go
		{
			// PR-7 lever B-1: per-edge load<Node><Edge>, loadNamed<Node><Edge>,
			// With<Node><Edge>, WithNamed<Node><Edge>, Query<Node><Edge>, and
			// Query<Node><Edge>FromQuery bodies emit into a single shared edges/
			// package. The root facade keeps `var X = edges.X` one-line forwarders
			// for the public function names. Flat dir layout (not SubPackage),
			// mirrors internal/model and internal/mutation.
			Name: "edges/type",
			Cond: func(t *Type) bool { return len(t.Edges) > 0 },
			Format: func(t *Type) string {
				return fmt.Sprintf("edges/%s.go", t.PackageDir())
			},
		},
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./entc/gen/ -run 'TestTemplates_EdgesType' -v`
Expected: PASS for both.

- [ ] **Step 5: Commit**

```bash
git add entc/gen/template.go entc/gen/template_test.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entc/gen): register edges/type template

Adds the registration for the per-entity edges/<entity>.go file that will
hold the load/With/Query function bodies hoisted out of facade.tmpl. Flat
directory layout (single edges package, not per-entity sub-package) —
mirrors internal/model. Cond gates on len(t.Edges) > 0 so edge-free
entities don't emit empty files.

Template body lands in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Wire graph.go to apply InEdgesPackage scope when executing edges template

**Files:**
- Modify: `entc/gen/graph.go:276-277` (where SubPackage scope is applied)

- [ ] **Step 1: Read the current graph.go SubPackage dispatch**

Run: `grep -n "SubPackage\|InSubPackage" entc/gen/graph.go`

Expected output (around line 276):
```
276:		if tmpl.SubPackage {
277:			data = &typeScope{Type: n, Scope: map[any]any{"InSubPackage": true}}
278:		}
```

- [ ] **Step 2: Modify the dispatch to also handle edges/type**

Edit `entc/gen/graph.go` around line 276. Current code:

```go
if tmpl.SubPackage {
    data = &typeScope{Type: n, Scope: map[any]any{"InSubPackage": true}}
}
```

Replace with:

```go
if tmpl.SubPackage {
    data = &typeScope{Type: n, Scope: map[any]any{"InSubPackage": true}}
} else if tmpl.Name == "edges/type" {
    data = &typeScope{Type: n, Scope: map[any]any{"InEdgesPackage": true}}
}
```

- [ ] **Step 3: Run the ent gen tests to confirm no regression**

Run: `go test ./entc/gen/ -count=1`
Expected: PASS — no template body exists yet, so the new branch is dead code, but the wiring compiles.

- [ ] **Step 4: Commit**

```bash
git add entc/gen/graph.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entc/gen/graph): apply InEdgesPackage scope for edges/type template

Mirrors the existing SubPackage scope-wrap dispatch. Without this, the
upcoming edges.tmpl would render with default (root) qualification and
produce broken code. Branch is dead until Task 4 lands the template body.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase B — Move template bodies

### Task 4: Create edges.tmpl with header + imports + load<Edge> bodies

**Files:**
- Create: `entc/gen/template/edges.tmpl`
- Modify: `entc/gen/template/facade.tmpl:185-385` (delete load<Edge> block — moved to edges.tmpl)

- [ ] **Step 1: Create the new edges.tmpl skeleton**

Create `entc/gen/template/edges.tmpl`:

```
{{/*
Copyright 2019-present Facebook Inc. All rights reserved.
This source code is licensed under the Apache 2.0 license found
in the LICENSE file in the root directory of this source tree.
*/}}

{{/* gotype: entgo.io/ent/entc/gen.typeScope */}}

{{ define "edges/type" }}
{{ with extend $.Type "Package" "edges" }}
	{{ template "header" . }}
{{ end }}

{{ $n := $.Type }}
{{ $pkg := $n.Package }}

import (
	"context"
	"database/sql/driver"
	"fmt"

	{{ $n.PackageAlias }} "{{ $n.Config.Package }}/{{ $n.PackageDir }}"
	"{{ $n.Config.Package }}/predicate"
	{{- $imported := dict $n.PackageDir true }}
	{{- range $e := $n.Edges }}
	{{- if not (hasKey $imported $e.Type.PackageDir) }}
	{{ $e.Type.PackageAlias }} "{{ $n.Config.Package }}/{{ $e.Type.PackageDir }}"
	{{- $imported = set $imported $e.Type.PackageDir true }}
	{{- end }}
	{{- end }}

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
)

{{- range $e := $n.Edges }}
{{/* PASTE per-edge load + loadNamed + With + WithNamed + Query + QueryFromQuery bodies HERE in Task 4-6 */}}
{{- end }}

{{ end }}
```

- [ ] **Step 2: Copy the load<Node><Edge> + loadNamed<Node><Edge> body blocks from facade.tmpl**

The two function bodies currently live at `entc/gen/template/facade.tmpl:185-385` (load) and `:387-553` (loadNamed). Copy both block contents (the `{{ $lfunc := ... }} ... // {{ $lfunc }} ... func {{ $lfunc }}(...)  { ... }` blocks) into the `{{- range $e := $n.Edges }}` section of edges.tmpl, replacing the `{{/* PASTE ... */}}` placeholder.

**Important type-qualification fix-ups when pasting** (because edges.tmpl runs with `InEdgesPackage` scope, NOT `InSubPackage`):
- `*{{ $n.Name }}` → `*{{ $n.Package }}.{{ $n.Name }}` (current-entity refs must qualify)
- `*{{ $e.Type.Name }}` → `*{{ $e.Type.Package }}.{{ $e.Type.Name }}` (edge-target refs already qualify via `$e.Type.Package`)
- `predicate.{{ $e.Type.Name }}` stays — predicate package is a sibling import already in scope
- `{{ $pkg }}.` references inside the existing template (line 204, 210, 211 in facade.tmpl) stay — `$pkg = $n.Package` from line 17 in facade is now `$pkg = $n.Package` from the `$pkg := $n.Package` line in edges.tmpl. Identical.
- `byID := make(map[{{ $n.ID.Type }}]*{{ $n.Name }})` → `byID := make(map[{{ $n.ID.Type }}]*{{ $n.Package }}.{{ $n.Name }})`
- `nodes[i].Edges.{{ $e.StructField }}` → `nodes[i].Edges.{{ $e.StructField }}` (Edges field stays — accessing struct field through a pointer, package qualifier is on the type not the access)
- `nodes[i].{{ $fk.StructField }}` and `nodes[i].Get{{ pascal $fk.StructField }}()` likewise stay — field/method access doesn't get the package qualifier.

Audit by reading the full block side-by-side with facade.tmpl while you paste — every `*Name` type reference, every `make([]*Name, ...)`, every `map[K]*Name` needs the package prefix.

- [ ] **Step 3: Delete the load<Node><Edge> and loadNamed<Node><Edge> blocks from facade.tmpl**

Edit `entc/gen/template/facade.tmpl`. Delete:
- Lines 185-385 (the `{{ $lfunc := print "load" ... }} ... func {{ $lfunc }}(...) error { ... }` block)
- Lines 387-553 (the `{{- if and ($.FeatureEnabled "namedges") (not $e.Unique) }} ... {{- end }}` block containing `loadNamed`)

What remains in the `{{- range $e := $n.Edges }}` loop in facade.tmpl: the `WithUserCreatedBy`, `WithNamed`, `Query`, `QueryFromQuery` function definitions (lines 99-183 in the original). Task 6 turns those into `var` forwarders; for now they stay as full bodies so facade.tmpl still compiles standalone.

- [ ] **Step 4: Run in-repo regen + tests**

Run from ent root:
```bash
cd entc/integration && GOMEMLIMIT=4GiB GOGC=25 go generate ./... 2>&1 | tail -30
```

Expected: regen succeeds, new `edges/*.go` files appear under each `entc/integration/*/ent/edges/`.

```bash
go test ./entc/...
```

Expected: PASS (the regenerated code still has all functions — they're now duplicated between root facade.tmpl and edges.tmpl, but that's fine for one commit; Task 6 deduplicates).

If regen fails with template errors, fix paste mistakes (most likely: missing `$n.Package` prefix on a type reference) and re-run.

- [ ] **Step 5: Commit**

```bash
git add entc/gen/template/edges.tmpl entc/gen/template/facade.tmpl entc/integration/
```

```bash
git commit -m "$(cat <<'EOF'
feat(entc/gen/template): emit load<Edge>/loadNamed<Edge> into edges.tmpl

Moves the two heaviest per-edge function bodies (load and loadNamed,
which contain the M2M/O2M/M2O eager-load dispatch) from facade.tmpl into
a new edges.tmpl that emits to gen/edges/<entity>.go. The edges package
qualifies current-entity refs via the InEdgesPackage scope flag added in
Task 1.

Root facade.tmpl keeps the With/Query/QueryFromQuery defs at full body
for now — Task 6 converts those to var forwarders. Net code is duplicated
between facade.tmpl and edges.tmpl in this commit; deduplication lands
in Task 6 so each commit produces working generated code.

In-repo entc/integration fixtures regenerated.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Move With/WithNamed/Query/QueryFromQuery bodies into edges.tmpl

**Files:**
- Modify: `entc/gen/template/edges.tmpl`
- Modify: `entc/gen/template/facade.tmpl:99-183` (delete the With/Query function bodies — moved)

- [ ] **Step 1: Copy the four function bodies from facade.tmpl into edges.tmpl**

Source blocks in `facade.tmpl`:
- `WithUserCreatedBy` (around lines 102-114)
- `WithNamed<Node><Edge>` (around lines 116-133, gated by `featureEnabled "namedges"`)
- `Query<Node><Edge>` (around lines 135-150)
- `Query<Node><Edge>FromQuery` (around lines 152-183)

Paste into edges.tmpl inside the `{{- range $e := $n.Edges }}` loop, **after** the load/loadNamed blocks pasted in Task 4. Same qualification fix-ups as Task 4:
- `*{{ $n.QueryName }}` → stays — `$n.QueryName` already resolves to `UserQuery`. But signature must reference `*{{ $n.Package }}.{{ $n.QueryName }}`. Audit per block.
- `New{{ $edgeClient }}(q.Config)` → `{{ $e.Type.Package }}.New{{ $edgeClient }}(q.Config)` (NewUserClient lives in user pkg, not root)
- The `func {{ $func }}(q *{{ $n.QueryName }}, opts ...func(*{{ $edgeQuery }}))` signature → `func {{ $func }}(q *{{ $n.Package }}.{{ $n.QueryName }}, opts ...func(*{{ $e.Type.Package }}.{{ $edgeQuery }}))`
- `*{{ $edgeQuery }}` return types similarly qualify with `$e.Type.Package`.
- `{{ $n.Package }}.{{ $n.ID.Constant }}` and similar table-constant refs already use the package prefix in facade.tmpl — no change needed.

- [ ] **Step 2: Delete the function bodies from facade.tmpl**

Delete `facade.tmpl` lines 99-183 (the `{{- range $e := $n.Edges }} ... {{- end }}` loop's With/Query/QueryFromQuery content). The outer `{{- range $e := $n.Edges }}` loop and its closing `{{- end }}` stay — Task 6 fills the loop body with `var` forwarders.

For now, leave the loop body **empty** between the range start and end. The fixture regen in Step 3 will produce a facade with edge functions removed entirely, breaking calls to `gen.WithUserCreatedBy(...)`. That's expected and Task 6 fixes it.

- [ ] **Step 3: Run regen — expect compile errors in fixtures**

Run from ent root:
```bash
cd entc/integration && GOMEMLIMIT=4GiB GOGC=25 go generate ./... 2>&1 | tail -10
```

Expected: regen succeeds (template is valid Go template).

```bash
go vet ./entc/integration/... 2>&1 | tail -30
```

Expected: vet errors like `undefined: WithUserPosts` in fixture test code. This is intentional — the edge functions now live in `gen/edges/` but root has no forwarder yet. Task 6 fixes this.

- [ ] **Step 4: Commit (compile-broken intermediate)**

```bash
git add entc/gen/template/edges.tmpl entc/gen/template/facade.tmpl entc/integration/
```

```bash
git commit -m "$(cat <<'EOF'
refactor(entc/gen/template): move With/Query edge funcs to edges.tmpl

Edge function bodies (With, WithNamed, Query, QueryFromQuery) now emit
into gen/edges/<entity>.go. Root facade.tmpl's edge loop is empty after
this commit — consumer call sites to gen.WithUserPosts etc. will fail to
compile until Task 6 lands the var forwarders.

Intentional intermediate state — splits cleanly along template-content
boundaries so each commit has one focused concern. In-repo fixtures
regenerated; their vet errors are expected pending Task 6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Add `var X = edges.X` forwarders to facade.tmpl

**Files:**
- Modify: `entc/gen/template/facade.tmpl` (the empty edge loop from Task 5)

- [ ] **Step 1: Add the edges package import to facade.tmpl**

Edit `entc/gen/template/facade.tmpl`. In the import block (around line 19-43), inside the `{{- if $n.Edges }}` import section, add:

```
"{{ $.Config.Package }}/edges"
```

So the import block looks like:
```
import (
	{{- if $n.Edges }}
	{{ $.PackageAlias }} "{{ $.Config.Package }}/{{ $.PackageDir }}"
	"{{ $.Config.Package }}/edges"
	{{- /* per-edge type imports below... */ -}}
	{{- end }}
	...
)
```

Verify the import is added inside the `{{- if $n.Edges }}` so it doesn't appear for edge-free entities (where the edges package wouldn't import them either).

- [ ] **Step 2: Add var forwarders to the empty edge loop**

Find the `{{- range $e := $n.Edges }} ... {{- end }}` loop that's now empty. Replace with:

```
{{- range $e := $n.Edges }}
{{ $func := print "With" $n.Name $e.StructField }}
{{ $qfunc := print "Query" $n.Name $e.StructField }}
{{ $qqfunc := print "Query" $n.Name $e.StructField "FromQuery" }}

// {{ $func }} forwards to edges.{{ $func }} — body hoisted to gen/edges/{{ $n.Package }}.go for build parallelism.
var {{ $func }} = edges.{{ $func }}

{{- if and ($.FeatureEnabled "namedges") (not $e.Unique) }}
{{ $nfunc := print "WithNamed" $n.Name $e.StructField }}
// {{ $nfunc }} forwards to edges.{{ $nfunc }}.
var {{ $nfunc }} = edges.{{ $nfunc }}
{{- end }}

// {{ $qfunc }} forwards to edges.{{ $qfunc }}.
var {{ $qfunc }} = edges.{{ $qfunc }}

// {{ $qqfunc }} forwards to edges.{{ $qqfunc }}.
var {{ $qqfunc }} = edges.{{ $qqfunc }}
{{- end }}
```

- [ ] **Step 3: Run regen + tests**

```bash
cd entc/integration && GOMEMLIMIT=4GiB GOGC=25 go generate ./... 2>&1 | tail -10
```

Expected: regen succeeds.

```bash
go test ./entc/...
```

Expected: PASS — fixtures compile again, consumer call sites resolve via the var forwarders. If a fixture test fails, debug the most likely cause: signature mismatch between the edges/<entity>.go function and the root forwarder. Both must have identical types — root's `UserQuery` is a type alias of `user.UserQuery`, so they should match. If not, the edges.tmpl signature was over-qualified — recheck Task 5 paste.

- [ ] **Step 4: Spot-check a regenerated facade file**

```bash
wc -l entc/integration/edgeschema/ent/user_facade.go entc/integration/edgeschema/ent/edges/user.go
```

Expected: user_facade.go is now small (tens to ~150 lines, depending on edges + aliases); edges/user.go contains the moved bulk (hundreds of lines per entity, scaling with edge count).

- [ ] **Step 5: Commit**

```bash
git add entc/gen/template/facade.tmpl entc/integration/
```

```bash
git commit -m "$(cat <<'EOF'
feat(entc/gen/template): facade.tmpl edge funcs become var forwarders

Replaces the per-edge function-body emissions in facade.tmpl with
`var WithUserPosts = edges.WithUserPosts`-style forwarders. Works because
root's UserQuery is a type alias of user.UserQuery — function-value
signatures match exactly across the boundary.

Net facade.tmpl output per entity drops from O(edges * 100 lines) to
O(edges * 4 lines). Bulk lives in gen/edges/<entity>.go (added in
Tasks 4-5). Root gen package shrinks accordingly; consumer call sites
to gen.WithUserPosts(...) etc. continue to resolve.

In-repo entc/integration fixtures regenerated and tests pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase C — Validate against ent test suite

### Task 7: Run full ent test suite

**Files:** none (validation only)

- [ ] **Step 1: Run all ent unit + integration tests**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
go test ./entc/... ./runtime/... 2>&1 | tail -40
```

Expected: all pass. If integration tests fail with a compile error in a fixture, the most common cause is the load function body referencing a method that's only on the root type alias and not on the underlying entity-pkg type. Fix in edges.tmpl, re-regen, retest.

- [ ] **Step 2: Verify no leftover duplication**

```bash
grep -c "func loadUserPosts" entc/integration/edgeschema/ent/user_facade.go entc/integration/edgeschema/ent/edges/user.go
```

Expected: `user_facade.go: 0`, `edges/user.go: 1`. (Adjust filenames if edgeschema's User has different edges.)

```bash
grep -c "WithUserPosts" entc/integration/edgeschema/ent/user_facade.go
```

Expected: `>= 1` — at least one `var WithUserPosts = edges.WithUserPosts` line.

- [ ] **Step 3: Commit if any test fixture file changed during this validation**

Most likely no further commits — Task 6 already regen'd. If `go test` triggered any auto-regeneration:

```bash
git status
```

If clean, skip. Otherwise:

```bash
git add entc/integration/
```

```bash
git commit -m "$(cat <<'EOF'
chore(entc/integration): regenerate fixtures after lever B-1 template split

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase D — Bench measurement (consumer scale)

### Task 8: Snapshot bench worktree state and run B-1 bench

**Files:**
- Read: `/tmp/bench-pr6-post-uncapped.sh`
- New artifact: `/var/tmp/bench-pr6/post-b1-<timestamp>.txt`

**Bench worktree path:** `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql`

The bench script invokes `task generate-go-no-cache` → `task gqlgen-no-cache` → `task build-no-cache` → warm variants. The first step regenerates the gen tree using the ent worktree's entc (linked via `go.mod replace`). The migration tool runs as part of the consumer's regen pipeline. Both pick up the lever B-1 template changes automatically.

- [ ] **Step 1: Snapshot bench worktree state**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
git status
git rev-parse HEAD > /tmp/bench-pr6-pre-b1-sha.txt
```

The bench worktree is detached HEAD on gemini@main per [[project-codegen-reduction-bench-results]]. Capture SHA so the post-bench state can be diffed if needed. Per the spec §5 constraint: don't reset the bench worktree without explicit decision — only the GENERATE step mutates it during the bench run, and that's the point.

- [ ] **Step 2: Run the bench script**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
/tmp/bench-pr6-post-uncapped.sh post-b1
```

Expected runtime: ~12-15 min total (generate cold + warm + gqlgen cold + warm + build cold + warm; current post-PR-6 totals are 2:43 + 1:24 + 3:33 + 0:36 + 2:32 + 0:06 = ~11 min).

Output written to: `/var/tmp/bench-pr6/post-b1-<timestamp>.txt` and tee'd to stdout.

If the build cold step fails with a compile error in `./api-graphql/...`, the migration tool didn't fully migrate the new facade-forwarder layout. Inspect the error, decide whether to extend the migration tool (out of scope per spec) or hand-fix the bench-side templates that generate the broken code. Stop and report — don't try to patch on the fly.

- [ ] **Step 3: Diff B-1 measurements vs post-PR-6 baseline**

```bash
LATEST=$(ls -t /var/tmp/bench-pr6/post-b1-*.txt | head -1)
BASELINE=/var/tmp/bench-pr6/post-pr06-uncapped-20260518-040049.txt
echo "=== POST-B1 SUMMARY ===" && grep -E "Elapsed|Maximum resident" "$LATEST" | head -20
echo "=== POST-PR6 SUMMARY ===" && grep -E "Elapsed|Maximum resident" "$BASELINE" | head -20
```

Compute deltas manually or with jq if results emit JSON. Compare against §6 B-1 prediction:
- BUILD cold wall: −5% to −10% vs post-PR-6 (current 2:32 → target ≤2:24)
- BUILD cold peak RSS: −5% vs post-PR-6 (current 8.8 GB → target ≤8.4 GB)
- Other sections (GENERATE, GQLGEN): expected unchanged (B-1 doesn't touch them)

- [ ] **Step 4: Update bench-results memory with B-1 outcome**

Edit `/home/smoothbrain/.claude/projects/-var-home-smoothbrain-dev-matthewsreis-ent/memory/project_codegen_reduction_bench_results.md`. Append a "## Post-B-1 results (YYYY-MM-DD)" section with the actual measured wall + RSS deltas vs post-PR-6, citing the output file path.

Also update `/home/smoothbrain/.claude/projects/-var-home-smoothbrain-dev-matthewsreis-ent/memory/project_codegen_reduction_post_bench_plan.md` to mark B-1 as measured and reference the new bench result.

- [ ] **Step 5: Commit memory updates**

Memory files live outside the ent worktree (not in git), so no git commit needed for memory.

If the bench worktree gained changes from the regen run, do NOT commit them — they're ephemeral per [[project-codegen-reduction-pr6-consumer-migration-gap]]'s working-state policy. They'll persist for now and get refreshed by the next bench run.

---

## Phase E — Decision gate

### Task 9: Evaluate measurement vs §6 prediction

**Files:** none (analytical step)

- [ ] **Step 1: Categorize the measured outcome**

| Outcome | Measured BUILD cold wall vs post-PR-6 | Action |
|---|---|---|
| **Hit** | −5% to −10% (or better) | Proceed to lever B-2 (entgql contrib pagination + node) |
| **Partial** | 0% to −5% | Investigate why parallelism didn't materialize before B-2; check action-graph for whether `gen/edges/` is now serial long-pole |
| **Miss** | 0% or worse | Stop. Re-investigate per spec §8 verification: was the gen root compile actually shrunk? Did edges/ compile become the new long-pole? Is the critical path now bottlenecked elsewhere? Update spec R1 hypothesis or pivot. |

- [ ] **Step 2: If HIT, write the B-2 plan**

Trigger writing-plans skill for B-2 (entgql contrib changes — pagination + node per-entity in the contrib worktree at `/var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg`). Use the existing `mutation_input_subpkg.tmpl` precedent for the wrapper-template pattern per the design §4.1.

- [ ] **Step 3: If PARTIAL or MISS, run a build action-graph profile on the bench**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
export GOCACHE="/tmp/profile-gocache-b1-$$" && mkdir -p "$GOCACHE"
GOMEMLIMIT=8GiB GOFLAGS=-buildvcs=false /usr/bin/time -v go build "-gcflags=github.com/MatthewsREIS/gemini/...=-N -l" -debug-actiongraph=/tmp/build-actiongraph-b1.json ./api-graphql/... 2>&1 | tee /tmp/build-profile-b1.log
rm -rf "$GOCACHE"
```

Then parse for the new long-pole:

```bash
jq -r '[.[] | select(.Mode=="build" and .TimeStart != null and .TimeDone != null and (.Package | test("MatthewsREIS"))) | {pkg: .Package, dur: (((.TimeDone[0:19] + "Z") | fromdateiso8601) - ((.TimeStart[0:19] + "Z") | fromdateiso8601))}] | sort_by(-.dur) | .[0:15] | .[] | "\(.dur)s\t\(.pkg)"' /tmp/build-actiongraph-b1.json
```

Expected after B-1 if it worked: `src/ent/gen` time should drop from 52s to ~35-40s (since 70K of 403K moved out). `src/ent/gen/edges` should appear as a new compile at ~10-20s. Total critical path should drop ~10-15s.

If `src/ent/gen/edges` is unexpectedly large (>30s), the edges.tmpl emitted too much, or qualifications generated lots of extra method imports — diagnose before continuing.

- [ ] **Step 4: Stop and report**

Whichever branch you took (HIT / PARTIAL / MISS), write a one-message summary to the user with:
- Measured deltas vs prediction
- Action-graph long-pole picture (if profiled)
- Recommended next step (B-2 vs investigate vs pivot)

End the plan here. Do not auto-launch B-2 — the user explicitly wants the bench gate per [[feedback-no-prs-until-end-of-epic]] philosophy of "improvements without numbers don't ship."

---

## Self-review notes

**Spec coverage:** §4.1 (sibling-subpackage architecture, facade load bodies → gen/edges/) covered by Tasks 1-7. §5 step 1 (B-1 staged + bench gate + decision branch) covered by Tasks 8-9. §6 B-1 prediction (−5% to −10% wall, −5% RSS) used as gate criterion in Task 9. §8 verification criteria invoked by name in Task 9 step 1.

**Out of scope verified absent:** No lever F work (entbuilder edge helpers) anywhere in this plan. No migration-tool extensions. No entgql contrib changes (that's B-2). No `git push` / `gs submit` invocations. No N (nibbles) work.

**Type-consistency check:** `TypeTemplate` field names (`Name`, `Cond`, `Format`, `SubPackage`) consistent across Tasks 2 and 3. `typeScope.Scope["InEdgesPackage"]` consistent across Tasks 1, 3. Template name `"edges/type"` consistent across Tasks 2, 3, 9.

**Open items the executing engineer should be aware of:**
- The Task 4 / Task 5 paste audit is the riskiest part of the plan — most likely failure mode is forgetting `$n.Package.` on one type reference, producing an `undefined: User` error during regen. Read every type reference in the pasted blocks before committing.
- The `var X = edges.X` pattern in Task 6 relies on root types being exact aliases of subpackage types. If a fixture entity has a method defined on the root type but not the sub-package type, the var forwarder breaks. Unlikely in modern PR-6 layout but flag if seen.
- The migration tool is NOT extended in this plan. If Task 8 step 2 fails because the bench consumer code can't compile after regen, that's a migration-tool gap — recovery is hand-fix in the bench (out of scope here) or extend the tool (out of scope per spec).
