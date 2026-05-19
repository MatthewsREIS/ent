# Codegen Reduction Lever W — WhereInput Predicate-Accumulator Collapse (Lever F + N, scoped to whereinputs)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Shrink the consumer `gen/whereinputs/` package by collapsing the 16,119 generated `if i.X != nil { append(preds, X(*i.X)) }` accumulator blocks into one-line generic helper calls. Then optionally trim UUID Cmp operators (which generate fields that are semantically nonsensical on random IDs). Projected: whereinputs 136K LOC → ~85-100K LOC, compile wall 25s → ~16-19s on the consumer build, **~6-10s wall savings, zero API change** for Lever F alone.

**Architecture:** Add a new `runtime/wherehelpers/` package in ent that provides generic `AppendPtr[T,P]`, `AppendSlice[T,P]`, `AppendBool[P]` helpers. Edit contrib's `where_input_subpkg.tmpl` so the per-field-per-operator emission switches from 3-line `if X != nil { append }` blocks to single-line `preds = wherehelpers.AppendXxx(preds, i.X, entity.XOp)` calls. The struct field shape stays unchanged (GraphQL contract preserved). Lever N (drop UUID `Gt/Gte/Lt/Lte/In/NotIn`) is a contingent follow-on if Phase F shows F1 alone underdelivered AND the frontend audit shows the ops are unused.

**Why this lever instead of B-4 (per-entity whereinputs split):** the 2026-05-18 recon revealed a single strongly-connected component of size **100** in the whereinputs cross-entity reference graph (94% of files participate in a cycle). Standard Go workarounds (interfaces, package merging, generics, reflection) all either lose type safety or push the cycle to the call site. The boilerplate density check then showed that **16,119 nil-check accumulators occupy 12% of the file** — collapsing this pattern via generics shrinks the package without needing to split it. See [[project-codegen-reduction-b4-b5-b6-sibling-cycles]] for the recon data.

**Tech Stack:** Go 1.25 (generics), ent codegen (`text/template`), entgql codegen extension (contrib). No migrator changes needed for Lever F alone (struct field shape preserved). Bench worktree at `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go`.

**Stack position:** new branch `codegen-reduction-whereinput-collapse` stacked on `codegen-reduction-per-entity-and-post-bench` (ent) via git-spice. Contrib gets a new branch `entgql-whereinput-collapse` stacked on `entgql-collection-subpkg`. Consumer reuses existing `codegen-reduction-consumer-migration` branch (forward-push, no new branch) — same in-place extension policy as the previous plan.

**Per [[feedback-bench-build-memory-limits]]:** every `go build` / `go run entc.go` in the bench worktree gets `GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 -p 4`.

**Per [[feedback-bench-same-host-comparison]]:** wall comparisons are same-host pre/post on the current machine. PR C baseline is **3:06.83 wall, 5.6 GB peak RSS** from memory [[project-codegen-reduction-b4-b5-b6-opportunities]].

---

## File Structure

### ent worktree

**New runtime package:**
- Create: `runtime/wherehelpers/wherehelpers.go` — three generic helper functions
- Create: `runtime/wherehelpers/wherehelpers_test.go` — table-driven test for each helper

**Tests:**
- Modify: `runtime/wherehelpers/wherehelpers_test.go` — `go test ./runtime/wherehelpers/`

### contrib worktree

**Templates:**
- Modify: `entgql/template/where_input_subpkg.tmpl:182-206` — replace inline nil-check emission with wherehelpers calls
- Modify: `entgql/template/where_input_subpkg.tmpl:30-43` — add `wherehelpers` import

**Tests:**
- Modify: `entgql/template_test.go` (if present) — assert generated code uses helpers

### consumer worktree (no source changes required for Lever F)

**Regen only:**
- Regenerated: `api-graphql/src/ent/gen/whereinputs/*.go` — every file's `P()` method body collapses
- Modified: `go.mod` / `go.sum` — pseudo-version bump for new ent + contrib branches

---

## Phase A — Sanity check + design verification

### Task 1: Verify generic helper signatures compile against the template patterns

The template at line 182-206 emits three distinct nil-check shapes based on operator type:
- **Niladic** (e.g., `IsNil`, `NotNil`): `if i.X { append(predicates, contact.XIsNil()) }`
- **Variadic** (e.g., `In`, `NotIn`): `if len(i.X) > 0 { append(predicates, contact.XIn(i.X...)) }`
- **Comparable** (default): `if i.X != nil { append(predicates, contact.XEQ(*i.X)) }`

Each shape needs a generic equivalent. Verify the type parameters work cleanly with Go's inference.

**Files:**
- Test: scratch file `/tmp/wherehelpers_sketch.go` — not committed

- [ ] **Step 1: Write a sketch that exercises each shape**

Create `/tmp/wherehelpers_sketch.go`:

```go
package main

import "fmt"

type Predicate = func(int) // stand-in for predicate.Contact

func AppendPtr[T any](preds []Predicate, value *T, op func(T) Predicate) []Predicate {
    if value != nil {
        preds = append(preds, op(*value))
    }
    return preds
}

func AppendSlice[T any](preds []Predicate, values []T, op func(...T) Predicate) []Predicate {
    if len(values) > 0 {
        preds = append(preds, op(values...))
    }
    return preds
}

func AppendBool(preds []Predicate, value bool, op func() Predicate) []Predicate {
    if value {
        preds = append(preds, op())
    }
    return preds
}

// Simulated entity helpers
func IDEQ(id string) Predicate    { return func(x int) {} }
func IDIn(ids ...string) Predicate { return func(x int) {} }
func IDIsNil() Predicate          { return func(x int) {} }

func main() {
    var preds []Predicate
    var id *string = nil
    var idIn []string
    isNil := false

    preds = AppendPtr(preds, id, IDEQ)
    preds = AppendSlice(preds, idIn, IDIn)
    preds = AppendBool(preds, isNil, IDIsNil)
    fmt.Println(len(preds))
}
```

- [ ] **Step 2: Compile**

```bash
go run /tmp/wherehelpers_sketch.go
```

Expected: exit 0, prints `0`.

If type inference fails (e.g., requires explicit type args at call site), the helper signatures are wrong — adjust before going further. The goal is no type args required at the call site, so the generated code stays single-line.

- [ ] **Step 3: Check the `*i.X` dereference case for non-pointer fields**

The template at line 201 emits `*i.{{ $field }}` only if `not $f.Type.RType.IsPtr`. Some field types are already pointers (e.g., the JSON-tagged `*time.Time`). For those, the dereference is skipped. The generic `AppendPtr` always dereferences — verify that's consistent or add a second helper variant if needed.

Quick check:
```bash
grep -nE "i\.[A-Z][A-Za-z0-9]+\)" /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql/src/ent/gen/whereinputs/contact.go | head -5
```

Look at the actual emitted shape. If some predicate ops take `*T` instead of `T`, a `AppendPtrPtr` helper is needed; otherwise `AppendPtr` suffices.

- [ ] **Step 4: Document the helper API decision**

Append to this plan section (or write a 10-line decision note `/tmp/wherehelpers-design.md`) recording:
- Final list of helpers (3 expected: AppendPtr, AppendSlice, AppendBool)
- Whether the call sites need explicit type args (should be no — Go inference handles `T` from the value)
- Whether any helper needs a pointer-to-pointer variant for already-pointer fields

---

## Phase B — Create the wherehelpers runtime package

### Task 2: Create branch + scaffold the package

**Files:**
- Create: `runtime/wherehelpers/wherehelpers.go`
- Create: `runtime/wherehelpers/wherehelpers_test.go`

- [ ] **Step 1: Create branch stacked on PR C HEAD**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git checkout codegen-reduction-per-entity-and-post-bench
git checkout -b codegen-reduction-whereinput-collapse
git spice branch track --base=codegen-reduction-per-entity-and-post-bench
```

- [ ] **Step 2: Write the failing test first**

Create `runtime/wherehelpers/wherehelpers_test.go`:

```go
package wherehelpers_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "entgo.io/ent/runtime/wherehelpers"
)

type pred func(int)

func TestAppendPtr_NilValue(t *testing.T) {
    var preds []pred
    var v *int
    got := wherehelpers.AppendPtr(preds, v, func(int) pred { return func(int) {} })
    require.Len(t, got, 0, "nil value must not append")
}

func TestAppendPtr_NonNilValue(t *testing.T) {
    var preds []pred
    v := 42
    got := wherehelpers.AppendPtr(preds, &v, func(int) pred { return func(int) {} })
    require.Len(t, got, 1, "non-nil value must append once")
}

func TestAppendSlice_Empty(t *testing.T) {
    var preds []pred
    got := wherehelpers.AppendSlice(preds, []int{}, func(...int) pred { return func(int) {} })
    require.Len(t, got, 0, "empty slice must not append")
}

func TestAppendSlice_NonEmpty(t *testing.T) {
    var preds []pred
    got := wherehelpers.AppendSlice(preds, []int{1, 2, 3}, func(...int) pred { return func(int) {} })
    require.Len(t, got, 1, "non-empty slice must append once")
}

func TestAppendBool_False(t *testing.T) {
    var preds []pred
    got := wherehelpers.AppendBool(preds, false, func() pred { return func(int) {} })
    require.Len(t, got, 0, "false must not append")
}

func TestAppendBool_True(t *testing.T) {
    var preds []pred
    got := wherehelpers.AppendBool(preds, true, func() pred { return func(int) {} })
    require.Len(t, got, 1, "true must append once")
}
```

- [ ] **Step 3: Run test to verify failure**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
go test ./runtime/wherehelpers/... -v
```

Expected: FAIL — package doesn't exist yet.

- [ ] **Step 4: Implement the package**

Create `runtime/wherehelpers/wherehelpers.go`:

```go
// Package wherehelpers provides generic predicate-accumulator helpers used by
// entgql-generated where_input code. They collapse what would otherwise be
// per-field-per-operator boilerplate blocks of the form:
//
//   if i.X != nil { preds = append(preds, entity.XOp(*i.X)) }
//
// to a single helper call:
//
//   preds = wherehelpers.AppendPtr(preds, i.X, entity.XOp)
package wherehelpers

// AppendPtr appends op(*value) to preds when value is non-nil.
// Use for single-value field predicates (EQ, NEQ, GT, GTE, LT, LTE, Contains, etc.).
func AppendPtr[T any, P any](preds []P, value *T, op func(T) P) []P {
    if value != nil {
        preds = append(preds, op(*value))
    }
    return preds
}

// AppendSlice appends op(values...) to preds when values is non-empty.
// Use for variadic field predicates (In, NotIn).
func AppendSlice[T any, P any](preds []P, values []T, op func(...T) P) []P {
    if len(values) > 0 {
        preds = append(preds, op(values...))
    }
    return preds
}

// AppendBool appends op() to preds when value is true.
// Use for niladic field predicates (IsNil, NotNil).
func AppendBool[P any](preds []P, value bool, op func() P) []P {
    if value {
        preds = append(preds, op())
    }
    return preds
}
```

- [ ] **Step 5: Run tests to verify pass**

```bash
go test ./runtime/wherehelpers/... -v
```

Expected: PASS — all 6 cases.

- [ ] **Step 6: Commit**

```bash
git add runtime/wherehelpers/
git commit -m "feat(runtime/wherehelpers): generic predicate-accumulator helpers for entgql where_input"
```

---

## Phase C — Edit contrib `where_input_subpkg.tmpl`

### Task 3: Create contrib branch + edit the template emission

**Files:**
- Modify: contrib `entgql/template/where_input_subpkg.tmpl:30-43` (imports)
- Modify: contrib `entgql/template/where_input_subpkg.tmpl:182-206` (per-field emission)

- [ ] **Step 1: Create contrib branch**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
git checkout entgql-collection-subpkg
git checkout -b entgql-whereinput-collapse
git spice branch track --base=entgql-collection-subpkg
```

- [ ] **Step 2: Add wherehelpers import to the template's import block**

Find the existing imports in `entgql/template/where_input_subpkg.tmpl` (around line 30-43). Add:

```
"entgo.io/ent/runtime/wherehelpers"
```

The other imports stay unchanged.

- [ ] **Step 3: Replace the per-field nil-check emission**

Current emission at lines 182-206:

```
{{- range $f := $comparableFields }}
    {{- range $op := safeOps $f }}
        {{- $func := print $f.StructField $op.Name }}
        {{- $field := $func }}
        {{- if eq $op.Name "EQ" }}
            {{- $field = $f.StructField }}
        {{- end }}
        {{- if $op.Niladic }}
            if i.{{ $field }} {
                predicates = append(predicates, {{ $n.Package }}.{{ $func }}())
            }
        {{- else }}
            {{- if $op.Variadic }}
                if len(i.{{ $field }}) > 0 {
                    predicates = append(predicates, {{ $n.Package }}.{{ $func }}(i.{{ $field }}...))
                }
            {{- else }}
                if i.{{ $field }} != nil {
                    predicates = append(predicates, {{ $n.Package }}.{{ $func }}({{ if not $f.Type.RType.IsPtr }}*{{ end }}i.{{ $field }}))
                }
            {{- end }}
        {{- end }}
    {{- end }}
{{- end }}
```

Replacement (each `if` block becomes a single helper call):

```
{{- range $f := $comparableFields }}
    {{- range $op := safeOps $f }}
        {{- $func := print $f.StructField $op.Name }}
        {{- $field := $func }}
        {{- if eq $op.Name "EQ" }}
            {{- $field = $f.StructField }}
        {{- end }}
        {{- if $op.Niladic }}
            predicates = wherehelpers.AppendBool(predicates, i.{{ $field }}, {{ $n.Package }}.{{ $func }})
        {{- else if $op.Variadic }}
            predicates = wherehelpers.AppendSlice(predicates, i.{{ $field }}, {{ $n.Package }}.{{ $func }})
        {{- else }}
            {{- /* AppendPtr expects op(T); template currently emits op(*ptr). If the field is already a pointer type (IsPtr=true), the existing code does NOT add `*`. We need a parallel helper for that case OR pre-dereference in the helper signature. Decision per Phase A Task 1 Step 3. */}}
            {{- if $f.Type.RType.IsPtr }}
                {{- /* Field type itself is a pointer — pass through */}}
                predicates = wherehelpers.AppendPtr(predicates, i.{{ $field }}, func(v {{ $f.Type.String }}) predicate.{{ $n.Name }} { return {{ $n.Package }}.{{ $func }}(v) })
            {{- else }}
                predicates = wherehelpers.AppendPtr(predicates, i.{{ $field }}, {{ $n.Package }}.{{ $func }})
            {{- end }}
        {{- end }}
    {{- end }}
{{- end }}
```

The `IsPtr=true` branch wraps with a function literal to handle the type mismatch — `AppendPtr` dereferences but the entity predicate takes the pointer. The function literal is small enough that its emission cost is negligible.

If Phase A Task 1 Step 3 found that `IsPtr=true` fields are rare or non-existent, simplify by removing the `IsPtr` branch entirely.

- [ ] **Step 4: Save (do NOT commit yet) — wait for regen verification**

---

## Phase D — Regen + iterate

### Task 4: Regenerate all entgql fixtures and verify they compile

**Files:** all `entgql/internal/todo/ent/whereinputs/*.go` (and similar in other fixtures)

- [ ] **Step 1: Regenerate one canonical fixture first**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
go generate ./entgql/internal/todo/...
```

Expected: regen succeeds. Emitted `whereinputs/*.go` files use `wherehelpers.AppendPtr` etc instead of `if X != nil { append }`.

- [ ] **Step 2: Build the regenerated fixture**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go build -p 4 ./entgql/internal/todo/...
```

Common failure modes:
- **Type inference failure**: Go can't infer `T` and `P` for the helper. Fix: either explicit type args in the template emission, or restructure helper signatures.
- **Unused imports**: if a fixture has no comparable fields (unlikely but possible), the `wherehelpers` import becomes unused. Fix: add an `if has-any-comparable-field` gate around the import emission.
- **Pointer-pointer mismatch**: if the IsPtr branch is wrong, the emitted code passes `*T` where helper expects `T`. Fix: adjust the wrapping function literal.

- [ ] **Step 3: Run fixture tests**

```bash
go test -count=1 ./entgql/internal/todo/...
```

Expected: all pass (Lever F is a refactor; behavior is identical).

- [ ] **Step 4: Repeat for all entgql fixtures**

```bash
for fix in $(ls entgql/internal/); do
  echo "=== $fix ==="
  go generate "./entgql/internal/$fix/..." || { echo "FAIL regen $fix"; break; }
  GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go build -p 4 "./entgql/internal/$fix/..." || { echo "FAIL build $fix"; break; }
  go test -count=1 "./entgql/internal/$fix/..." || { echo "FAIL test $fix"; break; }
done
```

Expected: all green.

- [ ] **Step 5: Spot-check the generated diff for LOC reduction**

```bash
# Pick a representative file and count its size pre/post
git diff --stat entgql/internal/todo/ent/whereinputs/ | head
```

Expected: each `whereinputs/<entity>.go` is meaningfully smaller (typical reduction: 2/3 of the nil-check block lines).

- [ ] **Step 6: Commit contrib template + regen**

```bash
git add entgql/template/where_input_subpkg.tmpl entgql/internal/
git commit -m "feat(entgql): collapse where_input predicate accumulators via wherehelpers"
```

---

## Phase E — Consumer build + bench (no migrator needed)

Lever F preserves the WhereInput struct field shape (every nil-check stays gated by the same field), so consumer code that constructs `WhereInput` literals and gqlgen-generated marshaling code is unchanged. The only consumer-side step is `go.mod` bump + regen + build.

### Task 5: Push branches + bump consumer pseudo-versions

**Files:** consumer `go.mod` / `go.sum`

- [ ] **Step 1: Push contrib branch**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
git push -u origin entgql-whereinput-collapse
```

- [ ] **Step 2: Push ent branch**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git push -u origin codegen-reduction-whereinput-collapse
```

- [ ] **Step 3: Compute new pseudo-versions + update consumer go.mod (in-place on existing branch)**

```bash
ENT_SHA=$(git -C /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake rev-parse codegen-reduction-whereinput-collapse)
ENT_TS=$(TZ=UTC git -C /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake show -s --format=%cd --date=format-local:%Y%m%d%H%M%S "$ENT_SHA")
ENT_SHORT=$(git -C /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake rev-parse --short=12 "$ENT_SHA")
CONTRIB_SHA=$(git -C /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg rev-parse entgql-whereinput-collapse)
CONTRIB_TS=$(TZ=UTC git -C /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg show -s --format=%cd --date=format-local:%Y%m%d%H%M%S "$CONTRIB_SHA")
CONTRIB_SHORT=$(git -C /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg rev-parse --short=12 "$CONTRIB_SHA")

cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
git status --short  # should be clean or local-path replaces only
git checkout -- go.mod go.sum 2>/dev/null || true
go mod edit \
  -replace="entgo.io/ent=github.com/MatthewsREIS/ent@v0.0.0-${ENT_TS}-${ENT_SHORT}" \
  -replace="entgo.io/contrib=github.com/codelite7/contrib@v0.0.0-${CONTRIB_TS}-${CONTRIB_SHORT}"
GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go mod tidy
```

### Task 6: Regenerate consumer + build + test

**Files:** all consumer `*_gen.go` (entirely auto-generated; no hand-edits expected)

- [ ] **Step 1: Regenerate consumer ent**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go run ./api-graphql/src/cmd/ent_codegen.go
```

(Adjust the regen entry path if different — read from `Taskfile.yml`.)

Expected: every `gen/whereinputs/*.go` shrinks; `wherehelpers` import appears in each.

- [ ] **Step 2: Cold build (this is the bench measurement)**

```bash
go clean -cache 2>/dev/null || true
cat > /tmp/compile-timer.sh <<'TIMER'
#!/usr/bin/env bash
start=$EPOCHREALTIME
"$@"
rc=$?
end=$EPOCHREALTIME
dur=$(awk -v s="$start" -v e="$end" 'BEGIN{printf "%.3f", e - s}')
tool=$(basename "$1")
pkg=""
for ((i=1; i<=$#; i++)); do
  if [[ "${!i}" == "-p" ]]; then j=$((i+1)); pkg="${!j}"; break; fi
done
echo "PROF $tool $dur $pkg" >&2
exit $rc
TIMER
chmod +x /tmp/compile-timer.sh

GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 GODEBUG=gctrace=1 \
  /usr/bin/time -v go build -p 4 -a -toolexec=/tmp/compile-timer.sh ./api-graphql/... \
  > /tmp/bench-leverw.stdout 2> /tmp/bench-leverw.log
```

Expected: exit 0. Watch for type-inference errors at the call sites — if seen, return to Phase C and adjust template/helper signatures.

- [ ] **Step 3: Run consumer unit tests**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
task test-unit
```

Expected: 2200 tests pass, ≤17 skipped, 0 failures (matches the PR C baseline). Lever F is a refactor — behavior is identical.

- [ ] **Step 4: Parse bench results**

```bash
echo "=== wall + RSS ==="
grep -E "Elapsed|Maximum resident" /tmp/bench-leverw.log

echo "=== whereinputs compile wall ==="
grep "^PROF compile" /tmp/bench-leverw.log | grep "whereinputs" | head

echo "=== LOC reduction ==="
find /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql/src/ent/gen/whereinputs -name "*.go" | xargs wc -l | tail -1
```

Expected metrics:
- Wall: < 3:06.83 (PR C baseline). Target: 3:00 or below (~6-10s savings).
- whereinputs single compile: < 25s (PR C measured). Target: 16-19s.
- whereinputs total LOC: < 136K. Target: 85-100K (35% reduction).

If wall is FLAT (within ~25-35s host noise per [[feedback-bench-same-host-comparison]]), Lever F alone didn't deliver. Decide: proceed to Phase F (Lever N extension) or ship as a LOC-organization improvement without wall claim.

- [ ] **Step 5: Commit consumer regen output to the existing branch**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
git add go.mod go.sum api-graphql/src/ent/gen
git commit -m "$(cat <<'COMMIT'
feat(ent): Lever W — WhereInput predicate-accumulator collapse

Consumer regen pulling ent + contrib branches that replace ~16K nil-check
accumulator blocks in whereinputs with single-line wherehelpers calls.
Struct field shape preserved (no GraphQL contract change). Pure refactor.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
COMMIT
)"
git push origin codegen-reduction-consumer-migration
```

---

## Phase F — Lever N (UUID Cmp drop) — contingent extension

Only execute Phase F if:
1. Phase E Task 6 Step 4 showed wall savings < 5s (Lever F alone underdelivered), AND
2. Frontend audit (next step) confirms `IDGT/IDGTE/IDLT/IDLTE/IDIn/IDNotIn`-style operators on UUID fields are unused by API consumers

Lever N drops operators from the generated WhereInput struct, which IS a GraphQL schema change. External API consumers using those operators would break.

### Task 7 (contingent): Frontend audit — verify UUID Cmp ops are unused

**Files:** read-only audit; produces an inventory doc

- [ ] **Step 1: Find the GraphQL schema source**

```bash
find /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go -name "*.graphql" -o -name "*.graphqls" -o -name "schema*.go" 2>/dev/null | head
```

- [ ] **Step 2: Find frontend or external API consumer queries**

The frontend may live in a separate repo. Ask the user: "Where does the GraphQL frontend (or external API) consume the schema?" Without knowing, Lever N is not safe to ship.

- [ ] **Step 3: Grep for UUID Cmp operator usage in the discovered consumers**

For each `<EntityID>GT`, `<EntityID>GTE`, `<EntityID>LT`, `<EntityID>LTE`, `<EntityID>In` (questionable for UUIDs), `<EntityID>NotIn` usage in frontend GraphQL queries, count occurrences.

- [ ] **Step 4: Make the ship decision**

If ANY discovered usage of dropped operators exists → halt Lever N, ship Lever F alone.
If zero usage → proceed to Task 8.

### Task 8 (contingent): Edit `safeOps` to skip UUID Cmp ops

**Files:** contrib `entgql/template_funcs.go` or wherever `safeOps` is defined

- [ ] **Step 1: Find `safeOps` definition**

```bash
grep -rn "safeOps\b" /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg/entgql/
```

- [ ] **Step 2: Add UUID-specific filter**

Modify `safeOps` so that when the field type is `uuid.UUID`, it emits only `EQ`, `NEQ`, `IsNil`, `NotNil`, `In`, `NotIn` (or even narrower — drop `In`/`NotIn` too if confirmed unused). Drop `GT`, `GTE`, `LT`, `LTE` unconditionally for UUIDs.

The exact decision matrix:
- `EQ`, `NEQ`: keep — primary lookup pattern
- `In`, `NotIn`: keep — batch lookup pattern, commonly used
- `IsNil`, `NotNil`: keep — required for nullable UUID fields
- `GT`, `GTE`, `LT`, `LTE`: drop — UUID lexicographic ordering is meaningless

- [ ] **Step 3: Re-run Phase D + Phase E**

The regen + build cycle from Phase D Step 4 and Phase E Task 6 Step 2 re-runs with the new safeOps logic. Expect another ~10-20K LOC reduction in whereinputs and another ~2-4s wall savings.

- [ ] **Step 4: Commit Lever N**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
git add entgql/
git commit -m "feat(entgql): Lever N — skip Cmp operators on UUID fields"
```

Repeat the push + bump pseudo-versions + consumer regen + commit cycle.

---

## Phase G — Sign-off + PR open

### Task 9: Open follow-up PRs

**Files:** none — git operations only

- [ ] **Step 1: Submit ent branch via git-spice**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git checkout codegen-reduction-whereinput-collapse
git spice branch submit --no-draft \
  --title "codegen-reduction PR E: Lever W — WhereInput predicate-accumulator collapse" \
  --body "<bench-led description with Δ wall + Δ LOC from Phase E Task 6 Step 4>"
```

Expected: PR created on `MatthewsREIS/ent`, base = `codegen-reduction-per-entity-and-post-bench` (i.e., stacked on PR C).

- [ ] **Step 2: Open contrib follow-up PR**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
gh pr create --repo codelite7/contrib --base entgql-collection-subpkg --head entgql-whereinput-collapse \
  --title "entgql: collapse where_input predicate accumulators via wherehelpers" \
  --body "<companion to ent PR E — bench-led description>"
```

- [ ] **Step 3: Extend consumer PR #3902 description**

Add a "## Lever W follow-up" section noting:
- The 4th commit on the existing `codegen-reduction-consumer-migration` branch is the regen + go.mod bump pulling Lever W
- Bench delta vs PR C baseline (from Phase E Task 6 Step 4)
- LOC delta in `api-graphql/src/ent/gen/whereinputs/`

- [ ] **Step 4: Update memory**

Update `project-codegen-reduction-bench-results.md` with a new "Post-Lever W" section. Update `project-codegen-reduction-prs-opened.md` with the new PR URLs.

---

## Risks + escalation

**Generic type-inference failure at scale.** The sketch in Phase A may succeed but the real generated code has ~16K helper calls with varied type combinations. If Go's inference chokes on any specific shape (e.g., entity-specific predicate types interacting with field types), the emission needs explicit type args. Mitigation: catch this in Phase D Step 2 (build the first fixture); if it fails, adjust the template emission to add explicit type params.

**Pointer-to-pointer fields.** Some entgql fields are already declared as `*time.Time` etc. The `AppendPtr` helper dereferences, but the entity-package predicate constructor accepts the dereferenced type. The wrapping function literal in Phase C Step 3 handles this — but if the IsPtr branch was off by one in real templates, regen fails. Catch in Phase D Step 2; adjust the IsPtr branch.

**Wall savings underdeliver.** If Lever F alone saves < 5s on the consumer cold build, the LOC-reduction benefits are real but the wall-time argument for the lever weakens. Decision: ship anyway for LOC organization OR proceed to Lever N (frontend audit required).

**Frontend audit not feasible.** If the frontend GraphQL consumer can't be audited (different repo, no access, etc.), Lever N is unsafe to ship. Skip Lever N; ship Lever F alone.

---

## Out of scope (explicit, do NOT pursue here)

- Per-entity sibling subpkg split (B-4/5/6) — disproven by [[project-codegen-reduction-b4-b5-b6-sibling-cycles]] for whereinputs + gqlcollections. The other 4 families (internal, edges, mutationinputs, gqledges) remain potentially viable as separate future plans.
- gqlcollections predicate collapse — gqlcollections has different boilerplate density (0 nil-checks per the survey) so Lever F doesn't apply. Different lever needed.
- Edge-predicate boilerplate in whereinputs (`HasXxx`, `HasXxxWith`) — separate template section (lines 207-243); not collapsed in this plan because the edge-predicate logic is more entwined with cross-entity references (the same SCC issue). Possible follow-up lever.
- gqlgen `graph` package (49s compile, biggest single hotspot). Out of ent/contrib scope.

---

## Definition of done

- [ ] `runtime/wherehelpers/` package exists and tests pass
- [ ] `entgql/template/where_input_subpkg.tmpl` uses wherehelpers for all comparable-field emission
- [ ] All `entgql/internal/*` fixtures regen + build + test green
- [ ] Consumer `go build ./api-graphql/...` succeeds with new ent + contrib pseudo-versions
- [ ] Consumer `task test-unit` matches PR C baseline (2200 tests, 17 skipped, 0 failures)
- [ ] Cold bench on current host shows wall ≤ 3:00 (~6s savings vs 3:06.83 baseline) OR LOC reduction >= 30K with wall flat-within-noise
- [ ] `project-codegen-reduction-bench-results.md` updated with post-Lever-W row
- [ ] Three artifacts opened: ent PR E (stacked on PR C), contrib follow-up PR, consumer PR #3902 extended with 4th commit
- [ ] (If Phase F executed) Lever N: ANY UUID Cmp operator usage in frontend confirmed zero; additional regen + build + bench passes
