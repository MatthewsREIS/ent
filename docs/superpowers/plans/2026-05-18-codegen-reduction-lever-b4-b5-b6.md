# Codegen Reduction Lever B-4 + B-5 + B-6 — Per-Entity Sibling Subpackages (BATCHED)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take the six "sibling subpackages" that B-1/B-2/B-3 created as flat single packages — `whereinputs/`, `internal/`, `edges/`, `gqlcollections/`, `mutationinputs/`, `gqledges/` — and split each into per-entity Go subpackages so all 800+ resulting compile units run in parallel. Targeted savings on the consumer cold build: BUILD cold wall **−20–35s** (currently 3:06.83 → projected 2:32–2:47), zero LOC delta, memory neutral.

**Architecture:** Apply the per-entity-package pattern PR 6 used for entity packages, plus B-1's `InEdgesPackage` scope qualifier pattern, to all six sibling families *in a single batched cycle*. Edit all 6 template families first (ent: `edges.tmpl` + `internal*.tmpl`; contrib: `where_input_subpkg.tmpl` + `mutation_input_sibling.tmpl` + `gql_collection_subpkg.tmpl` + `gql_edge_subpkg.tmpl`), then run **one** regen+fixture+build cycle. Per [[feedback-batch-template-edits]]: serializing the three levers wastes ~4× cycle time. Cross-entity edge predicates remain the structural risk — the recon phase enumerates and resolves them via interface-bridge helpers (the same pattern as PR 6's `internal/types.go` exports).

**Tech Stack:** Go 1.25, ent codegen (`text/template`), entgql codegen extension (contrib), `cmd/ent-codegen-migrate` for consumer call-site rewrites, bench worktree at `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go`, `cmd/bench-codegen` for fixture-scale measurement.

**Per [[project-codegen-reduction-prs-opened]]:** PR A/B/C, contrib PR, consumer PR are already open against forks. This work continues the existing git-spice stack:

- **ent** — new branch `codegen-reduction-sibling-split` stacked on `codegen-reduction-per-entity-and-post-bench` (PR C). Holds: scope helpers, template edits, **migrator updates** (cmd/ent-codegen-migrate), fixture regen.
- **contrib** — new branch `entgql-sibling-split` stacked on `entgql-collection-subpkg`. Holds: entgql template edits + extension wiring.
- **gemini consumer** — **REUSE the existing `codegen-reduction-consumer-migration` branch.** No new branch. The migrator output for B-4/5/6 adds one new commit on top of the existing 3 commits (`3e69e1088e`, `94f2f2a7ed`, `ae2dee5553`). PR #3902 gains this commit via a normal forward push. Do NOT branch off — the consumer PR stays single-threaded and grows incrementally.

Do NOT merge anything in the existing stack until B-4/B-5/B-6 are verified — they should land as stacked follow-up PRs on `MatthewsREIS/ent` and `codelite7/contrib`, and as an in-place extension to `MatthewsREIS/gemini` #3902.

**Per [[feedback-bench-build-memory-limits]]:** every `go build` / `go run entc.go` invocation in the bench worktree gets `GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 -p 4`. Sweep variations only at the bench stage, not during iteration.

**Per [[feedback-bench-same-host-comparison]]:** all wall-time comparisons are same-host pre/post on the current machine. Memory's `3:06.83` from PR C profile run is the relevant baseline. Do not compare against the prior `3:10.49` from the original B-3 run on the bench host — different host, results not comparable.

---

## File Structure

### ent worktree (`/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake`)

**Branch:** new `codegen-reduction-sibling-split` stacked on `codegen-reduction-per-entity-and-post-bench`

**Scope helpers (extend B-1's `InEdgesPackage` pattern):**
- Modify: `entc/gen/func.go` — add `InSiblingPackage` scope flag with the sibling name as a string ("whereinputs" / "internal" / "edges" / etc.) so `PackageQualifier()` and `SiblingImports` work uniformly across all six families
- Modify: `entc/gen/func_test.go` — parallel `TestTypeScope_QualifiesInSiblingPackage` test

**Template registry:**
- Modify: `entc/gen/template.go` — register new per-entity sibling templates:
  - `internal/model/<entity>` (Format `internal/%s/model.go`) — splits `internal/<entity>_model.go` files into `internal/<entity>/model.go`
  - `internal/mutation/<entity>` (Format `internal/%s/mutation.go`)
  - `internal/types/<entity>` (Format `internal/%s/types.go`)
  - `edges/<entity>` (Format `edges/%s/edges.go`) — replaces the existing flat `edges/<entity>.go`
- Modify: `entc/gen/graph.go` — extend the per-entity emission loop to attach `InSiblingPackage` scope with the family name when executing these templates

**Templates:**
- Modify: `entc/gen/template/edges.tmpl` — change `package edges` → `package {{ $.Type.PackageName }}edges` (or equivalent per-entity package name); drop self-package import; update `edges/shared.tmpl` aliases to live in *each* per-entity edges subpackage
- Modify: `entc/gen/template/internal.tmpl` — split into `internal/<entity>/` per-entity packages, one for each entity's model + mutation + types
- Modify: `entc/gen/template/internal_model.tmpl` — `package internal` → `package <entity>`; references to sibling-entity types go through interface bridges from the parent `internal` package (kept as a thin facade for cross-entity contracts)
- Modify: `entc/gen/template/internal_mutation.tmpl` — same pattern
- Modify: `entc/gen/template/internal_types.tmpl` — same pattern

**Facade re-exports (root `gen/<entity>_facade.go`):**
- Modify: `entc/gen/template/facade.tmpl` — extend var-forwarder section to re-export from the new per-entity sibling subpackages (e.g. `var <Entity>WhereInput = <entity>whereinputs.<Entity>WhereInput`) so root call sites keep working

**Tests:**
- Modify: `entc/gen/template_test.go` — extend the `TestTemplates_PR6SubPackageFormat`-style assertion to cover all 6 new per-entity formats
- Modify: `entc/internal/integration/` — fixture lint (already present from PR 6) auto-asserts the new layout; no new test code needed

**Fixtures (regenerated, ~hundreds of files across `entc/integration/*/ent/<sibling>/<entity>/`):**
- Created: e.g. `entc/integration/edgeschema/ent/internal/user/model.go`, `entc/integration/edgeschema/ent/edges/user/edges.go`
- Modified: existing flat `internal/<entity>_model.go`, `edges/<entity>.go` files get deleted by codegen

### contrib worktree (`/var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg`)

**Branch:** new `entgql-sibling-split` stacked on `entgql-collection-subpkg`

**Templates (4 sibling families):**
- Modify: `entgql/template/where_input_subpkg.tmpl` — `package whereinputs` → `package <entity>whereinputs`; per-entity package name from `nodePaginationNames`
- Modify: `entgql/template/mutation_input_sibling.tmpl` — `package mutationinputs` → `package <entity>mutationinputs`
- Modify: `entgql/template/gql_collection_subpkg.tmpl` — `package gqlcollections` → `package <entity>gqlcollections`
- Modify: `entgql/template/gql_edge_subpkg.tmpl` — `package gqledges` → `package <entity>gqledges`
- Modify: `entgql/template/gql_collection_subpkg_runtime.tmpl` + `gql_edge_subpkg_runtime.tmpl` — runtime helpers move into matching per-entity subpackages or get folded into root facade

**Generator wiring:**
- Modify: `entgql/extension.go` — change `subPkgDir := filepath.Join(g.Target, "whereinputs")` → loop per-entity emitting `filepath.Join(g.Target, "whereinputs", n.Package)`; same for `mutationinputs`, `gqlcollections`, `gqledges`
- Modify: `entgql/template.go` — `WhereInputSubpkgTemplate` etc. stay registered; emission path is the per-entity loop

**Fixtures (regenerated):**
- Created: `entgql/internal/todo/ent/whereinputs/todo/where_input.go` etc.

### consumer worktree (`/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go`)

**Branch:** `codegen-reduction-consumer-migration` (existing — extend with B-4/5/6 migration commit)

**Migrator additions (in ent worktree):**
- Modify: `cmd/ent-codegen-migrate/passes/imports.go` (or wherever sibling-import resolution lives) — add rewrites:
  - `whereinputs.<Entity>WhereInput` → `<entity>whereinputs.<Entity>WhereInput`
  - `mutationinputs.<Entity>Create` → `<entity>mutationinputs.<Entity>Create`
  - `gqlcollections.<Entity>...` → `<entity>gqlcollections.<Entity>...`
  - `gqledges.<Entity>...` → `<entity>gqledges.<Entity>...`
  - `edges.<Entity>...` → `<entity>edges.<Entity>...`
  - `internal.<Entity>Model` → `<entity>internal.<Entity>Model` (or via facade re-export)

**Consumer regen + migration output:** all `*_gen.go` and call-site files

**go.mod / go.sum:**
- Modify: `go.mod` — pseudo-version replaces bumped to new ent + contrib branch HEADs

**Bench artifacts:**
- New: `/var/tmp/bench-pr6/post-b4b5b6-<timestamp>.txt`
- Modified: `project_codegen_reduction_bench_results.md` (memory) — append B-4/5/6 row

---

## Phase A — Reconnaissance

The premise of this lever is "each sibling subpkg can be split per-entity." Cross-entity references inside sibling subpkgs are the structural risk. Map them first, before writing template code.

### Task 1: Enumerate cross-entity references in current sibling subpkgs

**Files:**
- Read-only: bench worktree gen output at `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql/src/ent/gen/{whereinputs,internal,edges,gqlcollections,mutationinputs,gqledges}/*.go`

- [ ] **Step 1: For each sibling subpkg, run a grep for cross-entity identifier references inside per-entity files**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql/src/ent/gen
for sib in whereinputs internal edges gqlcollections mutationinputs gqledges; do
  echo "=== $sib ==="
  # For each file (one per entity in the per-entity-file layout), grep for references to OTHER entity types
  for f in "$sib"/*.go; do
    base=$(basename "$f" .go)
    # Look for references to package-level types defined by OTHER entities in this same sibling subpkg
    refs=$(grep -oE '[A-Z][A-Za-z0-9]+(WhereInput|Model|Mutation|Create|Update|Edges|Edge|Collection)' "$f" | sort -u | grep -v "^${base^}" 2>/dev/null | head -10)
    if [[ -n "$refs" ]]; then
      echo "  $f references:"
      echo "$refs" | sed 's/^/    /'
    fi
  done
done > /tmp/sibling-cross-refs.txt
wc -l /tmp/sibling-cross-refs.txt
```

Expected: hundreds of cross-references — e.g., `whereinputs/escrow.go` references `ContactWhereInput`, `PropertyWhereInput` because escrow has `Contact` / `Property` edges with edge-predicates.

- [ ] **Step 2: Categorize cross-references by pattern**

Read `/tmp/sibling-cross-refs.txt` and classify each pattern:
- **Edge predicates** (most common): `<Other>WhereInput` referenced from edge fields like `HasContact *ContactWhereInput`
- **Edge model types**: `<Other>Model` referenced from per-entity model files
- **Edge mutation types**: `<Other>Mutation` referenced from per-entity mutation files
- **Other**: anything else — these are the unknowns to investigate before template work

- [ ] **Step 3: Document the resolution pattern**

Write `/tmp/sibling-split-resolution.md` summarizing the chosen pattern for each category:
- **Edge predicates**: each `<entity>whereinputs` package imports sibling `<other>whereinputs` packages for the referenced types. **Risk: import cycles** if A's whereinput references B and B's references A. Mitigation: parent `whereinputs/` package (the same name as today) keeps a *facade* of shared interface contracts.
- **Edge model types**: same pattern via parent `internal/` facade.
- **Other**: list each case and decide explicitly. Block plan execution if any category has no clean solution.

- [ ] **Step 4: Commit reconnaissance notes to ent worktree**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
mkdir -p docs/superpowers/specs
cp /tmp/sibling-split-resolution.md docs/superpowers/specs/2026-05-18-codegen-reduction-b4b5b6-cross-refs.md
git add docs/superpowers/specs/2026-05-18-codegen-reduction-b4b5b6-cross-refs.md
git commit -m "spec(codegen): B-4/5/6 cross-entity reference resolution map"
```

Expected: commit on the new branch (created in Task 2), or on a recon branch that gets the rest of the work stacked on it.

### Task 2: Create the stacked branches (ent + contrib)

**Files:** none modified; branch creation only

- [ ] **Step 1: Create ent branch stacked on PR C HEAD**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git checkout codegen-reduction-per-entity-and-post-bench
git checkout -b codegen-reduction-sibling-split
git spice branch track --base=codegen-reduction-per-entity-and-post-bench
```

Expected: `git spice ls` shows the new branch above the existing leaf.

- [ ] **Step 2: Create contrib branch stacked on entgql-collection-subpkg**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
git checkout entgql-collection-subpkg
git checkout -b entgql-sibling-split
# Initialize git-spice in contrib if not already done
git spice repo init --remote=origin --trunk=master 2>/dev/null || true
git spice branch track --base=entgql-collection-subpkg
```

Expected: contrib `git spice ls` shows the stack.

- [ ] **Step 3: Verify both worktrees clean**

```bash
git -C /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake status --short
git -C /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg status --short
```

Expected: no output (or only `?? .claude/` in ent).

---

## Phase B — Scope helper infrastructure (ent + contrib in parallel)

The B-1 plan added an `InEdgesPackage` boolean scope flag. For B-4/5/6 we need a more general mechanism that accepts the sibling family name as a string, so the same code-path handles all six families uniformly.

### Task 3: Generalize the scope flag from `InEdgesPackage` to `InSiblingPackage`

**Files:**
- Test: `entc/gen/func_test.go`
- Modify: `entc/gen/func.go` (extend the existing `InEdgesPackage` branch in `PackageQualifier`)

- [ ] **Step 1: Write the failing test**

Append to `entc/gen/func_test.go`:

```go
func TestTypeScope_QualifiesInSiblingPackage(t *testing.T) {
    // The new InSiblingPackage scope takes the sibling family name as a string,
    // and produces a qualifier matching the per-entity sub-package within that family.
    ty := &Type{Name: "User", Package: "user"}
    cases := []struct {
        family string
        want   string
    }{
        {"whereinputs", "userwhereinputs."},
        {"internal", "userinternal."},
        {"edges", "useredges."},
        {"gqlcollections", "usergqlcollections."},
        {"mutationinputs", "usermutationinputs."},
        {"gqledges", "usergqledges."},
    }
    for _, c := range cases {
        scope := &typeScope{Type: ty, Scope: map[any]any{"InSiblingPackage": c.family}}
        require.Equal(t, c.want, scope.PackageQualifier(),
            "InSiblingPackage=%q must prefix current-entity refs with <package><family>.", c.family)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
go test ./entc/gen/ -run TestTypeScope_QualifiesInSiblingPackage -v
```

Expected: FAIL — `PackageQualifier()` doesn't yet read `InSiblingPackage`.

- [ ] **Step 3: Implement in `entc/gen/func.go`**

In the existing `typeScope.PackageQualifier` method, add a branch reading `InSiblingPackage` (string-valued) before falling through to the existing logic:

```go
func (s *typeScope) PackageQualifier() string {
    if v, ok := s.Scope["InSiblingPackage"]; ok {
        if family, ok := v.(string); ok && family != "" {
            return s.Type.Package + family + "."
        }
    }
    if v, ok := s.Scope["InEdgesPackage"]; ok {
        if b, ok := v.(bool); ok && b {
            return s.Type.Package + "."
        }
    }
    // ... existing logic falls through
    return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./entc/gen/ -run TestTypeScope_QualifiesInSiblingPackage -v
```

Expected: PASS — all 6 sub-cases.

- [ ] **Step 5: Also extend `SiblingImports` to drop the current per-entity sibling subpackage**

The existing `SiblingImports` helper drops imports of OTHER siblings inside a sub-package. For per-entity sibling subpackages, the same drop must apply to `<currentEntity><family>`. Modify the helper accordingly. Add a parallel test asserting that when `InSiblingPackage` is set, the current-entity-family import is dropped.

- [ ] **Step 6: Commit**

```bash
git add entc/gen/func.go entc/gen/func_test.go
git commit -m "feat(entc/gen/func): InSiblingPackage scope for per-entity sibling subpackages"
```

### Task 4: Register the new per-entity sibling templates in `entc/gen/template.go`

**Files:**
- Modify: `entc/gen/template.go` (extend `Templates` slice)

- [ ] **Step 1: Find the existing `edges/type` template entry**

```bash
grep -n "edges/type\|Format.*edges" /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake/entc/gen/template.go | head -5
```

Note the exact pattern used by B-1's `edges/type` entry.

- [ ] **Step 2: Add 4 new entries to the Templates slice**

For each of: `internal/model`, `internal/mutation`, `internal/types`, `edges/type` (replace existing flat one).

The Format strings must be `"internal/%s/model.go"`, `"internal/%s/mutation.go"`, `"internal/%s/types.go"`, `"edges/%s/edges.go"` — the `%s` resolves to `$.Package` of the type being emitted.

Each entry should have `SubPackage: true` (or whatever flag B-1 added — read the existing entry first to confirm) so the generator's emission loop iterates per entity.

- [ ] **Step 3: Run template registration tests**

```bash
go test ./entc/gen/ -run TestTemplates -v
```

Expected: existing templates pass; new entries register without error. If there's an explicit "expected file format list" assertion, extend it to include the new formats.

- [ ] **Step 4: Commit**

```bash
git add entc/gen/template.go entc/gen/template_test.go
git commit -m "feat(entc/gen): register per-entity templates for internal + edges siblings"
```

### Task 5: Wire `InSiblingPackage` into `graph.go` emission

**Files:**
- Modify: `entc/gen/graph.go` (the per-entity template execution loop)

- [ ] **Step 1: Find the existing `InEdgesPackage` wrapping in graph.go**

```bash
grep -n "InEdgesPackage" /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake/entc/gen/graph.go
```

- [ ] **Step 2: Add parallel wrapping for the new templates**

For each per-entity sibling template emission, wrap the type in `&typeScope{Type: n, Scope: map[any]any{"InSiblingPackage": "<family>"}}` where `<family>` matches the template's family ("edges", "internal", etc.).

- [ ] **Step 3: Smoke-test by regenerating one fixture**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
go generate ./entc/integration/edgeschema/ent/...
```

Expected: regen succeeds OR fails with template-error about unrecognized syntax — both are signals that the wiring is reaching the templates. (Templates themselves are edited in Phase C; this just confirms wiring.)

- [ ] **Step 4: Commit**

```bash
git add entc/gen/graph.go
git commit -m "feat(entc/gen): wire InSiblingPackage scope into per-entity emission loop"
```

### Task 6 (parallelizable with Tasks 3-5): Add `InSiblingPackage` equivalents in contrib

**Files:**
- Modify: contrib `entgql/extension.go` (`subPkgDir` resolution, per-entity loop)
- Modify: contrib `entgql/template.go` (templates already registered; just adjust emission path)

- [ ] **Step 1: Find the existing whereinputs emission in `extension.go`**

```bash
grep -n "subPkgDir\|whereinputs" /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg/entgql/extension.go | head -10
```

Note current line ~906: `subPkgDir := filepath.Join(g.Target, "whereinputs")`.

- [ ] **Step 2: Change directory layout to per-entity**

Modify the existing loop so for each entity `n`:
```go
entityDir := filepath.Join(g.Target, "whereinputs", n.Package)
if err := os.MkdirAll(entityDir, 0o755); err != nil {
    return fmt.Errorf("entgql: create whereinputs/%s dir: %w", n.Package, err)
}
filename := filepath.Join(entityDir, "where_input.go")
```

Repeat for `mutationinputs`, `gqlcollections`, `gqledges`.

- [ ] **Step 3: Run contrib unit tests**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
go test ./entgql/... -count=1
```

Expected: some tests fail because the templates themselves aren't updated yet — but the extension's path-resolution logic is now per-entity.

- [ ] **Step 4: Commit**

```bash
git add entgql/extension.go entgql/template.go
git commit -m "feat(entgql/extension): emit sibling subpackages into per-entity directories"
```

---

## Phase C — Batched template edits (BIG step; all 6 families at once)

**This is the batched-edit phase.** Per [[feedback-batch-template-edits]], do NOT serialize per-family — apply all six template changes, THEN regen once. The signal you want is "does the whole batch compile after regen", not "did family A compile before I touched family B".

### Task 7: Edit ent templates (3 families: `edges`, `internal_model`, `internal_mutation`, `internal_types`)

**Files:**
- Modify: `entc/gen/template/edges.tmpl`
- Modify: `entc/gen/template/internal_model.tmpl`
- Modify: `entc/gen/template/internal_mutation.tmpl`
- Modify: `entc/gen/template/internal_types.tmpl`

- [ ] **Step 1: `edges.tmpl` — change package name to per-entity**

In `entc/gen/template/edges.tmpl`, change all occurrences of `package edges` to:

```
package {{ $.Package }}edges
```

Update the `with extend $ "Package" "edges"` block to use the per-entity name (e.g. `extend $ "Package" (print $.Package "edges")`).

Update the cross-entity edge target imports (which previously didn't need qualifiers within the flat package) to import from the matching `<other>edges` per-entity packages.

- [ ] **Step 2: `internal_model.tmpl` — per-entity model package**

Change `package internal` to `package {{ $.Package }}internal`. Cross-entity model references that previously resolved within `internal/` now need either:
- (a) explicit import of `<other>internal` siblings, OR
- (b) hoist the cross-cutting interface contracts (Selector, Querier, etc.) into a thin `internal/` parent facade that all per-entity packages import.

Pattern (b) is preferred — mirrors PR 6's `internal/types.go` pattern. The parent `internal/` package becomes the "shared types" facade; `internal/<entity>/` holds entity-specific code.

- [ ] **Step 3: `internal_mutation.tmpl` — same pattern**

- [ ] **Step 4: `internal_types.tmpl` — same pattern**

- [ ] **Step 5: Save progress; do NOT commit yet — Phase C is one logical batch**

### Task 8: Edit contrib templates (4 families: `where_input`, `mutation_input`, `gql_collection`, `gql_edge`)

**Files:**
- Modify: contrib `entgql/template/where_input_subpkg.tmpl`
- Modify: contrib `entgql/template/mutation_input_sibling.tmpl`
- Modify: contrib `entgql/template/gql_collection_subpkg.tmpl`
- Modify: contrib `entgql/template/gql_edge_subpkg.tmpl`
- Modify: contrib `entgql/template/gql_collection_subpkg_runtime.tmpl`
- Modify: contrib `entgql/template/gql_edge_subpkg_runtime.tmpl`

- [ ] **Step 1: `where_input_subpkg.tmpl` — per-entity package**

Currently emits `package whereinputs`. Change to `package {{ $.Node.Package }}whereinputs`.

Currently imports `"{{ $.Config.Package }}/predicate"` and `"{{ $.Config.Package }}/{{ $n.Package }}"`. Add: import the matching `<other>whereinputs` for each edge that has `where_input` predicates pointing at another entity's `<Other>WhereInput`.

Replace the `*ContactWhereInput` field type with `*<entity>whereinputs.ContactWhereInput` where `<entity>` is `contact` — i.e. import `gen/whereinputs/contact` and use its qualified `ContactWhereInput`.

- [ ] **Step 2: `mutation_input_sibling.tmpl` — same pattern**

- [ ] **Step 3: `gql_collection_subpkg.tmpl` — same pattern, package name `<entity>gqlcollections`**

The runtime hooks file (`gql_collection_subpkg_runtime.tmpl`) — decide whether it stays at the root `gqlcollections/` package (as a shared dispatch entry) or moves into each per-entity subpkg. Default: keep one shared `gqlcollections/runtime.go` at the parent level for cross-entity dispatch (the same pattern as `internal/` parent facade), with per-entity hooks calling back to it.

- [ ] **Step 4: `gql_edge_subpkg.tmpl` — same pattern, package name `<entity>gqledges`**

- [ ] **Step 5: Save progress; do NOT commit yet**

### Task 9: Update root `facade.tmpl` re-exports

**Files:**
- Modify: `entc/gen/template/facade.tmpl`

- [ ] **Step 1: Find existing var-forwarder section for B-1's edges**

```bash
grep -n "var With\|edges\." /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake/entc/gen/template/facade.tmpl | head -10
```

- [ ] **Step 2: Extend forwarders to import from per-entity sibling subpackages**

For each entity `$n` and each sibling family that has root-facade re-exports, emit:

```
var <Entity>WhereInput = {{ $n.Package }}whereinputs.{{ $n.Name }}WhereInput
```

This keeps consumer code that uses `gen.UserWhereInput` working without changes.

- [ ] **Step 3: Save progress; do NOT commit yet**

---

## Phase D — Regen + iterate (single batched cycle)

Now run ONE full regen across all in-repo fixtures and observe what breaks. This is where issues surface; iterate on Phase C edits until clean.

### Task 10: Regen all in-repo fixtures + iterate

**Files:** all `entc/integration/*/ent/**` (hundreds of regenerated files)

- [ ] **Step 1: Wipe + regen one canonical fixture first**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
rm -rf entc/integration/edgeschema/ent
go generate ./entc/integration/edgeschema/...
```

Expected: regen succeeds. If it fails with template-syntax errors, fix in Phase C and re-run.

- [ ] **Step 2: Build the regenerated fixture**

```bash
GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go build -p 4 ./entc/integration/edgeschema/...
```

Expected: build succeeds. If it fails with import-cycle errors, the cross-entity reference resolution from Phase A Task 1 needs adjustment — likely need to hoist more types into parent facades.

Common failure modes and fixes:
- **Import cycle** (`<entity>whereinputs` imports `<other>whereinputs` which imports back): hoist the shared types into the parent `whereinputs/` package.
- **Undefined `X`**: the per-entity package's name was not updated everywhere, or a cross-entity reference wasn't qualified.
- **Duplicate package name in same dir**: emission path resolution wrong; check Task 6's `entityDir` logic.

- [ ] **Step 3: Run fixture tests**

```bash
go test -count=1 ./entc/integration/edgeschema/...
```

Expected: tests pass.

- [ ] **Step 4: Repeat Steps 1-3 for all other in-repo fixtures**

```bash
for fix in privacy hooks edgeschema migrate_v2 multischema; do
  echo "=== $fix ==="
  rm -rf "entc/integration/$fix/ent"
  go generate "./entc/integration/$fix/..." || { echo "FAIL regen $fix"; break; }
  GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go build -p 4 "./entc/integration/$fix/..." || { echo "FAIL build $fix"; break; }
done
```

Expected: all green. Iterate Phase C edits as needed until all pass.

- [ ] **Step 5: Commit the batched template edits as logical units**

Per [[feedback-batch-template-edits]], the *edits* were batched. The *commits* should still be readable per-family. Split the batched changes into per-family commits when staging:

```bash
# Commit 1 — ent edges
git add entc/gen/template/edges.tmpl entc/integration/*/ent/edges/
git commit -m "feat(entc/gen): per-entity edges subpackages (B-5a)"

# Commit 2 — ent internal
git add entc/gen/template/internal_*.tmpl entc/integration/*/ent/internal/
git commit -m "feat(entc/gen): per-entity internal subpackages (B-5b)"

# Commit 3 — root facade re-exports
git add entc/gen/template/facade.tmpl
git commit -m "feat(entc/gen): facade re-exports from per-entity sibling subpackages"
```

Similar split on contrib side for B-4 (whereinputs), B-6a (mutationinputs), B-6b (gqlcollections), B-6c (gqledges).

---

## Phase E — Migrator updates (ent worktree, on `codegen-reduction-sibling-split` branch)

Without migrator updates, the consumer migration becomes a manual sweep of ~hundreds of import statements and qualifier rewrites across 50K+ lines. With migrator updates, it's a single tool invocation that lands as one commit on the existing gemini branch.

The migrator already handles the PR 6 per-entity entity package split. B-4/B-5/B-6 add an analogous *sibling* per-entity split. The new passes follow the same architecture as the existing entity-package passes — read the latest passes added during the migrator follow-up commits (`62efb1eb6` `03b7a3bd4` etc. on the current PR C branch) as the implementation reference.

### Task 11: Inventory consumer call sites by sibling family

Before writing migrator code, enumerate what shapes the migrator must handle. This avoids "write pass, find it missed a pattern, iterate" churn.

**Files:**
- Read-only: consumer code at `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql/src`

- [ ] **Step 1: For each sibling family, grep consumer code for current call-site shapes**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
for sib in whereinputs mutationinputs gqlcollections gqledges edges internal; do
  echo "=== $sib (consumer usage) ==="
  # Direct package qualifier usage
  grep -rn "\b${sib}\." api-graphql/src --include="*.go" 2>/dev/null \
    | grep -v "/gen/" \
    | head -10
  echo ""
  # Import statements
  grep -rn "\"[^\"]*gen/${sib}\"" api-graphql/src --include="*.go" 2>/dev/null \
    | head -5
  echo ""
done > /tmp/sibling-consumer-usage.txt
wc -l /tmp/sibling-consumer-usage.txt
```

Expected: a list of every consumer file importing or referencing each sibling family. The output guides which passes are needed.

- [ ] **Step 2: Categorize the call-site shapes**

For each sibling family, write down which of these patterns appear:
- (i) `gen.<Entity><Suffix>` (facade re-export) — **no migrator pass needed**, root facade handles it
- (ii) `<family>.<Entity><Suffix>` (direct sibling-pkg qualifier) — **needs rewrite to `<entity><family>.<Entity><Suffix>`**
- (iii) Bare identifier inside the entity's own package — **no rewrite needed**
- (iv) Selector in struct field type / function signature / return type — same as (ii), needs rewrite

Record the count of each pattern per family. If pattern (i) dominates and (ii) is rare, the migrator work is small. If (ii) is common, every consumer file with that family's import needs touching.

- [ ] **Step 3: Save the inventory**

```bash
cp /tmp/sibling-consumer-usage.txt /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake/docs/superpowers/specs/2026-05-18-codegen-reduction-b4b5b6-consumer-inventory.md
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git add docs/superpowers/specs/2026-05-18-codegen-reduction-b4b5b6-consumer-inventory.md
git commit -m "spec(codegen): B-4/5/6 consumer call-site inventory"
```

### Task 12: Add the `siblingPackageImport` migrator pass

**Files:**
- Test: `cmd/ent-codegen-migrate/passes/sibling_package_import_test.go` (new)
- Create: `cmd/ent-codegen-migrate/passes/sibling_package_import.go` (new)
- Modify: `cmd/ent-codegen-migrate/passes/passes.go` (or wherever the pass registry lives — check existing pass file naming first)

- [ ] **Step 1: Find the existing PR 6 import-rewrite pass for reference**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
ls cmd/ent-codegen-migrate/
grep -rln "RewriteImport\|importPath\|alias" cmd/ent-codegen-migrate/ | head
```

Read the closest analog (the per-entity-package import pass from PR 6) and use its structure as the template for the new sibling pass.

- [ ] **Step 2: Write the unit-test scaffolding**

Create `cmd/ent-codegen-migrate/passes/sibling_package_import_test.go`:

```go
package passes_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "<module-root>/cmd/ent-codegen-migrate/passes"
)

// Each test asserts the migrator rewrites a specific sibling-pkg call-site shape
// from the flat-package layout (pre B-4/5/6) to the per-entity-subpkg layout.
//
// The cases cover all 6 sibling families plus the "use facade re-export" preference.

func TestSiblingPackageImport_WhereInputDirectQualifier(t *testing.T) {
    src := `package main

import (
    "example.com/gen/whereinputs"
)

func use() *whereinputs.UserWhereInput {
    return &whereinputs.UserWhereInput{}
}
`
    want := `package main

import (
    userwhereinputs "example.com/gen/whereinputs/user"
)

func use() *userwhereinputs.UserWhereInput {
    return &userwhereinputs.UserWhereInput{}
}
`
    got, err := passes.SiblingPackageImport(src, passes.SiblingConfig{
        GenPackage: "example.com/gen",
        Families:   []string{"whereinputs", "mutationinputs", "gqlcollections", "gqledges", "edges"},
    })
    require.NoError(t, err)
    require.Equal(t, want, got)
}

func TestSiblingPackageImport_FacadeReExportLeftAlone(t *testing.T) {
    // gen.UserWhereInput is the facade re-export — pass MUST NOT rewrite to userwhereinputs.UserWhereInput.
    src := `package main

import "example.com/gen"

func use() *gen.UserWhereInput {
    return &gen.UserWhereInput{}
}
`
    got, err := passes.SiblingPackageImport(src, passes.SiblingConfig{
        GenPackage: "example.com/gen",
        Families:   []string{"whereinputs"},
    })
    require.NoError(t, err)
    require.Equal(t, src, got)
}

func TestSiblingPackageImport_MutationInputDirect(t *testing.T) {
    src := `package main

import "example.com/gen/mutationinputs"

var _ = mutationinputs.CreatePropertyInput{}
`
    want := `package main

import propertymutationinputs "example.com/gen/mutationinputs/property"

var _ = propertymutationinputs.CreatePropertyInput{}
`
    got, err := passes.SiblingPackageImport(src, passes.SiblingConfig{
        GenPackage: "example.com/gen",
        Families:   []string{"mutationinputs"},
    })
    require.NoError(t, err)
    require.Equal(t, want, got)
}

func TestSiblingPackageImport_MultipleEntitiesSameFile(t *testing.T) {
    // A consumer file referencing multiple entities through one sibling family
    // gets one import per touched entity.
    src := `package main

import "example.com/gen/whereinputs"

func f(u *whereinputs.UserWhereInput, p *whereinputs.PropertyWhereInput) {}
`
    want := `package main

import (
    propertywhereinputs "example.com/gen/whereinputs/property"
    userwhereinputs "example.com/gen/whereinputs/user"
)

func f(u *userwhereinputs.UserWhereInput, p *propertywhereinputs.PropertyWhereInput) {}
`
    got, err := passes.SiblingPackageImport(src, passes.SiblingConfig{
        GenPackage: "example.com/gen",
        Families:   []string{"whereinputs"},
    })
    require.NoError(t, err)
    require.Equal(t, want, got)
}

func TestSiblingPackageImport_EdgesAccessor(t *testing.T) {
    // edges/<entity>/ holds the WithX / QueryX helpers
    src := `package main

import "example.com/gen/edges"

func f(q *UserQuery) { edges.WithUserCreatedBy(q) }
`
    want := `package main

import useredges "example.com/gen/edges/user"

func f(q *UserQuery) { useredges.WithUserCreatedBy(q) }
`
    got, err := passes.SiblingPackageImport(src, passes.SiblingConfig{
        GenPackage: "example.com/gen",
        Families:   []string{"edges"},
    })
    require.NoError(t, err)
    require.Equal(t, want, got)
}

func TestSiblingPackageImport_InternalModelType(t *testing.T) {
    // internal/<entity>/ holds the entity-specific model
    src := `package main

import "example.com/gen/internal"

var _ internal.UserModel
`
    want := `package main

import userinternal "example.com/gen/internal/user"

var _ userinternal.UserModel
`
    got, err := passes.SiblingPackageImport(src, passes.SiblingConfig{
        GenPackage: "example.com/gen",
        Families:   []string{"internal"},
    })
    require.NoError(t, err)
    require.Equal(t, want, got)
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./cmd/ent-codegen-migrate/passes/ -run TestSiblingPackageImport -v
```

Expected: FAIL — `passes.SiblingPackageImport` doesn't exist yet.

- [ ] **Step 4: Implement the pass**

Create `cmd/ent-codegen-migrate/passes/sibling_package_import.go`:

The pass uses `go/ast` + `astutil` (same as existing migrator passes). Algorithm:
1. Parse the input source.
2. Walk imports; identify any whose path matches `<GenPackage>/<family>` for `family` in `Config.Families`.
3. For each such import, scan the AST for SelectorExprs `<family>.<Ident>`.
4. For each unique selector identifier, determine the entity it belongs to (the first lowercase prefix of the camelCase identifier name, e.g. `UserWhereInput` → `user`, `CreatePropertyInput` → `property`).
5. Replace each selector's `X` (the package qualifier) with `<entity><family>` and add an import `<entity><family> "<GenPackage>/<family>/<entity>"`.
6. Remove the now-unused flat-family import if no remaining references.
7. Skip identifiers that resolve via the root facade (`gen.UserWhereInput`) — those are handled by `gen` package re-exports.
8. Use `go/printer` with `Mode: printer.UseSpaces|printer.TabIndent` and tabwidth 8 to match Go's default formatting (matches the existing migrator's output format).

Edge case to handle explicitly: if an identifier doesn't have a clear lowercase-prefix-derived entity (e.g. `SharedConfig`), leave it on the flat-family import — those are cross-cutting types that stay in the parent facade.

Reference the existing migrator passes for the AST manipulation idioms — the per-entity-package import pass from PR 6 has the exact import-add + selector-rewrite pattern this needs.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./cmd/ent-codegen-migrate/passes/ -run TestSiblingPackageImport -v
```

Expected: PASS for all 6 cases.

- [ ] **Step 6: Register the pass in the migrator pipeline**

Find the existing pass registration (where PR 6 passes are listed) and add the new `SiblingPackageImport` pass after the per-entity-package pass and before the formatter:

```bash
grep -rn "RewritePackage\|passes\.\|Run.*pass" cmd/ent-codegen-migrate/ | head -10
```

Register the new pass in the same slice / registry.

- [ ] **Step 7: Run all migrator tests + integration tests**

```bash
go test ./cmd/ent-codegen-migrate/... -count=1
```

Expected: existing tests still pass (no regressions), new sibling-pkg tests pass.

- [ ] **Step 8: Commit**

```bash
git add cmd/ent-codegen-migrate/passes/sibling_package_import.go cmd/ent-codegen-migrate/passes/sibling_package_import_test.go cmd/ent-codegen-migrate/passes/passes.go
git commit -m "feat(ent-codegen-migrate): siblingPackageImport pass for B-4/5/6"
```

### Task 13: Add migrator integration test against a real consumer-style fixture

The unit tests in Task 12 cover individual selector shapes. The integration test exercises the pass end-to-end on a multi-file fixture that resembles consumer code.

**Files:**
- Create: `cmd/ent-codegen-migrate/testdata/sibling_subpkg/before/...` (representative input fixture)
- Create: `cmd/ent-codegen-migrate/testdata/sibling_subpkg/after/...` (expected output)
- Modify: `cmd/ent-codegen-migrate/integration_test.go` (add `TestRewritePackage_SiblingSubpkg` to the test matrix)

- [ ] **Step 1: Find the existing integration-test fixture pattern**

```bash
ls cmd/ent-codegen-migrate/testdata/ 2>/dev/null
grep -rn "TestRewritePackage" cmd/ent-codegen-migrate/ | head -5
```

- [ ] **Step 2: Build a 3-file fixture covering each family**

Mirror the structure of the existing PR 6 fixtures. Include:
- A file using `whereinputs.<Entity>WhereInput` in struct fields
- A file using `mutationinputs.Create<Entity>Input` in function args
- A file using `edges.With<Entity><Other>` in chained calls
- A file using both flat-family and facade-re-export styles (facade calls must NOT be rewritten)

- [ ] **Step 3: Run the integration test to verify it fails (before/after don't match)**

```bash
go test ./cmd/ent-codegen-migrate/ -run TestRewritePackage_SiblingSubpkg -v
```

Expected: FAIL — the integration test is set up correctly but the pass isn't connected to the integration runner.

- [ ] **Step 4: Connect the pass to the integration runner**

If the integration runner already runs all registered passes (likely, given PR 6's structure), no extra wiring is needed — just verify the test now passes after Task 12 Step 6's registration.

- [ ] **Step 5: Run again**

```bash
go test ./cmd/ent-codegen-migrate/ -run TestRewritePackage_SiblingSubpkg -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/ent-codegen-migrate/testdata/sibling_subpkg/ cmd/ent-codegen-migrate/integration_test.go
git commit -m "test(ent-codegen-migrate): integration test for sibling-subpkg rewrite"
```

---

## Phase F — Consumer migration (gemini worktree, extends existing `codegen-reduction-consumer-migration` branch)

The existing `codegen-reduction-consumer-migration` branch on `MatthewsREIS/gemini` is at `ae2dee5553` (3 commits, PR #3902 open). This phase adds **one new commit** to it via a normal forward push. No branch creation, no force-push.

### Task 14: Push ent + contrib branches + bump consumer pseudo-versions

**Files:**
- Modify: consumer `go.mod` / `go.sum`

- [ ] **Step 1: Push contrib branch**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
git push -u origin entgql-sibling-split
```

- [ ] **Step 2: Push ent branch (includes migrator updates)**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git push -u origin codegen-reduction-sibling-split
```

- [ ] **Step 3: Compute new pseudo-versions**

```bash
ENT_SHA=$(git -C /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake rev-parse codegen-reduction-sibling-split)
ENT_TS=$(TZ=UTC git -C /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake show -s --format=%cd --date=format-local:%Y%m%d%H%M%S "$ENT_SHA")
ENT_SHORT=$(git -C /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake rev-parse --short=12 "$ENT_SHA")
CONTRIB_SHA=$(git -C /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg rev-parse entgql-sibling-split)
CONTRIB_TS=$(TZ=UTC git -C /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg show -s --format=%cd --date=format-local:%Y%m%d%H%M%S "$CONTRIB_SHA")
CONTRIB_SHORT=$(git -C /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg rev-parse --short=12 "$CONTRIB_SHA")
echo "ent:     v0.0.0-${ENT_TS}-${ENT_SHORT}"
echo "contrib: v0.0.0-${CONTRIB_TS}-${CONTRIB_SHORT}"
```

- [ ] **Step 4: Confirm consumer is on the existing branch**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
git status
git branch --show-current
```

Expected: branch is `codegen-reduction-consumer-migration`, HEAD at `ae2dee5553`, working tree clean (no local-path overrides).

If the working tree shows local-path replaces in `go.mod`/`go.sum` (from a prior dev session), discard them with `git checkout -- go.mod go.sum` first.

- [ ] **Step 5: Update consumer go.mod replaces (in-place on existing branch)**

```bash
go mod edit \
  -replace="entgo.io/ent=github.com/MatthewsREIS/ent@v0.0.0-${ENT_TS}-${ENT_SHORT}" \
  -replace="entgo.io/contrib=github.com/codelite7/contrib@v0.0.0-${CONTRIB_TS}-${CONTRIB_SHORT}"
GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go mod tidy
```

### Task 15: Run regen + migrator against consumer (output commits to existing branch)

**Files:** all consumer `*_gen.go` and call-site `*.go` files

- [ ] **Step 1: Regenerate consumer ent**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 GOMAXPROCS=4 go run ./api-graphql/src/cmd/ent_codegen.go
```

(Adjust the actual regen entry-point path if different — read the consumer's existing regen invocation from `Taskfile.yml` or similar.)

Expected: per-entity sibling subpackages now exist on disk (`api-graphql/src/ent/gen/whereinputs/escrow/`, etc.).

- [ ] **Step 2: Run the migrator against the consumer source tree**

```bash
GOMEMLIMIT=8GiB go run /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake/cmd/ent-codegen-migrate/ \
  -gen-package ./api-graphql/src/ent/gen \
  -consumer-root ./api-graphql/src \
  -passes siblingPackageImport
```

Adjust flag names to match the migrator's actual CLI (read the existing migrator usage from prior consumer-migration commits' messages, e.g. `3e69e1088e`). If the migrator runs all registered passes by default, the `-passes` flag may not be needed.

Expected: migrator emits a non-empty diff. The diff size depends on the consumer inventory from Phase E Task 11: if pattern (i) (facade re-export) dominated, the diff is small; if pattern (ii) (direct sibling-pkg qualifier) was common, the diff is larger.

- [ ] **Step 3: Build the consumer (cold first to catch the structural issue, then warm to confirm)**

```bash
go clean -cache 2>/dev/null || true
GOMEMLIMIT=8GiB GOGC=25 GOMAXPROCS=4 go build -p 4 -a ./api-graphql/...
```

Expected: exit 0.

Common failure modes and fixes:
- **Undefined `whereinputs.<X>`**: the migrator missed a call site. Either the identifier didn't match the entity-name heuristic, or the pass needs an additional pattern. Add a unit test reproducing the missed pattern in Phase E Task 12, fix the pass, re-run from Step 2.
- **Import cycle through the gen tree**: the per-entity sibling subpkg's import set conflicts with PR 6's per-entity entity packages. The pass added an import that already exists transitively. Rework the import resolution.
- **`gen.<X>` undefined**: a facade re-export went missing during the template work. Re-do Phase C Task 9 (root `facade.tmpl` re-exports) to cover the missing identifier.

If failures aren't tractable from the migrator side, escalate — the pass may need a redesign, or the template work in Phase C may need re-thinking.

- [ ] **Step 4: Run consumer unit tests**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
task test-unit
```

Expected: matches PR C baseline (2200 tests, 17 skipped, 0 failures). Any new failures → regression — bisect against the migrator pass and the template edits.

- [ ] **Step 5: Commit migrator output to the existing consumer branch**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
git add go.mod go.sum api-graphql/src/ent/gen api-graphql/src
git commit -m "$(cat <<'COMMIT'
feat(ent): B-4/B-5/B-6 sibling-subpackage migration

Per-entity sibling subpackages (whereinputs/, mutationinputs/, gqlcollections/,
gqledges/, edges/, internal/) — call sites rewritten by cmd/ent-codegen-migrate
siblingPackageImport pass. go.mod replaces bumped to PR D HEAD on
MatthewsREIS/ent + entgql-sibling-split on codelite7/contrib.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
COMMIT
)"
```

- [ ] **Step 6: Push to existing branch (forward push, no force)**

```bash
git push origin codegen-reduction-consumer-migration
```

Expected: push succeeds. `MatthewsREIS/gemini` PR #3902 picks up the new commit automatically — no PR description update needed (the existing description already covers the scope; the new commit just extends the migration).

### Task 16: Update existing consumer PR description with B-4/5/6 note

**Files:** none — `gh pr edit` only

- [ ] **Step 1: Append B-4/5/6 context to PR #3902 body**

```bash
gh pr view 3902 --repo MatthewsREIS/gemini --json body --jq .body > /tmp/pr-3902-body.md
# Manually append a "## B-4/B-5/B-6 follow-up" section noting the 4th commit, then:
gh pr edit 3902 --repo MatthewsREIS/gemini --body-file /tmp/pr-3902-body.md
```

The new section should note:
- B-4/B-5/B-6 sibling-subpackage migration added as commit 4 on this branch (`<sha>`)
- Migrator pass that generated it: `siblingPackageImport` (added on `MatthewsREIS/ent` PR D)
- ent + contrib pseudo-versions bumped to PR D HEAD + `entgql-sibling-split` HEAD
- Bench delta vs PR C baseline: <fill in from Phase G Task 17>

---

## Phase G — Bench + sign-off

### Task 17: Cold bench on current host (same-host comparison vs PR C baseline)

**Files:** bench artifacts only

- [ ] **Step 1: Clear consumer build cache**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
go clean -cache 2>/dev/null || true  # or use GOCACHE=/tmp/fresh-cache for one-off
```

Note: clearing the 28GB cache is expensive; alternative is `GOCACHE=/var/tmp/bench-pr6/cache-$(date +%s) go build -a ...` for an isolated cache.

- [ ] **Step 2: Run cold build with full profile (same recipe as PR C profile)**

```bash
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
  > /tmp/bench-b4b5b6.stdout 2> /tmp/bench-b4b5b6.log
```

Expected: exit 0, wall < 3:06.83 (the PR C baseline on this host).

- [ ] **Step 3: Parse hotspots**

```bash
echo "=== wall + RSS ==="
grep -E "Elapsed|Maximum resident" /tmp/bench-b4b5b6.log

echo "=== top 20 slowest compiles ==="
grep "^PROF compile" /tmp/bench-b4b5b6.log | sort -k3 -rn | head -20

echo "=== sibling subpkg compile totals ==="
awk '/^PROF compile/ && ($4 ~ /\/(whereinputs|internal|edges|mutationinputs|gqlcollections|gqledges)\//) {s += $3; n++} END {printf "  Sibling subpkg compiles: %d invocations, %.1fs total CPU\n", n, s}' /tmp/bench-b4b5b6.log
```

Expected: sibling-subpkg total compile time went UP (more invocations × per-package overhead) but parallelism is unlocked, so wall time goes DOWN. Each per-entity sibling subpkg compile should be < 2s individually.

- [ ] **Step 4: Project vs baseline**

Per the PR C profile (memory [[project-codegen-reduction-b4-b5-b6-opportunities]]):
- Baseline single-package siblings: 57.1s CPU on 6 invocations
- Target post-split: ~70-80s CPU on ~800 invocations (more pkg overhead) but 4-wide parallel = ~20s critical path
- Net wall savings: 15-30s

If wall is FLAT (within ~25-35s host noise), B-4/5/6 didn't deliver — escalate the finding and decide whether to ship for LOC-organization reasons or revert.

- [ ] **Step 5: Update memory**

Update `project-codegen-reduction-bench-results.md` with a new "Post-B-4/B-5/B-6" section: wall, peak RSS, sibling-subpkg invocation count delta, verdict.

```bash
# Write the memory update
$EDITOR /home/smoothbrain/.claude/projects/-var-home-smoothbrain-dev-matthewsreis-ent/memory/project_codegen_reduction_bench_results.md
```

- [ ] **Step 6: Commit bench artifacts**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
cp /tmp/bench-b4b5b6.log "/var/tmp/bench-pr6/post-b4b5b6-$(date +%Y%m%d-%H%M%S).txt"
# memory file already committed via $EDITOR above
git add internal/bench/  # if any new jsonl was emitted
git commit -m "bench(internal/bench): B-4/B-5/B-6 measurement vs PR C baseline" || echo "no bench changes"
```

### Task 18: Open the follow-up PRs for ent + contrib (consumer PR already extended in Phase F Task 16)

**Files:** none — git operations only.

- [ ] **Step 1: Submit ent branch via git-spice**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
git checkout codegen-reduction-sibling-split
git spice branch submit --no-draft \
  --title "codegen-reduction PR D: per-entity sibling subpackages + migrator pass (B-4/5/6)" \
  --body "<paste bench-led description with Δ wall + Δ RSS from Phase G Task 17>"
```

Expected: PR created on `MatthewsREIS/ent`, base = `codegen-reduction-per-entity-and-post-bench`. This PR includes the `siblingPackageImport` migrator pass alongside the template work.

- [ ] **Step 2: Open contrib follow-up PR**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
git checkout entgql-sibling-split
gh pr create --repo codelite7/contrib --base entgql-collection-subpkg --head entgql-sibling-split \
  --title "entgql: per-entity sibling subpackages (B-4/B-6)" \
  --body "<companion to ent PR D — bench-led description>"
```

- [ ] **Step 3: Consumer PR is already updated**

Per Phase F Task 16, `MatthewsREIS/gemini` #3902 already has the B-4/5/6 commit on its existing branch (`codegen-reduction-consumer-migration`) and the PR description was extended. **No new gemini PR is opened.** The single consumer PR carries the full epic migration including all stack levers.

If `MatthewsREIS/gemini` #3902 was merged before this work landed, fall back to opening a new PR against current `main`:

```bash
gh pr create --repo MatthewsREIS/gemini --base main --head codegen-reduction-consumer-migration \
  --title "codegen-reduction follow-up: B-4/B-5/B-6 sibling subpackage migration" \
  --body "<bench-led description>"
```

But the expected path is the PR-stays-open case.

- [ ] **Step 4: Update memory**

Append PR URLs and outcomes to `project-codegen-reduction-prs-opened` memory. Note explicitly that the consumer PR is single-threaded (3 → 4 commits, same PR number) per the in-place extension policy in this plan's stack section.

---

## Risks + escalation

**The structural risk is import cycles between per-entity sibling subpackages.** If the recon phase (Task 1) shows that `whereinputs/A` ↔ `whereinputs/B` cross-references can't be cleanly broken via a parent facade, B-4 is blocked. Two escape hatches:
1. **Keep `whereinputs/` as a single package for the cyclic minority**: split only the entities that have no cross-references. Partial win.
2. **Hoist all `*WhereInput` types into the parent `whereinputs/` facade with per-entity files**: drops the package boundary entirely; preserves layout, no wall-time win for whereinputs. Use only as a fallback.

**Migrator complexity risk:** if the new import-rewrite pass conflicts with existing PR 6 passes (e.g., aliases collide), the consumer migration could regress. Mitigation: re-run the existing `task test-unit` after migration; if any new failures show up vs the PR C baseline (2200 tests, 17 skipped, 0 failures), bisect to the conflicting pass.

**Bench result risk:** if wall time is flat-within-noise (per [[feedback-bench-same-host-comparison]] threshold of ~25-35s on ~3-minute runs), B-4/5/6 didn't deliver the wall win. Decision matrix:
- LOC unchanged + memory unchanged + wall flat → revert as no-value churn.
- LOC unchanged + memory neutral + wall improved by 5-15s → ship for LOC organization + small wall win.
- LOC unchanged + memory degraded by >300 MB peak + wall flat → revert.
- LOC unchanged + memory neutral + wall improved by >20s → ship, clear win.

**Cycle-time risk per [[feedback-batch-template-edits]]:** if a single Phase C edit error blocks the regen, you don't get partial signal. The fix is **NOT** to fall back to serializing — the cycle-time waste of serializing dominates. The fix is to fix the template error and re-run. If the same template error appears repeatedly, that's a signal the cross-entity dependency model is wrong; return to Phase A and re-do the resolution map.

---

## Out of scope (explicit, do NOT pursue here)

- `graph` package (gqlgen output, 49s compile). Requires gqlgen-level changes.
- `resolvers` hand-written package (24s). Consumer code organization.
- Lever F (generic edge/field helpers) — separate plan if pursued.
- Lever N (skip Cmp on UUID, conditional string preds) — separate plan if pursued.
- Splitting `escrow/` entity package (90K LOC) — single entity, by design indivisible without Lever F.
- GC tuning sweep (GOGC=25→50) — separate quick experiment, can run independently.

---

## Definition of done

- [ ] All 6 sibling subpackages emit per-entity subpackages in fixture regen
- [ ] `go test ./entc/...` passes
- [ ] `go test ./entgql/...` passes (contrib)
- [ ] `go test ./cmd/ent-codegen-migrate/...` passes
- [ ] Consumer `go build ./api-graphql/...` succeeds with new ent + contrib pseudo-versions
- [ ] Consumer `task test-unit` matches the PR C baseline (2200 tests, 17 skipped, 0 failures)
- [ ] Cold bench on current host shows wall time reduction OR a decision to ship/revert per the risk matrix
- [ ] `project-codegen-reduction-bench-results.md` updated with post-B-4/5/6 row
- [ ] Three PRs opened (ent #D, contrib #follow-up, consumer #follow-up-or-amended) against forks only
