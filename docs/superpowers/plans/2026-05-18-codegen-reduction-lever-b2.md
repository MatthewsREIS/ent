# Codegen Reduction Lever B-2 — Pagination + Node → Per-Entity Subpackages

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In the entgql contrib extension, redirect per-entity `gql_pagination_<entity>.go` and `gql_node_<entity>.go` files from the gen root package into per-entity subpackages (`gen/<entity>/gql_pagination.go` and `gen/<entity>/gql_node.go`). Move the bulk OUT of the gen root package while keeping the consumer-facing API resolvable via type-alias re-exports in root. Predicted bench impact (per spec §6): BUILD cold wall −3% to −5%, RSS −3%, layered on top of B-1.

**Architecture:** Each entity's pagination types (`<E>Connection`, `<E>Edge`, `<E>Order`, `<E>OrderField`, pagination Paginate method) and node types (`<E>.Node()` impl) move into the entity subpackage. New `pagination_subpkg.tmpl` and `node_subpkg.tmpl` modeled on the existing `mutation_input_subpkg.tmpl` precedent (`entgql/template/mutation_input_subpkg.tmpl`, `entgql/extension.go:871`). Two new emission funcs (`generatePaginationSubpkgFile`, `generateNodeSubpkgFile`) wired into the AllTemplates hook dispatch at `extension.go:611`. Root re-exports (type aliases + value forwarders) emitted so consumer call sites like `gen.UserConnection`, `gen.UserPaginate` continue to compile — same pattern as lever B-1's facade `var X = edges.X`. Existing root-emission functions (`generatePaginationEntityFile`, `generateNodeEntityFile`) are repurposed to emit only the thin re-export shells.

**Tech Stack:** Go 1.25, entgql contrib templates (Go `text/template`), bench worktree at `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go` for consumer-scale measurement.

**Per [[feedback-no-prs-until-end-of-epic]]:** all commits stay local. The contrib worktree branch is `entgql-collection-subpkg` (3 commits ahead of origin/main, no upstream). No `git push`, no PRs at any step.

**Per [[feedback-bench-build-memory-limits]]:** every bench-worktree `go build` invocation gets `GOMEMLIMIT=8GiB GOGC=25`.

---

## File Structure

**Templates (entgql/template/):**
- Create: `pagination_subpkg.tmpl` — new template emitting pagination types + methods in the entity subpackage
- Create: `node_subpkg.tmpl` — new template emitting Node implementation in the entity subpackage
- Modify: `pagination_entity.tmpl` — strip body down to type-alias re-exports + value forwarders (root-side thin shim)
- Modify: `node_entity.tmpl` — same (thin shim that forwards Node lookup to subpkg)

**Extension wiring (entgql/):**
- Modify: `extension.go` — register the two new templates (`PaginationSubpkgTemplate`, `NodeSubpkgTemplate`), add two emission funcs (`generatePaginationSubpkgFile`, `generateNodeSubpkgFile`), wire them into the hook dispatch at lines 605-625, and extend the `keep` set used by `cleanupSplitGeneratedGoFiles` to preserve subpkg-emitted files
- Modify: `template.go` — declare and parse the two new templates (mirroring `MutationInputSubpkgTemplate` declaration at lines 80, 177)

**Bench worktree artifacts (not committed):**
- Regenerated `src/ent/gen/<entity>/gql_pagination.go` and `src/ent/gen/<entity>/gql_node.go` per entity
- Shrunk `src/ent/gen/gql_pagination_<entity>.go` and `src/ent/gen/gql_node_<entity>.go` (now thin re-export shims)
- New bench output: `/var/tmp/bench-pr6/post-b2-<timestamp>.txt`

**Memory updates:**
- Append B-2 measurement outcome to `project_codegen_reduction_bench_results.md`
- Update `project_codegen_reduction_post_bench_plan.md` to mark B-2 measured (HIT/PARTIAL/MISS)

---

## Phase A — Investigation gate (no commits)

### Task 1: Read pagination/node templates and characterize what they emit

**Files:**
- Read-only: `entgql/template/pagination_entity.tmpl`, `entgql/template/node_entity.tmpl`, `entgql/template/mutation_input_subpkg.tmpl`
- Read-only: `entgql/extension.go` around lines 946-963 (generatePaginationEntityFile), lines 1077-1097 (generateNodeEntityFile), lines 871-904 (generateMutationInputSubpkgFile precedent)
- Read-only: a sample generated `gql_pagination_<entity>.go` and `gql_node_<entity>.go` in the bench worktree (e.g., `/home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql/src/ent/gen/gql_pagination_property.go` and `gql_node_property.go`)

- [ ] **Step 1: Inventory the exported symbols in `gql_pagination_<entity>.go`**

For one large entity (Property recommended, or Escrow), grep the file for top-level decls:
```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
grep -E "^(type|func|var|const)" src/ent/gen/gql_pagination_property.go | head -40
```

Record:
- Type names defined (likely: `PropertyConnection`, `PropertyEdge`, `PropertyPageInfo`, `PropertyOrder`, `PropertyOrderField`, `PropertyOrderTermXxx`, etc.)
- Top-level functions (likely: `Paginate` method on `PropertyQuery`, helper constructors)
- Vars/consts (likely: `PropertyOrderFieldXxx` sentinels)

- [ ] **Step 2: Find where those symbols are referenced from outside the file**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
# Where is PropertyConnection referenced (excluding the file that defines it)?
grep -rn "PropertyConnection\|PropertyEdge\|PropertyOrder" src/ --include="*.go" | grep -v "src/ent/gen/gql_pagination_property.go" | head -20
```

The key question: are these symbols referenced from `src/graph` (gqlgen-generated), `src/resolvers`, or anywhere else? If yes, root MUST re-export them as type aliases or consumer code breaks.

- [ ] **Step 3: Same inventory for `gql_node_<entity>.go`**

```bash
grep -E "^(type|func|var|const)" src/ent/gen/gql_node_property.go | head -20
grep -rn "Property.*Node\|propertyImplementsNode" src/ --include="*.go" | grep -v "src/ent/gen/gql_node_property.go" | head -10
```

Node files typically expose a small surface (a `Node` method on the entity type, possibly a global node-resolver registry entry). May be simpler than pagination.

- [ ] **Step 4: Decide and record the strategy**

Three possible outcomes from the investigation:

**Outcome A — clean split possible.** Symbols defined per-entity are referenced only via `gen.<EntityName>X`-style call sites. Solution: subpkg defines the types; root emits `type PropertyConnection = property.PropertyConnection` aliases plus `var PropertyPaginate = property.Paginate` value forwarders. Same pattern as B-1's `var WithX = edges.X`. **Proceed to Phase B.**

**Outcome B — cross-package type references inside the pagination type definitions themselves.** E.g., `PropertyConnection.Edges []*PropertyEdge` where `PropertyEdge.Node *Property` — that's fine within the entity's own subpkg (Property type is local). But if `PropertyOrder` references an unrelated entity's type for an FK-driven ordering, you have a cycle. Inspect the generated file for any `<OtherEntity>X` reference. If clean, still **proceed to Phase B**.

**Outcome C — gqlgen-generated `src/graph/generated.go` directly references `gen.PropertyConnection` (or similar) and is regenerated as part of the bench's GQLGEN step.** This is the hardest case: re-exports in root are required AND the gqlgen schema-to-Go bindings must continue to resolve through them. Test it: re-export the types, regen gqlgen, check that resolvers compile. If gqlgen handles re-exported aliases cleanly, **proceed to Phase B**. If not, **stop and report BLOCKED** to the controller; we'd need to either teach gqlgen the new layout or pivot.

**Step 5: Report findings (NO commit)**

If Outcome A or B: report a one-page summary of the strategy decision (which symbols re-export, which value-forward). Continue to Task 2.

If Outcome C blocking case: report BLOCKED with concrete evidence (the specific symbols, the specific files that reference them in problematic ways). The controller decides whether to pivot.

---

## Phase B — Pagination subpackage emission

### Task 2: Add `PaginationSubpkgTemplate` to extension template registry

**Files:**
- Modify: `entgql/template.go` (template declaration around lines 80-90 and parse call around lines 175-180)

- [ ] **Step 1: Add the template declaration**

In `entgql/template.go`, find the `var (` block declaring template variables (around line 65-130). Find the `MutationInputSubpkgTemplate` declaration at line 80:

```go
	// MutationInputSubpkgTemplate generates SetInput methods in the entity sub-package to avoid
	// circular imports between the sub-package and root gen.
	MutationInputSubpkgTemplate *template.Template
```

After it, add:

```go
	// PaginationSubpkgTemplate generates pagination types and Paginate method in the entity
	// sub-package (lever B-2). Companion to PaginationEntityTemplate which now emits thin
	// re-export shims in root.
	PaginationSubpkgTemplate *template.Template
```

- [ ] **Step 2: Add the parse call**

In the same file, find the parse calls section (around line 175-186):

```go
	MutationInputSubpkgTemplate = parseEntityTemplate("template/mutation_input_subpkg.tmpl", "gql_mutation_input_subpkg")
	PaginationEntityTemplate = parseEntityTemplate("template/pagination_entity.tmpl", "gql_pagination_entity")
```

After the `PaginationEntityTemplate` parse, add:

```go
	PaginationSubpkgTemplate = parseEntityTemplate("template/pagination_subpkg.tmpl", "gql_pagination_subpkg")
```

This will fail at init if the template file doesn't exist — that's expected; Step 3 creates it.

- [ ] **Step 3: Create the `pagination_subpkg.tmpl` file**

Create `entgql/template/pagination_subpkg.tmpl`. Start with the existing `pagination_entity.tmpl` as the base. Key transformations:

```
{{/*
Copyright 2019-present Facebook Inc. All rights reserved.
This source code is licensed under the Apache 2.0 license found
in the LICENSE file in the root directory of this source tree.
*/}}

{{ define "gql_pagination_subpkg" }}

{{- /*gotype: entgo.io/contrib/entgql.paginationEntityData*/ -}}

{{ $.Config.Header }}

{{ $node := $.Node }}
package {{ $node.Package }}

{{ $gqlNodes := filterNodes $.Nodes (skipMode "type") }}
{{ $idType := gqlIDType $gqlNodes $.IDType }}
{{ $orderFields := orderFields $node }}

{{ $names := nodePaginationNames $node -}}
{{ $name := $names.Node -}}

import (
	"io"
	"strconv"
	"encoding/base64"

	"{{ $.Config.Package }}/predicate"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"entgo.io/contrib/entgql"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vmihailenco/msgpack/v5"
)

{{- /* PASTE BODY OF pagination_entity.tmpl FROM ITS BODY SECTION DOWNWARD, with these substitutions:
       - References to `<Name>X` (e.g., `PropertyConnection`) stay unqualified — they're now in the same package
       - References to `<node.Package>.X` (e.g., `property.X`) become `X` — same package
       - The import of `"{{ $.Config.Package }}/{{ $node.Package }}"` is REMOVED (it would be a self-import)
       Get the body text by reading pagination_entity.tmpl from line ~40 onward (after the import block ends). */}}
{{ end }}
```

The exact body to paste is the part of `pagination_entity.tmpl` after the closing `)` of its import block. Read it carefully and apply the substitutions noted in the placeholder comment.

**Important qualification rules when pasting:**

| In pagination_entity.tmpl (root) | In pagination_subpkg.tmpl (entity subpkg) |
|---|---|
| `{{ $node.Package }}.Order{{ $names.Order }}Asc` | `Order{{ $names.Order }}Asc` (same pkg, drop prefix) |
| `{{ $node.Package }}.Table` | `Table` (same pkg) |
| `*{{ $name }}` | `*{{ $name }}` — no change, types are now local |
| `{{ $name }}{Edges: ...}` | `{{ $name }}{Edges: ...}` — no change |
| `func (n *{{ $name }})` | `func (n *{{ $name }})` — no change |
| `Paginate(...)` calls into entgql lib | no change — entgql still imported |

- [ ] **Step 4: Add `generatePaginationSubpkgFile` to extension.go**

In `entgql/extension.go`, find `generateMutationInputSubpkgFile` at line 871. After it (or near `generatePaginationEntityFile` at line 946 for proximity), add:

```go
// generatePaginationSubpkgFile generates pagination types + Paginate method in the
// entity's sub-package (e.g., src/ent/gen/property/gql_pagination.go). The root
// gql_pagination_<entity>.go file is reduced to a thin re-export shim by the
// updated generatePaginationEntityFile.
func (e *Extension) generatePaginationSubpkgFile(g *gen.Graph, n *gen.Type) error {
	subPkgDir := filepath.Join(g.Target, n.Package())
	if _, err := os.Stat(subPkgDir); os.IsNotExist(err) {
		return nil // sub-package doesn't exist, skip
	}

	path := filepath.Join(subPkgDir, "gql_pagination.go")

	tmpl := PaginationSubpkgTemplate
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		*gen.Graph
		Node *gen.Type
	}{g, n}); err != nil {
		return fmt.Errorf("entgql: execute pagination_subpkg template for %s: %w", n.Name, err)
	}

	content, err := e.processImports(path, buf.Bytes())
	if err != nil {
		return fmt.Errorf("entgql: format pagination_subpkg for %s: %w", n.Name, err)
	}

	return os.WriteFile(path, content, 0644)
}
```

- [ ] **Step 5: Wire `generatePaginationSubpkgFile` into the hook dispatch**

In `extension.go`, find the hook dispatch block around lines 605-625 that schedules `generatePaginationEntityFile`. Currently it looks like:

```go
	for _, n := range nodes {
		fns = append(fns,
			func() error { return e.generatePaginationEntityFile(&staged, n) },
			func() error { return e.generateNodeEntityFile(&staged, n) },
		)
	}
```

Replace with (adds the subpkg emission for pagination; node will be added in Task 3):

```go
	for _, n := range nodes {
		fns = append(fns,
			func() error { return e.generatePaginationEntityFile(&staged, n) },
			func() error { return e.generatePaginationSubpkgFile(&staged, n) },
			func() error { return e.generateNodeEntityFile(&staged, n) },
		)
	}
```

There may also be a second loop around line 770 — check and add the same line there if present:

```go
	for _, n := range nodes {
		fns = append(fns, func() error { return e.generatePaginationEntityFile(g, n) })
		// add the subpkg companion:
		fns = append(fns, func() error { return e.generatePaginationSubpkgFile(g, n) })
	}
```

- [ ] **Step 6: Reduce `generatePaginationEntityFile` to emit a thin re-export shim**

Now modify the root pagination template (`pagination_entity.tmpl`) to emit thin re-export aliases. Replace the body (everything between `{{ define ... }}` and `{{ end }}`) with:

```
{{ define "gql_pagination_entity" }}

{{- /*gotype: entgo.io/contrib/entgql.paginationEntityData*/ -}}

{{ $pkg := base $.Config.Package }}
{{ $.Config.Header }}

package {{ $pkg }}

{{ $node := $.Node }}
{{ $names := nodePaginationNames $node }}
{{ $name := $names.Node }}

import (
	"{{ $.Config.Package }}/{{ $node.Package }}"
)

// Type aliases — body lives in {{ $node.Package }}/gql_pagination.go (lever B-2 split).
type (
	{{ $name }}PageInfo    = {{ $node.Package }}.{{ $name }}PageInfo
	{{ $name }}Edge        = {{ $node.Package }}.{{ $name }}Edge
	{{ $name }}Connection  = {{ $node.Package }}.{{ $name }}Connection
	{{ $name }}Order       = {{ $node.Package }}.{{ $name }}Order
	{{ $name }}OrderField  = {{ $node.Package }}.{{ $name }}OrderField
)

{{ end }}
```

Add additional aliases for any other top-level types the original template defined (Order direction sentinels, etc.) — use Task 1's symbol inventory to list them all.

If the original template defined top-level VARS (e.g., `Default<Entity>Order = ...`) or FUNCS (e.g., `<Entity>OrderFieldByX()`), add value forwarders:

```
var (
	Default{{ $name }}Order = {{ $node.Package }}.Default{{ $name }}Order
)

var {{ $name }}OrderFieldByCreatedAt = {{ $node.Package }}.{{ $name }}OrderFieldByCreatedAt
```

- [ ] **Step 7: Regen bench worktree to validate**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
export GOMEMLIMIT=8GiB GOGC=25
task generate-go-no-cache 2>&1 | tail -30
```

Expected: regen succeeds. If template parse errors, fix the template syntax. If "duplicate declaration" errors during build, your re-export aliases conflict with something — investigate.

```bash
ls src/ent/gen/property/gql_pagination.go 2>&1
wc -l src/ent/gen/gql_pagination_property.go src/ent/gen/property/gql_pagination.go
```

Expected: subpkg file exists, root file is now small (~30-50 lines of aliases), subpkg file is large (where the body moved to).

Build:
```bash
GOMEMLIMIT=8GiB GOGC=25 go build ./api-graphql/... 2>&1 | tail -20
```

Expected: clean build. If undefined-symbol errors in src/graph or src/resolvers, the re-export aliases missed something — add them.

- [ ] **Step 8: Verify in contrib worktree**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
pwd && git rev-parse --abbrev-ref HEAD
# Expected: /var/home/.../contrib/.claude/worktrees/entgql-collection-subpkg on branch entgql-collection-subpkg
go test ./entgql/... 2>&1 | tail -20
```

Expected: PASS for entgql tests. The split-mode tests in `extension_split_test.go` and related files should still cover the basic pattern.

- [ ] **Step 9: Commit**

Two-step. First:
```bash
git add entgql/template.go entgql/template/pagination_subpkg.tmpl entgql/template/pagination_entity.tmpl entgql/extension.go
```

Then:
```bash
git commit -m "$(cat <<'EOF'
feat(entgql): emit gql_pagination to per-entity subpackage

Lever B-2 first half: pagination types (PropertyConnection, PropertyEdge,
PropertyOrder, PropertyPaginate, etc.) now emit into
gen/<entity>/gql_pagination.go via the new PaginationSubpkgTemplate.

The root gql_pagination_<entity>.go file is reduced to thin re-export
aliases — consumer code referencing gen.PropertyConnection continues
to resolve through the type alias. Mirrors the mutation_input_subpkg
pattern (extension.go:871) and B-1's facade var-forwarder approach in
the ent core.

Node files split lands in a follow-up commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Add `NodeSubpkgTemplate` and split node files

**Files:**
- Modify: `entgql/template.go`
- Create: `entgql/template/node_subpkg.tmpl`
- Modify: `entgql/template/node_entity.tmpl`
- Modify: `entgql/extension.go` (add generateNodeSubpkgFile, wire into dispatch)

This task mirrors Task 2 for the node template family. The work is the same shape: declare and parse a new template, create the subpkg template body, add a new emission function, wire it into the hook dispatch, reduce the root template to thin re-exports.

- [ ] **Step 1: Add the `NodeSubpkgTemplate` declaration in `entgql/template.go`**

Find `PaginationSubpkgTemplate` (just added in Task 2). After it:

```go
	// NodeSubpkgTemplate generates Node method + impl in the entity sub-package (lever B-2).
	// Companion to NodeEntityTemplate which is reduced to a thin re-export shim in root.
	NodeSubpkgTemplate *template.Template
```

After the `PaginationSubpkgTemplate = parseEntityTemplate(...)` line, add:

```go
	NodeSubpkgTemplate = parseEntityTemplate("template/node_subpkg.tmpl", "gql_node_subpkg")
```

- [ ] **Step 2: Create `entgql/template/node_subpkg.tmpl`**

Start from `node_entity.tmpl` (read it in full) and apply the same transformations as Task 2's pagination_subpkg.tmpl:
- `package {{ $pkg }}` → `package {{ $node.Package }}`
- Remove the self-import `"{{ $.Config.Package }}/{{ $node.Package }}"`
- Strip the `{{ $node.Package }}.` qualifier from intra-pkg references

Use the qualification table from Task 2 Step 3 as a reference; apply the same rules to node template content.

- [ ] **Step 3: Add `generateNodeSubpkgFile` to extension.go**

After `generatePaginationSubpkgFile` (just added in Task 2), add:

```go
// generateNodeSubpkgFile generates the Node implementation in the entity's
// sub-package (e.g., src/ent/gen/property/gql_node.go). The root
// gql_node_<entity>.go is reduced to a thin re-export shim.
func (e *Extension) generateNodeSubpkgFile(g *gen.Graph, n *gen.Type) error {
	subPkgDir := filepath.Join(g.Target, n.Package())
	if _, err := os.Stat(subPkgDir); os.IsNotExist(err) {
		return nil
	}

	path := filepath.Join(subPkgDir, "gql_node.go")

	tmpl := NodeSubpkgTemplate
	_, hasCollection := e.hasTemplate(CollectionTemplate)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		*gen.Graph
		Node                  *gen.Type
		HasCollectionTemplate bool
	}{g, n, hasCollection}); err != nil {
		return fmt.Errorf("entgql: execute node_subpkg template for %s: %w", n.Name, err)
	}

	content, err := e.processImports(path, buf.Bytes())
	if err != nil {
		return fmt.Errorf("entgql: format node_subpkg for %s: %w", n.Name, err)
	}

	return os.WriteFile(path, content, 0644)
}
```

Note: pass the `HasCollectionTemplate` parameter — `node_entity.tmpl` uses it (see `generateNodeEntityFile` at extension.go:1077).

- [ ] **Step 4: Wire into the hook dispatch**

In the same block(s) where pagination was wired in Task 2 Step 5, add the node subpkg emission alongside:

```go
	for _, n := range nodes {
		fns = append(fns,
			func() error { return e.generatePaginationEntityFile(&staged, n) },
			func() error { return e.generatePaginationSubpkgFile(&staged, n) },
			func() error { return e.generateNodeEntityFile(&staged, n) },
			func() error { return e.generateNodeSubpkgFile(&staged, n) },
		)
	}
```

And in the second loop around line 844 if it exists.

- [ ] **Step 5: Reduce `node_entity.tmpl` to thin re-exports**

Based on Task 1's symbol inventory, write the re-export shim. Likely small — node templates usually just have an Init function and a Node method on the entity type. Re-exports look like:

```
{{ define "gql_node_entity" }}

{{- /*gotype: entgo.io/contrib/entgql.nodeEntityData*/ -}}

{{ $pkg := base $.Config.Package }}
{{ $.Config.Header }}

package {{ $pkg }}

{{ $node := $.Node }}

import (
	"{{ $.Config.Package }}/{{ $node.Package }}"
)

// Node-handler re-exports — body lives in {{ $node.Package }}/gql_node.go (lever B-2 split).
// Specific aliases depend on Task 1 symbol inventory.

{{ end }}
```

If `gql_node_<entity>.go` exports types/funcs needed by external callers, alias them all. The Node interface method `Node()` is likely on the entity struct itself, so it's automatically resolvable via the type alias `type Property = property.Property` already in the facade.

- [ ] **Step 6: Regen + build + test**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
export GOMEMLIMIT=8GiB GOGC=25
task generate-go-no-cache 2>&1 | tail -30
GOMEMLIMIT=8GiB GOGC=25 go build ./api-graphql/... 2>&1 | tail -20
```

```bash
cd /var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg
pwd && git rev-parse --abbrev-ref HEAD
go test ./entgql/... 2>&1 | tail -10
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add entgql/template.go entgql/template/node_subpkg.tmpl entgql/template/node_entity.tmpl entgql/extension.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entgql): emit gql_node to per-entity subpackage

Lever B-2 second half: Node implementation moves from
gen/gql_node_<entity>.go into gen/<entity>/gql_node.go via the new
NodeSubpkgTemplate. Root gql_node_<entity>.go reduced to thin re-export
shim. Companion to the pagination split in the previous commit.

Lever B-2 implementation complete; awaits bench measurement.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase C — Bench validation

### Task 4: Run consumer bench measurement

**Files:** none in source; produces `/var/tmp/bench-pr6/post-b2-<timestamp>.txt`.

- [ ] **Step 1: Verify bench worktree state**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
git status
```

Working tree may have changes from prior regen (post-B-1 + B-2 changes). The bench's GENERATE step refreshes from scratch.

```bash
grep "replace entgo.io/contrib" go.mod
```

Verify the contrib replace points to `/var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg`. If it points elsewhere, the bench won't pick up B-2 — STOP and report BLOCKED.

- [ ] **Step 2: Run the bench script**

Run from THIS controller (not from a subagent — past lesson: subagent-started bench processes die when the subagent returns):

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go
/tmp/bench-pr6-post-uncapped.sh post-b2
```

Use `run_in_background: true` if dispatched via the Bash tool. Wall expected: ~12-15 min. Output goes to `/var/tmp/bench-pr6/post-b2-<timestamp>.txt`.

If the BUILD cold step fails with consumer-code errors in `./api-graphql/...`, B-2 introduced a regression — capture the first 20 lines and STOP. Don't hand-fix consumer code (out of scope per spec).

- [ ] **Step 3: Compute deltas vs post-B-1 baseline**

```bash
LATEST=$(ls -t /var/tmp/bench-pr6/post-b2-*.txt | head -1)
BASELINE=/var/tmp/bench-pr6/post-b1-20260518-100614.txt
echo "Latest:   $LATEST"
echo "Baseline: $BASELINE"
```

Extract Wall clock and Peak memory for each section. The bench output uses this format (per the post-b1 file):

```
  Wall clock:       2:48.37
  CPU time:         2146.91s user, 438.92s sys (1535%)
  Peak memory:      6.4 GB
```

Compute Δ wall and Δ RSS for the 6 sections (generate/gqlgen/build × cold/warm).

Also measure root-package LOC:
```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
find src/ent/gen -maxdepth 1 -name "*.go" | xargs wc -l 2>/dev/null | tail -1
```

Expected post-B-2: root drops further from B-1's 328K — pagination + node bodies move out. Magnitude depends on how much pagination/node contributed to root pre-B-2 (likely 80-120K combined per the LOC inventory).

---

## Phase D — Decision gate

### Task 5: Categorize outcome and report to user

**Files:** memory updates only.

- [ ] **Step 1: Verdict**

Compare measured BUILD cold wall vs B-1 baseline:

| Outcome | Measured BUILD cold wall vs B-1 | Action |
|---|---|---|
| **HIT** | −3% to −5% (or better) | Update memory, recommend B-3 |
| **PARTIAL** | 0% to −3% | Update memory; flag that sibling parallelism is partial; recommend B-3 with caveats |
| **MISS** | 0% or worse | STOP. Run action-graph profile to find new long-pole. Report. Do NOT auto-launch B-3. |

- [ ] **Step 2: Update memory**

Append a "## Post-B-2 results (YYYY-MM-DD)" section to `/home/smoothbrain/.claude/projects/-var-home-smoothbrain-dev-matthewsreis-ent/memory/project_codegen_reduction_bench_results.md` with the measured delta table and verdict.

Update `/home/smoothbrain/.claude/projects/-var-home-smoothbrain-dev-matthewsreis-ent/memory/project_codegen_reduction_post_bench_plan.md` to mark B-2 as MEASURED with HIT/PARTIAL/MISS.

- [ ] **Step 3: If MISS, run action-graph profile**

```bash
cd /home/smoothbrain/dev/matthewsreis/worktrees/bench-pr6/service-api-go/api-graphql
export GOCACHE="/tmp/profile-gocache-b2-$$" && mkdir -p "$GOCACHE"
GOMEMLIMIT=8GiB GOFLAGS=-buildvcs=false /usr/bin/time -v go build "-gcflags=github.com/MatthewsREIS/gemini/...=-N -l" -debug-actiongraph=/tmp/build-actiongraph-b2.json ./api-graphql/... 2>&1 | tee /tmp/build-profile-b2.log
rm -rf "$GOCACHE"

jq -r '[.[] | select(.Mode=="build" and .TimeStart != null and .TimeDone != null and (.Package | test("MatthewsREIS"))) | {pkg: .Package, dur: (((.TimeDone[0:19] + "Z") | fromdateiso8601) - ((.TimeStart[0:19] + "Z") | fromdateiso8601))}] | sort_by(-.dur) | .[0:15] | .[] | "\(.dur)s\t\(.pkg)"' /tmp/build-actiongraph-b2.json
```

Compare the new long-pole picture to the B-1 profile (`/tmp/build-actiongraph-b1.json`). If gen root + edges + per-entity subpackages got smaller but src/graph is still 60+ seconds, that's expected (gqlgen long-pole, §9 out of scope).

- [ ] **Step 4: Report to user**

Whatever the outcome, write a clear message: measured deltas, verdict, recommendation. Stop the plan here. Do NOT auto-launch B-3 — the user explicitly wants the gate per the "improvements without numbers don't ship" rule.

---

## Self-review notes

**Spec coverage:** §4.1 (gql_pagination_* and gql_node_* moved to per-entity sub-packages) covered by Tasks 2 and 3. §5 step 2 (B-2 staged + bench gate + decision branch) covered by Tasks 4 and 5. §6 B-2 prediction (−3% to −5% wall, −3% RSS) used as gate criterion in Task 5. Spec §3 non-goal "no API-contract changes" honored via type-alias re-exports.

**Out of scope verified absent:** No lever F work. No B-3 (whereinputs/mutationinputs/edges/collections — cross-entity siblings). No ent core changes (those landed in B-1). No migration-tool work.

**Risk acknowledgment:** Task 1 investigation may surface a blocker (Outcome C — gqlgen-generated `src/graph` has direct references to per-entity pagination types that don't resolve through re-exports). If so, Task 1 escalates and the plan halts. The complexity here is higher than the spec §4.1 implied; the investigation is a real gate, not a formality.

**Type-consistency check:** `PaginationSubpkgTemplate`/`NodeSubpkgTemplate` declared with `*template.Template` matching `MutationInputSubpkgTemplate` (template.go:80). Function names `generatePaginationSubpkgFile`/`generateNodeSubpkgFile` match the precedent `generateMutationInputSubpkgFile`. File names `gql_pagination.go`/`gql_node.go` in subpkg match the existing `gql_mutation_input.go` precedent. Template names `gql_pagination_subpkg`/`gql_node_subpkg` match the existing `gql_mutation_input_subpkg`.

**Test coverage gap:** The entgql contrib has unit tests (`extension_split_test.go` etc.) but no consumer-scale integration tests. Most B-2 validation comes from the bench measurement in Task 4. This is the same coverage gap as B-1 had; flagged in the spec.

**Open items for the executing engineer:**
- The qualification audit in Task 2 Step 3 (pasting pagination body with subpkg-package substitutions) is the riskiest step — analogous to the B-1 Task 4 paste audit. Read every type reference before committing.
- If Task 1 reveals Outcome C, escalate immediately — don't try to patch around blocking gqlgen integration issues.
- The `cleanupSplitGeneratedGoFiles` function (extension.go:678) already lists `gql_pagination*.go` and `gql_node*.go` patterns. After B-2, the existing root files like `gql_pagination_property.go` continue to exist (as thin re-export shims) and stay in the `keep` set. No cleanup changes needed.
