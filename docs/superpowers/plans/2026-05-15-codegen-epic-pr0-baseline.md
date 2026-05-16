# Codegen Epic PR 0 — Baseline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a bench harness that records reproducible gen-time and build-time metrics for ent codegen, and stabilize the `entc/gen` regression test suite so the codegen-reduction epic starts from green.

**Architecture:**
- `cmd/bench-codegen/main.go` — small CLI binary that drives the bench
- `internal/bench/` — package containing the measurement logic (LOC counting, subprocess wrapping, fixture orchestration, JSONL output)
- Each fixture is copied to a tmpdir before measurement; in-repo state is never modified by the bench
- Subprocess-based measurement so each gen/build pass produces an isolated rusage reading via `ProcessState.SysUsage()`

**Tech Stack:** Go 1.23, stdlib only (`os/exec`, `syscall`, `encoding/json`, `time`), `github.com/stretchr/testify/require` for tests (already a dependency).

**Spec reference:** `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` §4 PR 0.

**Branch:** `codegen-epic/00-baseline` (git-spice managed; create the branch with `gs branch create codegen-epic/00-baseline` before starting Task 1).

---

## Task 1: Confirm the baseline failure set is exactly the four documented tests

This task is a paranoia check — if the failure set has drifted (extra failures appeared, or some of the four documented ones now pass on their own), the rest of the plan needs adjustment.

**Files:**
- Read: `entc/gen/regression_test.go`

- [ ] **Step 1: Run the regression tests, capture output**

Run: `go test -count=1 -run TestGraph_Gen ./entc/gen/... 2>&1 | tee /tmp/pr0-baseline-failures.txt`

Expected: 4 FAIL lines:
- `--- FAIL: TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport`
- `--- FAIL: TestGraph_Gen_AssignGeneratedUserIntIDAcceptsInt`
- `--- FAIL: TestGraph_Gen_SQLModifierDeleteBuilderHasModify`
- `--- FAIL: TestGraph_Gen_SQLSchemaConfigHooksInDescriptorPaths`

- [ ] **Step 2: Assert failure-set parity**

Run: `grep -c '^--- FAIL:' /tmp/pr0-baseline-failures.txt`

Expected: `4`

If the count is not 4, STOP and update this plan before continuing. Either the spec was wrong about the baseline, or template work has shifted since the spec was written.

- [ ] **Step 3: Read the four failing tests**

Read `entc/gen/regression_test.go` end to end. Each failing test starts with `func TestGraph_Gen_<Name>(t *testing.T)`. Note what the test generates and what it asserts — Tasks 2–5 will use this context.

- [ ] **Step 4: No commit — this task is read-only**

---

## Task 2: Fix `TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport`

The test exercises the gremlin storage `delete` template's alias handling. It forces `graph.Nodes[0].alias = "enttask"` and expects exactly one aliased import of `gremlinregen/ent/task` in the generated `task_delete.go`. The current failure is a Go parse error in the generated output (`expected operand, found '<'` — almost certainly a template emitting `<no value>` or `<nil>` where a real symbol should be).

**Files:**
- Read first: `entc/gen/regression_test.go:24-68`
- Investigate (template candidates): `entc/gen/template/dialect/gremlin/delete.tmpl`, `entc/gen/template/dialect/gremlin/*.tmpl`
- Fix: whichever template/Go file the investigation identifies

- [ ] **Step 1: Run the failing test in verbose mode and save the generated output for inspection**

Run: `go test -count=1 -v -run '^TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport$' ./entc/gen/... 2>&1 | tee /tmp/pr0-task2-failure.txt`

Read `/tmp/pr0-task2-failure.txt`. The failure message includes the generated file path inside `.gotmp/`. Copy the actual generated `task_delete.go` somewhere stable for inspection:

Run: `cp $(grep -o '/[^ :]*task_delete.go' /tmp/pr0-task2-failure.txt | head -1) /tmp/pr0-task2-task_delete.go`

Open `/tmp/pr0-task2-task_delete.go` and find the line that contains the `<` that the parser rejected. The line number is in the failure output (e.g. `task_delete.go:143:65: expected operand, found '<'`).

- [ ] **Step 2: Identify the template that emitted the bad line**

Find the template responsible:

Run: `grep -rn 'task_delete\|"delete"' entc/gen/template/dialect/gremlin/ | head -20`

Then read the most likely template file. The generated bad line will correspond to a template expression like `{{ .Foo }}` where `.Foo` evaluated to a sentinel. Often this is an alias field that wasn't propagated, e.g. `{{ .Package.Alias }}` resolving to the literal `<no value>` because `Alias` is empty.

- [ ] **Step 3: Decide template-fix vs. assertion-fix**

Decision rule:
- If the bad output is **clearly wrong Go** (parse error, undefined symbol, malformed expression) → **template-fix**: the template is broken, fix it to emit correct Go. This is the typical case here.
- If the bad output is **valid Go that just no longer matches the test's expectation** → **assertion-fix**: the template intentionally changed, update the test.

The current failure is a parse error → template-fix.

- [ ] **Step 4: Apply the template fix**

Edit the responsible template so the alias is propagated correctly. Typically this means adding `{{ with $.Package.Alias }}{{ . }}{{ else }}<package-name>{{ end }}` around an existing reference, or adding the alias to the relevant `imports` map.

If you cannot pinpoint the template without further evidence, use `git log -p --all -- entc/gen/template/dialect/gremlin/delete.tmpl entc/gen/template/dialect/gremlin/*.tmpl | head -200` to see recent changes; the last template touch likely introduced the regression. The matthewsreis fork's `c93835e32 Merge pull request #1 from MatthewsREIS/moar-files` and adjacent commits restructured templates — the gremlin delete file may have lost alias plumbing in that pass.

- [ ] **Step 5: Run the test and confirm it passes**

Run: `go test -count=1 -v -run '^TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport$' ./entc/gen/...`

Expected: `PASS`.

- [ ] **Step 6: Run the full regression suite to verify no collateral damage**

Run: `go test -count=1 -run TestGraph_Gen ./entc/gen/... 2>&1 | grep -E 'PASS|FAIL|---' | head -20`

Expected: the previously-three remaining failures still fail; no NEW failures.

- [ ] **Step 7: Commit**

```bash
git add entc/gen/template/dialect/gremlin/
git commit -m "fix(codegen): restore alias propagation in gremlin delete template

Regression test TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport
was failing with a parse error in generated task_delete.go because
the gremlin delete template stopped emitting the entity-package alias.
Restoring the alias-aware import preserves single-import-with-alias
semantics for aliased entities.
"
```

---

## Task 3: Fix `TestGraph_Gen_AssignGeneratedUserIntIDAcceptsInt`

The test generates a User entity with only an `int` ID field, writes an inline `_test.go` that references `userCreateDescriptor.ID.AssignGenerated(...)`, and runs `go test` against it. Current failure: `undefined: userCreateDescriptor`. The descriptor pattern was introduced by the COMPACT_HELPERS work (`runtime/entbuilder/`); either the descriptor isn't being generated for the int-ID-only case, or it was renamed.

**Files:**
- Read first: `entc/gen/regression_test.go:70-108`
- Investigate (descriptor templates): `entc/gen/template/dialect/sql/create.tmpl`, `runtime/entbuilder/create.go`
- Fix: template if descriptor missing; test if descriptor was renamed

- [ ] **Step 1: Run the failing test and capture the actual generated package**

Run: `go test -count=1 -v -run '^TestGraph_Gen_AssignGeneratedUserIntIDAcceptsInt$' ./entc/gen/... 2>&1 | tee /tmp/pr0-task3-failure.txt`

The failure output shows the build error: `undefined: userCreateDescriptor`. The test's tmpdir is logged. Find and inspect the generated `user_create.go`:

Run: `find /tmp -name 'user_create.go' -path '*useridregen*' 2>/dev/null | head -1 | xargs grep -n 'Descriptor\|^var ' | head -20`

- [ ] **Step 2: Determine whether the descriptor exists under a different name**

If `grep` finds a similarly-named symbol (e.g. `userDescriptor`, `userCreateBuilderDescriptor`, `createDescriptor`), the symbol was renamed. **Assertion-fix path** — update the test's inline source code to use the new symbol name (line 94 of regression_test.go).

If `grep` finds no descriptor variable at all, **template-fix path** — the descriptor isn't being emitted in the int-ID-only case. Continue to Step 3.

- [ ] **Step 3 (template-fix path only): Compare to a working case**

Generate a similar fixture with multiple fields (the working path), and grep for the descriptor:

Run: `grep -rn 'createDescriptor\b\|CreateDescriptor\[' entc/integration/edgeschema/ent/ | head -10`

Compare with the int-ID-only fixture — the difference identifies the missing branch in the template. Likely cause: a `{{ if .Fields }}` gate that suppresses descriptor emission when there are no non-ID fields.

Find the gating expression:

Run: `grep -n 'CreateDescriptor\|createDescriptor' entc/gen/template/dialect/sql/create.tmpl entc/gen/template/dialect/sql/*.tmpl`

Edit the template so the descriptor is emitted even when the entity has only an ID field.

- [ ] **Step 4 (assertion-fix path only): Update the test's inline source**

Edit `entc/gen/regression_test.go:94` to reference the new descriptor name. Update the test name comment and commit message accordingly.

- [ ] **Step 5: Confirm the test passes**

Run: `go test -count=1 -v -run '^TestGraph_Gen_AssignGeneratedUserIntIDAcceptsInt$' ./entc/gen/...`

Expected: `PASS`.

- [ ] **Step 6: Run the full regression suite**

Run: `go test -count=1 -run TestGraph_Gen ./entc/gen/...`

Expected: two failures remaining (Modifier and SchemaConfig); no new failures.

- [ ] **Step 7: Commit**

Use a commit message matching the path taken in Step 2:
- Template-fix: `fix(codegen): emit create descriptor for ID-only entities`
- Assertion-fix: `test(entc/gen): align AssignGenerated regression with renamed create descriptor`

```bash
git add entc/gen/
git commit -m "<message from above>"
```

---

## Task 4: Fix `TestGraph_Gen_SQLModifierDeleteBuilderHasModify`

The test enables `FeatureModifier` and expects the generated `TaskDelete` type to have a `Modify(...)` method that mutates an internal `modifiers` slice. Current failure: `_d.modifiers undefined (type *TaskDelete has no field or method modifiers)`. The descriptor refactor likely removed the `modifiers` field while leaving the `Modify` method that references it (or the reverse: removed the method that initializes the field).

**Files:**
- Read first: `entc/gen/regression_test.go:110-148`
- Investigate (modifier feature templates): `entc/gen/template/dialect/sql/delete.tmpl`, `entc/gen/template/dialect/sql/modify_*.tmpl`, `entc/gen/feature.go` (FeatureModifier)
- Fix: whichever template lost the symbol pair

- [ ] **Step 1: Run the failing test and locate the generated `task_delete.go`**

Run: `go test -count=1 -v -run '^TestGraph_Gen_SQLModifierDeleteBuilderHasModify$' ./entc/gen/... 2>&1 | tee /tmp/pr0-task4-failure.txt`

Find the generated delete file:

Run: `find /tmp -path '*deletemodifierregen*' -name 'task_delete.go' 2>/dev/null | head -1 | xargs grep -n 'modifiers\|Modify\b\|func.*TaskDelete' | head -40`

- [ ] **Step 2: Identify the mismatch**

You will see one of:
- `Modify` method exists but no `modifiers` field on the struct → the field declaration template was elided. Restore the field.
- `modifiers` field exists but `Modify` method does not → the method template was elided. Restore the method.
- Neither exists → the entire FeatureModifier delete-builder integration regressed. Restore both.

- [ ] **Step 3: Find the responsible template(s)**

Run: `grep -rn 'modifiers\b\|FeatureModifier\|"modify"' entc/gen/template/dialect/sql/ entc/gen/feature.go | head -30`

The `delete.tmpl` likely has a section like `{{ if $.Features.HasFeature "sql/modifier" }}` that gates the field/method emission. The descriptor refactor (see `runtime/entbuilder/delete.go`) may have moved the storage off the struct without restoring it for the modifier path.

- [ ] **Step 4: Apply the fix**

Restore the missing symbol(s) in the appropriate template under the existing `FeatureModifier` gate. Reference the equivalent code in the Update path (FeatureModifier almost certainly applies symmetrically there) for the exact field type and method signature.

If unclear, examine a working integration fixture that uses FeatureModifier:

Run: `grep -rln 'FeatureModifier' entc/integration/ | head -5`

If a fixture uses it, its generated `*_delete.go` shows the correct shape.

- [ ] **Step 5: Confirm the test passes**

Run: `go test -count=1 -v -run '^TestGraph_Gen_SQLModifierDeleteBuilderHasModify$' ./entc/gen/...`

Expected: `PASS`.

- [ ] **Step 6: Run the full regression suite**

Run: `go test -count=1 -run TestGraph_Gen ./entc/gen/...`

Expected: one failure remaining (SchemaConfig); no new failures.

- [ ] **Step 7: Commit**

```bash
git add entc/gen/template/dialect/sql/
git commit -m "fix(codegen): restore Modify/modifiers pair on FeatureModifier delete builder

The descriptor-based delete refactor dropped <field|method> from the
generated *<Entity>Delete type, so consumers using FeatureModifier hit
a build error. Restored the missing declaration under the existing
FeatureModifier gate so Modify(...) compiles again."
```

---

## Task 5: Fix `TestGraph_Gen_SQLSchemaConfigHooksInDescriptorPaths`

The test enables `FeatureSchemaConfig` and asserts two things:

1. Generated `user_query.go` must contain `joinT.Schema(_q.schemaConfig.UserGroups)` (and must NOT contain a stale `edge.Schema = _q.schemaConfig.UserGroups` form).
2. Generated `group_update.go` must contain `edge.Schema = cfg.schemaConfig.UserGroups` at least 3 times (clear/remove/add edge mutations) and must NOT contain `edge.Schema = _u.schemaConfig.UserGroups` (the pre-descriptor receiver name).

Recent commits explicitly working in this area: `e7baca853 entc/gen: fix schemaconfig descriptor receiver leaks` and `72e36e830 entc/gen: restore schemaconfig hooks in descriptor SQL paths`. The fix was partial — one or more descriptor paths still emit the wrong receiver or omit the hook.

**Files:**
- Read first: `entc/gen/regression_test.go:150-210`
- Investigate (template paths): `entc/gen/template/dialect/sql/query.tmpl`, `entc/gen/template/dialect/sql/update.tmpl`, anything referencing `schemaConfig`
- Fix: whichever descriptor path still uses the wrong receiver or skips the hook

- [ ] **Step 1: Run the failing test and capture both generated files**

Run: `go test -count=1 -v -run '^TestGraph_Gen_SQLSchemaConfigHooksInDescriptorPaths$' ./entc/gen/... 2>&1 | tee /tmp/pr0-task5-failure.txt`

The failure output dumps the generated `user_query.go` content. Save it for inspection:

Run: `grep -o '/[^ :]*user_query.go' /tmp/pr0-task5-failure.txt | head -1 | xargs -I{} cp {} /tmp/pr0-task5-user_query.go`

Run: `grep -o '/[^ :]*group_update.go' /tmp/pr0-task5-failure.txt | head -1 | xargs -I{} cp {} /tmp/pr0-task5-group_update.go 2>/dev/null || echo 'group_update.go not in output - inspect tmpdir directly'`

If the second file isn't in the dump, find it: `find /tmp -path '*schemaconfigregen*' -name 'group_update.go' | head -1`.

- [ ] **Step 2: Determine which assertion is failing**

The require error message tells you whether the assertion about `user_query.go` or `group_update.go` failed first. Read the relevant generated file in `/tmp/pr0-task5-*.go` and compare against what the assertion expects.

For the user_query.go assertion: search for `loadGroups` or `withGroups` in the file. The hook should appear inside the join setup. If `joinT.Schema(_q.schemaConfig.UserGroups)` is missing but you see `joinT.Schema(...)` with a different receiver (e.g. `_u.` or `cfg.`), the receiver was named inconsistently.

For the group_update.go assertion: count occurrences of `edge.Schema = cfg.schemaConfig.UserGroups`. If you see `edge.Schema = _u.schemaConfig.UserGroups` instead, the descriptor path is still using the builder's receiver name (`_u`) rather than the descriptor's local `cfg` parameter.

- [ ] **Step 3: Locate the offending template**

Run: `grep -rn 'schemaConfig\.\|schemaConfig\b\|joinT.Schema' entc/gen/template/dialect/sql/ | head -40`

Cross-reference with the recent fix commits to see which paths were already updated:

Run: `git log --oneline -p -- entc/gen/template/dialect/sql/ | grep -A 30 'schemaconfig descriptor\|schemaconfig hooks' | head -80`

The path the recent fixes did NOT cover is what you need to update.

- [ ] **Step 4: Apply the fix**

In the descriptor path, replace the wrong receiver with the descriptor's local parameter (typically `cfg` in the descriptor signature) and/or add the missing schema hook call. The pattern from the already-fixed paths is the template to copy.

- [ ] **Step 5: Confirm the test passes**

Run: `go test -count=1 -v -run '^TestGraph_Gen_SQLSchemaConfigHooksInDescriptorPaths$' ./entc/gen/...`

Expected: `PASS`.

- [ ] **Step 6: Run the full regression suite and broader gen tests**

Run: `go test -count=1 ./entc/gen/...`

Expected: zero `--- FAIL` lines. **This is the green-baseline gate for the entire epic.**

- [ ] **Step 7: Commit**

```bash
git add entc/gen/template/dialect/sql/
git commit -m "fix(codegen): cover remaining schemaConfig hook paths in descriptor templates

Completes the descriptor-receiver fix begun in e7baca853 and 72e36e830:
the <which path> descriptor still emitted the builder receiver instead
of the descriptor's cfg parameter, leaving the schema hook unattached
in <user_query.go|group_update.go|both>. Now all descriptor paths route
schema configuration through cfg.schemaConfig consistently."
```

---

## Task 6: Lay out the bench harness file structure

Create the skeleton: directories, empty Go files with `package` declarations, a placeholder test file. This task locks in the layout decisions so subsequent tasks slot into a known structure.

**Files:**
- Create: `cmd/bench-codegen/main.go`
- Create: `internal/bench/bench.go`
- Create: `internal/bench/metrics.go`
- Create: `internal/bench/loc.go`
- Create: `internal/bench/loc_test.go`
- Create: `internal/bench/fixtures.go`
- Create: `internal/bench/README.md`

- [ ] **Step 1: Create directories**

Run: `mkdir -p cmd/bench-codegen internal/bench`

- [ ] **Step 2: Create `internal/bench/metrics.go` with the result types**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import "time"

// Run is one fixture's complete bench result. Serialized as a single JSON line.
type Run struct {
	Timestamp        time.Time   `json:"timestamp"`
	GitSHA           string      `json:"git_sha"`
	Fixture          string      `json:"fixture"`
	GenWallNS        int64       `json:"gen_wall_ns"`
	GenPeakRSSBytes  int64       `json:"gen_peak_rss_bytes"`
	BuildWallNS      int64       `json:"build_wall_ns"`
	BuildPeakRSSBytes int64      `json:"build_peak_rss_bytes"`
	TotalLOC         int         `json:"total_loc"`
	FileCount        int         `json:"file_count"`
	TopFiles         []FileStats `json:"top_files"`
}

// FileStats is the LOC count for a single generated file.
type FileStats struct {
	Path string `json:"path"`
	LOC  int    `json:"loc"`
}
```

- [ ] **Step 3: Create `internal/bench/loc.go` with the LOC counter (empty implementation; test drives it in Task 7)**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

// CountLOC walks the directory tree rooted at root, counting newlines in every
// regular file whose name ends with ".go". Returns total LOC, file count, and
// the top-N files by LOC (largest first). N=0 returns no top files.
func CountLOC(root string, topN int) (totalLOC, fileCount int, top []FileStats, err error) {
	panic("not implemented")
}
```

- [ ] **Step 4: Create `internal/bench/fixtures.go` with the fixture registry stub**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

// Fixture is one in-repo schema that the bench can measure end-to-end.
// SchemaDir is the path to the directory containing the ent.Schema-implementing
// Go files (relative to the repo root).
type Fixture struct {
	Name      string
	SchemaDir string
}

// InRepoFixtures returns the list of integration fixtures the bench knows about.
// Wired up in Task 10 once subprocess measurement is in place.
func InRepoFixtures() []Fixture {
	return []Fixture{
		{Name: "privacy", SchemaDir: "entc/integration/privacy/ent/schema"},
		{Name: "hooks", SchemaDir: "entc/integration/hooks/ent/schema"},
		{Name: "edgeschema", SchemaDir: "entc/integration/edgeschema/ent/schema"},
	}
}
```

- [ ] **Step 5: Create `internal/bench/bench.go` with the orchestration stub**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package bench measures the generation and compile cost of ent codegen
// fixtures. See cmd/bench-codegen for the CLI entrypoint.
package bench

import "errors"

// RunFixture copies the given fixture's schema to a temp module, runs codegen,
// runs go build, and returns a populated Run. Implemented in Task 9.
func RunFixture(repoRoot string, f Fixture) (Run, error) {
	return Run{}, errors.New("not implemented")
}
```

- [ ] **Step 6: Create `cmd/bench-codegen/main.go` with the CLI stub**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Command bench-codegen measures ent codegen cost across fixtures.
// See internal/bench/README.md for usage.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "bench-codegen: not yet implemented (see PR 0 plan, Task 11)")
	os.Exit(2)
}
```

- [ ] **Step 7: Create `internal/bench/README.md` with a usage stub**

```markdown
# bench-codegen

Measures ent codegen cost (generation wall + peak RSS, `go build` wall + peak RSS, LOC)
across in-repo fixtures and arbitrary downstream consumer schemas. Output is newline-
delimited JSON (NDJSON), one `Run` per fixture.

## In-repo fixtures (default)

(Filled in by Task 11.)

## Consumer-scale measurement

(Filled in by Task 13.)
```

- [ ] **Step 8: Verify the package compiles**

Run: `go build ./cmd/bench-codegen ./internal/bench/...`

Expected: clean build, no errors. The CLI binary will exit 2 if invoked — that's intentional until Task 11.

- [ ] **Step 9: Commit the scaffold**

```bash
git add cmd/bench-codegen internal/bench
git commit -m "feat(bench): scaffold cmd/bench-codegen and internal/bench package

Lays out the file structure for the codegen bench harness. Real
implementation lands in subsequent commits (Tasks 7-13)."
```

---

## Task 7: Implement and test the LOC counter

LOC counting is pure I/O + arithmetic. TDD it first; this gives the smallest possible round-trip to validate the bench package builds and tests run.

**Files:**
- Modify: `internal/bench/loc.go`
- Create: `internal/bench/loc_test.go`

- [ ] **Step 1: Write the failing test**

Write `internal/bench/loc_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountLOC(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package p\n\nfunc A() {}\n")            // 3 lines
	mustWrite(t, filepath.Join(root, "b.go"), "package p\nvar B = 1\n")                 // 2 lines
	mustWrite(t, filepath.Join(root, "sub", "c.go"), "package q\nfunc C() {\n\treturn\n}\n") // 4 lines
	mustWrite(t, filepath.Join(root, "ignore.txt"), "should be ignored\n")              // skipped: not .go

	total, count, top, err := CountLOC(root, 3)
	require.NoError(t, err)
	require.Equal(t, 9, total, "total LOC across all .go files")
	require.Equal(t, 3, count, "file count")
	require.Len(t, top, 3)

	// Top files are sorted by LOC desc; sort by path within equal-LOC ties handled by test order.
	sort.SliceStable(top, func(i, j int) bool { return top[i].LOC > top[j].LOC })
	require.Equal(t, 4, top[0].LOC)
	require.True(t, filepath.Base(top[0].Path) == "c.go", "largest file should be c.go, got %s", top[0].Path)
}

func TestCountLOC_TopNZero(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package p\n")
	total, count, top, err := CountLOC(root, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, count)
	require.Empty(t, top, "topN=0 returns no top-files list")
}

func TestCountLOC_MissingRoot(t *testing.T) {
	_, _, _, err := CountLOC("/this/path/does/not/exist", 5)
	require.Error(t, err)
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 -run TestCountLOC ./internal/bench/...`

Expected: PANIC (not implemented).

- [ ] **Step 3: Implement `CountLOC`**

Replace `internal/bench/loc.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CountLOC walks the directory tree rooted at root, counting newlines in every
// regular file whose name ends with ".go". Returns total LOC, file count, and
// the top-N files by LOC (largest first). N=0 returns no top files.
func CountLOC(root string, topN int) (totalLOC, fileCount int, top []FileStats, err error) {
	if _, statErr := os.Stat(root); statErr != nil {
		return 0, 0, nil, fmt.Errorf("loc: stat root: %w", statErr)
	}
	var all []FileStats
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		n, lerr := countLines(path)
		if lerr != nil {
			return fmt.Errorf("loc: %s: %w", path, lerr)
		}
		all = append(all, FileStats{Path: path, LOC: n})
		totalLOC += n
		fileCount++
		return nil
	})
	if walkErr != nil {
		return 0, 0, nil, walkErr
	}
	if topN > 0 && len(all) > 0 {
		sort.Slice(all, func(i, j int) bool { return all[i].LOC > all[j].LOC })
		if topN > len(all) {
			topN = len(all)
		}
		top = all[:topN]
	}
	return totalLOC, fileCount, top, nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024) // allow long lines (snapshot files)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -run TestCountLOC ./internal/bench/...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bench/loc.go internal/bench/loc_test.go
git commit -m "feat(bench): implement LOC counter for .go files

CountLOC walks a directory tree, counts newlines in every .go file,
and returns total LOC, file count, and the top-N largest files by LOC.
TDD with table-style tests covering basic counting, topN=0 elision,
and missing-root error."
```

---

## Task 8: Implement and test subprocess measurement

Subprocess measurement is the foundation for both gen-time and build-time metrics. Wrapping `exec.Cmd` to capture wall time and peak RSS via `ProcessState.SysUsage()` keeps both measurements consistent.

**Files:**
- Create: `internal/bench/proc.go`
- Create: `internal/bench/proc_test.go`

- [ ] **Step 1: Write the failing test**

Write `internal/bench/proc_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMeasureCmd_SuccessfulProcess(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("MeasureCmd uses POSIX rusage; skipping on %s", runtime.GOOS)
	}
	cmd := exec.Command("sh", "-c", "sleep 0.05")
	wall, peakRSS, err := MeasureCmd(cmd)
	require.NoError(t, err)
	require.GreaterOrEqual(t, wall, 50*time.Millisecond, "wall should include the 50ms sleep")
	require.Greater(t, peakRSS, int64(0), "peak RSS must be measurable on Linux/macOS")
}

func TestMeasureCmd_FailedProcess(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("MeasureCmd uses POSIX rusage; skipping on %s", runtime.GOOS)
	}
	cmd := exec.Command("sh", "-c", "exit 7")
	wall, peakRSS, err := MeasureCmd(cmd)
	require.Error(t, err, "non-zero exit must surface as error")
	require.GreaterOrEqual(t, wall, time.Duration(0), "wall should still be reported")
	require.GreaterOrEqual(t, peakRSS, int64(0), "peakRSS may be zero on early failure but never negative")
}
```

- [ ] **Step 2: Run the test to verify it fails (no `MeasureCmd` yet)**

Run: `go test -count=1 -run TestMeasureCmd ./internal/bench/...`

Expected: build failure — `undefined: MeasureCmd`.

- [ ] **Step 3: Implement `MeasureCmd`**

Write `internal/bench/proc.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// MeasureCmd runs cmd to completion, returning wall-clock time and peak RSS in
// bytes. peakRSS is read from the child's ProcessState after Wait() returns;
// the units of syscall.Rusage.Maxrss are platform-dependent (Linux: KB,
// macOS: bytes) and normalized here. On platforms without POSIX rusage, peakRSS
// will be 0.
//
// The error return reflects cmd.Run()'s error; non-nil err does not invalidate
// wall or peakRSS — partial measurements remain useful for diagnostics.
func MeasureCmd(cmd *exec.Cmd) (wall time.Duration, peakRSS int64, err error) {
	start := time.Now()
	err = cmd.Run()
	wall = time.Since(start)
	if ps := cmd.ProcessState; ps != nil {
		if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
			peakRSS = normalizeMaxRSS(ru.Maxrss)
		}
	}
	return wall, peakRSS, err
}

func normalizeMaxRSS(maxrss int64) int64 {
	// Linux: kilobytes. macOS: bytes. Other Unix: typically kilobytes.
	if runtime.GOOS == "darwin" {
		return maxrss
	}
	return maxrss * 1024
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -run TestMeasureCmd ./internal/bench/...`

Expected: PASS (both subtests on Linux/macOS; skipped on Windows).

- [ ] **Step 5: Commit**

```bash
git add internal/bench/proc.go internal/bench/proc_test.go
git commit -m "feat(bench): MeasureCmd captures wall time + peak RSS via rusage

Wraps exec.Cmd.Run() to record wall-clock duration and peak resident
set size from the child's ProcessState. Normalizes platform-specific
Maxrss units (Linux: KB, macOS: bytes) to bytes for output. Skips on
non-POSIX platforms."
```

---

## Task 9: Implement the fixture runner

`RunFixture` is the orchestrator: copy schema to a tmpdir module, run codegen via `entc.Generate`, run `go build`, count LOC, populate a `Run`. Use a subprocess invocation of `cmd/ent` (the existing CLI) so MeasureCmd works the same way for gen as for build.

**Files:**
- Modify: `internal/bench/bench.go`
- Create: `internal/bench/bench_test.go`

- [ ] **Step 1: Write the failing test (small, smoke-only)**

Write `internal/bench/bench_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package bench

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunFixture_SmokePrivacy is a smoke test: it runs the bench against the
// in-repo privacy fixture and asserts the Run struct is populated. It does
// NOT pin specific numbers (they're machine-dependent); it only verifies the
// pipeline produces non-zero metrics end to end.
func TestRunFixture_SmokePrivacy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bench requires POSIX rusage")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	repoRoot := repoRootForTest(t)
	run, err := RunFixture(repoRoot, Fixture{
		Name:      "privacy",
		SchemaDir: "entc/integration/privacy/ent/schema",
	})
	require.NoError(t, err, "bench should complete")
	require.Equal(t, "privacy", run.Fixture)
	require.Greater(t, run.GenWallNS, int64(0), "gen wall should be > 0")
	require.Greater(t, run.BuildWallNS, int64(0), "build wall should be > 0")
	require.Greater(t, run.TotalLOC, 0, "generated LOC should be > 0")
	require.Greater(t, run.FileCount, 0, "generated file count should be > 0")
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err, "git rev-parse must work in the test environment")
	return filepath.Clean(string(wd[:len(wd)-1])) // strip trailing newline
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 -run TestRunFixture_SmokePrivacy ./internal/bench/...`

Expected: FAIL with `not implemented`.

- [ ] **Step 3: Implement `RunFixture`**

Replace `internal/bench/bench.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package bench measures the generation and compile cost of ent codegen
// fixtures. See cmd/bench-codegen for the CLI entrypoint.
package bench

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunFixture copies the given fixture's schema to a fresh temp module, runs
// codegen, runs go build, and returns a populated Run. The in-repo fixture is
// never modified. repoRoot must be an absolute path to the ent module root.
func RunFixture(repoRoot string, f Fixture) (Run, error) {
	if !filepath.IsAbs(repoRoot) {
		return Run{}, fmt.Errorf("bench: repoRoot must be absolute, got %q", repoRoot)
	}
	mod, err := os.MkdirTemp("", "bench-"+f.Name+"-*")
	if err != nil {
		return Run{}, fmt.Errorf("bench: tempdir: %w", err)
	}
	defer os.RemoveAll(mod)

	if err := stageFixture(repoRoot, f, mod); err != nil {
		return Run{}, fmt.Errorf("bench: stage fixture: %w", err)
	}

	genWall, genRSS, err := runGen(mod)
	if err != nil {
		return Run{}, fmt.Errorf("bench: gen: %w", err)
	}

	target := filepath.Join(mod, "ent")
	buildWall, buildRSS, err := runBuild(target)
	if err != nil {
		return Run{}, fmt.Errorf("bench: build: %w", err)
	}

	totalLOC, fileCount, top, err := CountLOC(target, 20)
	if err != nil {
		return Run{}, fmt.Errorf("bench: count LOC: %w", err)
	}

	return Run{
		Timestamp:         time.Now().UTC(),
		GitSHA:            gitSHA(repoRoot),
		Fixture:           f.Name,
		GenWallNS:         genWall.Nanoseconds(),
		GenPeakRSSBytes:   genRSS,
		BuildWallNS:       buildWall.Nanoseconds(),
		BuildPeakRSSBytes: buildRSS,
		TotalLOC:          totalLOC,
		FileCount:         fileCount,
		TopFiles:          top,
	}, nil
}

// stageFixture writes a fresh go.mod + entc.go + copies the schema directory
// into mod. The module replaces entgo.io/ent with the local repo so codegen
// runs against the worktree's templates.
func stageFixture(repoRoot string, f Fixture, mod string) error {
	goMod := fmt.Sprintf(`module bench-%s

go 1.23

require entgo.io/ent v0.0.0

replace entgo.io/ent => %s
`, f.Name, repoRoot)
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}

	srcSchema := filepath.Join(repoRoot, f.SchemaDir)
	dstSchema := filepath.Join(mod, "schema")
	if err := copyTree(srcSchema, dstSchema); err != nil {
		return err
	}

	entcGo := []byte(`//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./schema", &gen.Config{Target: "./ent"}); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
`)
	return os.WriteFile(filepath.Join(mod, "gen.go"), entcGo, 0o644)
}

func runGen(mod string) (time.Duration, int64, error) {
	cmd := exec.Command("go", "run", "-mod=mod", "./gen.go")
	cmd.Dir = mod
	cmd.Env = append(os.Environ(), "GOWORK=off")
	wall, rss, err := MeasureCmd(cmd)
	return wall, rss, err
}

func runBuild(target string) (time.Duration, int64, error) {
	cmd := exec.Command("go", "build", "-mod=mod", "./...")
	cmd.Dir = target
	cmd.Env = append(os.Environ(), "GOWORK=off")
	wall, rss, err := MeasureCmd(cmd)
	return wall, rss, err
}

func gitSHA(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out[:len(out)-1])
}

func copyTree(src, dst string) error {
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
		return os.WriteFile(target, data, 0o644)
	})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 -timeout 120s -run TestRunFixture_SmokePrivacy ./internal/bench/...`

Expected: PASS. This test invokes real codegen + go build, so it takes several seconds.

- [ ] **Step 5: Commit**

```bash
git add internal/bench/bench.go internal/bench/bench_test.go
git commit -m "feat(bench): RunFixture stages, generates, builds, measures

Copies a fixture's schema to a temp module with a replace directive
to the local ent worktree, runs codegen via 'go run ./gen.go', runs
'go build ./...' on the generated package, and counts LOC. All gen
and build measurements pass through MeasureCmd so the units stay
consistent. Smoke-tested against the privacy fixture."
```

---

## Task 10: Wire the CLI

Make `cmd/bench-codegen` actually do something: run all fixtures (or one named fixture), serialize each `Run` as one JSON line on stdout (or to a file).

**Files:**
- Modify: `cmd/bench-codegen/main.go`

- [ ] **Step 1: Replace `cmd/bench-codegen/main.go` with the real CLI**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Command bench-codegen measures ent codegen cost across fixtures.
// See internal/bench/README.md for usage.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"entgo.io/ent/internal/bench"
)

func main() {
	var (
		fixtureFlag = flag.String("fixture", "all", "fixture name to run, or 'all'")
		outFlag     = flag.String("out", "-", "output path; '-' means stdout")
		schemaFlag  = flag.String("schema", "", "absolute path to an external schema dir (overrides -fixture)")
		labelFlag   = flag.String("label", "external", "fixture label to record when using -schema")
	)
	flag.Parse()

	repoRoot, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bench-codegen: %v\n", err)
		os.Exit(1)
	}

	var fixtures []bench.Fixture
	switch {
	case *schemaFlag != "":
		if !filepath.IsAbs(*schemaFlag) {
			fmt.Fprintln(os.Stderr, "bench-codegen: -schema must be an absolute path")
			os.Exit(1)
		}
		// Express the external schema as a path relative to repoRoot if it's inside;
		// otherwise stage it from its absolute location.
		rel, _ := filepath.Rel(repoRoot, *schemaFlag)
		if strings.HasPrefix(rel, "..") {
			rel = *schemaFlag // outside the repo: pass through absolutely
		}
		fixtures = []bench.Fixture{{Name: *labelFlag, SchemaDir: rel}}
	case *fixtureFlag == "all":
		fixtures = bench.InRepoFixtures()
	default:
		for _, f := range bench.InRepoFixtures() {
			if f.Name == *fixtureFlag {
				fixtures = append(fixtures, f)
			}
		}
		if len(fixtures) == 0 {
			fmt.Fprintf(os.Stderr, "bench-codegen: unknown fixture %q (known: %v)\n", *fixtureFlag, fixtureNames(bench.InRepoFixtures()))
			os.Exit(1)
		}
	}

	out := io.Writer(os.Stdout)
	if *outFlag != "-" {
		f, err := os.Create(*outFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bench-codegen: open out: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)

	exitCode := 0
	for _, f := range fixtures {
		fmt.Fprintf(os.Stderr, "bench-codegen: running %s...\n", f.Name)
		run, err := bench.RunFixture(repoRoot, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bench-codegen: %s: %v\n", f.Name, err)
			exitCode = 1
			continue
		}
		if err := enc.Encode(run); err != nil {
			fmt.Fprintf(os.Stderr, "bench-codegen: encode %s: %v\n", f.Name, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	root := strings.TrimRight(string(out), "\n")
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("abs: %w", err)
	}
	return abs, nil
}

func fixtureNames(fs []bench.Fixture) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Name
	}
	return out
}
```

- [ ] **Step 2: Build the binary to confirm it compiles**

Run: `go build ./cmd/bench-codegen`

Expected: clean build.

- [ ] **Step 3: Smoke-test the CLI with a single fixture**

Run: `go run ./cmd/bench-codegen -fixture privacy -out /tmp/pr0-bench-smoke.jsonl`

Expected:
- Stderr: `bench-codegen: running privacy...`
- `/tmp/pr0-bench-smoke.jsonl` contains exactly 1 JSON line with non-zero `gen_wall_ns`, `build_wall_ns`, `total_loc`.

Verify:

Run: `wc -l /tmp/pr0-bench-smoke.jsonl && jq '.fixture, .total_loc, .gen_wall_ns > 0, .build_wall_ns > 0' /tmp/pr0-bench-smoke.jsonl`

Expected: `1 ...jsonl` then `"privacy"`, a positive integer, `true`, `true`.

- [ ] **Step 4: Smoke-test running all fixtures**

Run: `go run ./cmd/bench-codegen -out /tmp/pr0-bench-all.jsonl`

Expected:
- Stderr lists each fixture name
- `/tmp/pr0-bench-all.jsonl` contains 3 lines (privacy, hooks, edgeschema)

Verify: `wc -l /tmp/pr0-bench-all.jsonl` → `3 ...jsonl`.

- [ ] **Step 5: Commit**

```bash
git add cmd/bench-codegen/main.go
git commit -m "feat(bench): CLI for cmd/bench-codegen

Adds -fixture (named or 'all'), -out (stdout or file), -schema/-label
(external consumer path), and -h help via the flag package. Runs each
fixture through bench.RunFixture and emits one JSON line per result."
```

---

## Task 11: Capture the green baseline and commit it

Now that gen, build, and LOC measurement all work and the regression suite is green, freeze the baseline numbers for every in-repo fixture.

**Files:**
- Create: `internal/bench/baseline.jsonl`

- [ ] **Step 1: Verify the regression suite is green (final pre-baseline check)**

Run: `go test -count=1 ./entc/gen/...`

Expected: zero `--- FAIL` lines. If anything fails, return to Tasks 2–5 before proceeding.

- [ ] **Step 2: Run the bench across all fixtures**

Run: `go run ./cmd/bench-codegen -out internal/bench/baseline.jsonl`

Expected: 3 lines in `internal/bench/baseline.jsonl`, one per in-repo fixture.

- [ ] **Step 3: Sanity-check the baseline output**

Run: `jq -c '{fixture, total_loc, file_count, gen_wall_ms: (.gen_wall_ns/1000000|round), build_wall_ms: (.build_wall_ns/1000000|round), gen_rss_mb: (.gen_peak_rss_bytes/1048576|round), build_rss_mb: (.build_peak_rss_bytes/1048576|round)}' internal/bench/baseline.jsonl`

Expected: 3 lines with positive integers for every field. Eyeball the numbers — if anything is suspiciously zero or absurdly large, investigate before committing.

- [ ] **Step 4: Commit the baseline**

```bash
git add internal/bench/baseline.jsonl
git commit -m "chore(bench): capture green baseline for in-repo fixtures

Records gen/build wall time, peak RSS, total LOC, file count, and the
top-20 largest generated files for the privacy, hooks, and edgeschema
fixtures. Subsequent PRs in the codegen-reduction epic must include a
Measurement section that diffs against this file."
```

---

## Task 12: Document consumer-scale measurement

The `internal/bench/README.md` from Task 6 is a stub. Fill it in so future contributors and the team's downstream service maintainers can run the bench against their own schema.

**Files:**
- Modify: `internal/bench/README.md`

- [ ] **Step 1: Replace the README with the full version**

```markdown
# bench-codegen

Measures ent codegen cost (generation wall + peak RSS, `go build` wall + peak RSS,
LOC) across in-repo fixtures and arbitrary downstream consumer schemas. Output is
newline-delimited JSON (NDJSON), one `Run` per fixture.

## In-repo fixtures (default)

```
go run ./cmd/bench-codegen -out /tmp/bench.jsonl
```

Runs the bench against the three in-repo fixtures defined in `fixtures.go`
(privacy, hooks, edgeschema). Each line of `/tmp/bench.jsonl` is one `Run`.

To run a single fixture:

```
go run ./cmd/bench-codegen -fixture privacy
```

The baseline for these fixtures (as of PR 0) lives at `internal/bench/baseline.jsonl`.
Subsequent epic PRs include a `## Measurement` section in the PR description with the
delta against this file.

## Consumer-scale measurement

Point the same tool at any directory that contains an `ent.Schema`-implementing Go
package. The bench will:

1. Copy the schema directory to a fresh temp module
2. Add a `replace entgo.io/ent => /path/to/this/repo` directive
3. Run `go run ./gen.go` to generate the ent package
4. Run `go build ./...` on the generated package
5. Record gen wall + RSS, build wall + RSS, total LOC, file count, top-20 largest files

```
go run ./cmd/bench-codegen \
  -schema /absolute/path/to/service/ent/schema \
  -label service-api-go \
  -out /tmp/service-baseline.jsonl
```

The `-label` flag is the `fixture` field in the output — pick a stable name so cross-PR
comparisons stay aligned.

## Interpreting the output

Each line is a JSON object:

```json
{
  "timestamp": "2026-05-15T18:30:00Z",
  "git_sha": "fe4d71fd8...",
  "fixture": "privacy",
  "gen_wall_ns": 12345678901,
  "gen_peak_rss_bytes": 234567890,
  "build_wall_ns": 56789012345,
  "build_peak_rss_bytes": 1234567890,
  "total_loc": 45000,
  "file_count": 78,
  "top_files": [
    {"path": "/tmp/bench-privacy-.../ent/internal/task_mutation.go", "loc": 1234}
  ]
}
```

Notes:
- `top_files` paths are inside the temp module, not the in-repo fixture. Compare by
  base name or by relative position from the temp module's `ent/` directory.
- Peak RSS units are normalized to bytes (Linux's `Maxrss * 1024`, macOS's native bytes).
- Non-POSIX platforms produce `peak_rss_bytes: 0` — the wall time and LOC numbers are
  still meaningful.

## Comparing two runs

Diff JSONL with `jq`:

```
jq -s '.[0] as $a | .[1] as $b | $b | {fixture, total_loc_delta: (.total_loc - $a.total_loc), gen_wall_delta_ms: ((.gen_wall_ns - $a.gen_wall_ns)/1000000|round)}' baseline.jsonl new.jsonl
```

(Adapt by fixture if running multiple — the snippet above assumes one-line files.)
```

- [ ] **Step 2: Commit**

```bash
git add internal/bench/README.md
git commit -m "docs(bench): README for in-repo and consumer-scale measurement"
```

---

## Task 13: Final acceptance check

Verify all PR 0 acceptance criteria from spec §4.

- [ ] **Step 1: `go test ./entc/gen/...` passes clean**

Run: `go test -count=1 ./entc/gen/...`

Expected: every package PASS, no `--- FAIL`.

- [ ] **Step 2: Bench tool produces JSON for in-repo fixtures**

Run: `go run ./cmd/bench-codegen -out /tmp/pr0-final-check.jsonl && wc -l /tmp/pr0-final-check.jsonl`

Expected: `3 ...jsonl`.

- [ ] **Step 3: `internal/bench/baseline.jsonl` exists and contains 3 runs**

Run: `wc -l internal/bench/baseline.jsonl && jq -c '.fixture' internal/bench/baseline.jsonl`

Expected: `3 ...jsonl`, then `"privacy"`, `"hooks"`, `"edgeschema"` (in any order).

- [ ] **Step 4: `internal/bench/README.md` documents consumer-scale procedure**

Run: `grep -q 'Consumer-scale measurement' internal/bench/README.md && echo OK`

Expected: `OK`.

- [ ] **Step 5: Branch summary**

Run: `git log --oneline master..HEAD`

Expected: ~10 commits (one per task that committed). Commit ordering matches the task numbering.

- [ ] **Step 6: Leave the branch local — do NOT push or open a PR yet**

Per epic-level policy: no PRs until end-of-epic, after every PR in the stack has been merged locally and the team has verified DX + build-time wins with the bench harness. Each epic PR is built and committed locally on its own branch; pushing and PR creation happen once at the end as a coordinated stack submission.

Verify no accidental push:

Run: `git config --get branch.codegen-epic/00-baseline.remote || echo "no upstream configured (expected)"`

Expected: `no upstream configured (expected)`.

The branch stays local. When the full epic is ready, a separate end-of-epic procedure will push the stack via `gs stack submit` (or equivalent) and open the PRs.

---

## Self-Review

### Spec coverage

| Spec §4 PR 0 requirement | Plan task |
|---|---|
| `internal/bench/` (or `cmd/bench-codegen/`) tool that runs ent test schemas | Tasks 6, 9 |
| Times `entc.Generate` and `go build ./...` against each fixture | Tasks 8, 9 |
| Records gen wall, gen RSS, build wall, build RSS, total LOC, top-20 per-file LOC, file count | Tasks 2 (Metric types), 7 (LOC), 8 (proc), 9 (orchestration) |
| Outputs NDJSON for cross-PR diffing | Tasks 6, 10 |
| Fix `TestGraph_Gen_GremlinDeleteAvoidsDuplicateSelfImport` | Task 2 |
| Fix `TestGraph_Gen_AssignGeneratedUserIntIDAcceptsInt` | Task 3 |
| Fix `TestGraph_Gen_SQLModifierDeleteBuilderHasModify` | Task 4 |
| Fix `TestGraph_Gen_SQLSchemaConfigHooksInDescriptorPaths` | Task 5 |
| Document procedure for downstream consumer in `internal/bench/README.md` | Task 12 |
| Bench output committed as `internal/bench/baseline.jsonl` | Task 11 |
| Acceptance: `go test ./entc/gen/...` passes clean | Task 13 step 1 |
| Acceptance: bench produces JSON for in-repo schemas | Task 13 step 2 |

All requirements mapped.

### Placeholder scan

Reviewed for "TBD", "TODO", "implement later", "similar to Task N" — none present. Every code step has complete code; every decision point in Tasks 2–5 has a concrete decision rule.

### Type consistency

- `Run` struct fields used identically in metrics.go (Task 6 step 2), bench.go (Task 9 step 3), and bench_test.go assertions (Task 9 step 1).
- `FileStats{Path, LOC}` consistent across loc.go and metrics.go.
- `Fixture{Name, SchemaDir}` consistent across fixtures.go, main.go, bench.go.
- `MeasureCmd` signature `(*exec.Cmd) (time.Duration, int64, error)` consistent between proc.go and bench.go callers.
- CLI flag names (`-fixture`, `-out`, `-schema`, `-label`) consistent between main.go and README.md.

No drift.
