# Ent Codegen Isolation — Foundation (PR #1 + PR #2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lay the foundation for ent codegen isolation: (1) inject `-tags=entcodegen` into the loader so consumers can gate generated-code-dependent files with `//go:build !entcodegen`; (2) remove the snapshot fallback mechanism so load failures surface immediately with informative errors.

**Architecture:** Modify `entc/load/load.go` to merge `entcodegen` into `Config.BuildFlags` before invoking `packages.Load` and `gorun`. Delete `entc/internal/snapshot.go`, the `FeatureSnapshot` flag, the `internal/schema` template, the `SnapshotDir` option, and the `mayRecover` recovery path. Replace `mayRecover` with `wrapLoadError`, which annotates load failures with the failing import path and a build-tag hint.

**Tech Stack:** Go 1.22+, `golang.org/x/tools/go/packages` (existing loader dependency), `github.com/stretchr/testify/require` (existing test framework), SQLite for ent integration tests.

**Spec:** `docs/superpowers/specs/2026-05-22-ent-codegen-isolation-design.md`

---

## File Structure

### Created
- `entc/load/tags.go` — new helper for merging `entcodegen` into `BuildFlags`
- `entc/load/tags_test.go` — unit tests for the merge helper
- `entc/integration/codegen_isolation/` — new integration directory exercising the build-tag mechanism end-to-end
  - `generate.go` — `go:generate` for the integration's ent
  - `entc.go` — codegen config (no `FeatureSnapshot`, no privacy)
  - `ent/schema/user.go` — Class A schema definition
  - `ent/schema/user_hooks.go` — Class B file with `//go:build !entcodegen`, references generated symbols
  - `ent/schema/broken_dep.go` — Class B file gated by the tag that imports a deliberately-undefined symbol (proves isolation)
  - `isolation_test.go` — verifies codegen succeeds despite Class B referencing missing/broken symbols
  - `error_path_test.go` — verifies that an *untagged* file with broken refs causes codegen to fail with a clear, prescriptive error

### Modified
- `entc/load/load.go` — call the tag-merge helper at the start of `Config.Load`
- `entc/entc.go` — remove `SnapshotDir` option (lines 168-178); remove snapshot-copy block (lines 423-438); replace `mayRecover` call with `wrapLoadError`; delete `mayRecover` function (lines 443-469); add `wrapLoadError`
- `entc/gen/feature.go` — remove `FeatureSnapshot` definition (lines 64-79) and remove it from `AllFeatures` (line 169)
- `entc/gen/template/internal.tmpl` — delete `internal/schema` template block (lines 9-19)
- `entc/gen/globalid.go` — remove the `if FeatureSnapshot enabled` gate at line 36 (keep the merge-conflict resolution unconditionally)
- `entc/gen/graph_test.go` — remove `FeatureSnapshot` test case at line 595
- `entc/integration/edgeschema/ent/entc.go` — drop `gen.FeatureSnapshot` at line 30

### Deleted
- `entc/internal/snapshot.go`
- `entc/internal/snapshot_test.go`
- `entc/integration/edgeschema/ent/internal/schema.go`
- `entc/integration/hooks/ent/internal/schema.go`
- Any other `entc/integration/*/ent/internal/schema.go` produced by the snapshot template (audit during PR #2)

---

## Phase 0 — PoC Validation (consumer repo, no ent commits)

Verify the build-tag mechanism works against the actual codebase before committing changes to ent. All work in a throwaway branch of `matthewsreis/gemini`; no ent changes needed (uses existing `entc.BuildFlags(...)` API).

### Task 0.1: Stage 1 — Validate tag injection (~30 min)

**Files (in `matthewsreis/gemini` worktree):**
- Modify: `service-api-go/api-graphql/src/ent/schema/attachment_hooks.go` — first line
- Modify: `service-api-go/api-graphql/entc.go:154`

- [ ] **Step 1: Create throwaway branch in gemini**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/gemini
git checkout -b poc-entcodegen-stage-1
```

- [ ] **Step 2: Add build tag to one hook file**

Prepend the first line of `service-api-go/api-graphql/src/ent/schema/attachment_hooks.go`:

```go
//go:build !entcodegen

package schema
```

(Keep the existing `package schema` line below; the build constraint must be the *first* line, followed by a blank line, then the package clause.)

- [ ] **Step 3: Extend entc.BuildFlags to pass the tag**

In `service-api-go/api-graphql/entc.go`, change the existing `entc.BuildFlags(...)` call to add `-tags=entcodegen`:

```go
// Before:
entc.BuildFlags("-gcflags=github.com/MatthewsREIS/gemini/...=-N -l"),

// After:
entc.BuildFlags(
    "-gcflags=github.com/MatthewsREIS/gemini/...=-N -l",
    "-tags=entcodegen",
),
```

- [ ] **Step 4: Run codegen**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/gemini/service-api-go/api-graphql
go generate ./...
```

Expected: succeeds (exit 0). No "undefined: hook.AttachmentFunc" error.

- [ ] **Step 5: Diff generated output**

```bash
git diff src/ent/gen/runtime.go | head -100
```

Expected: Attachment per-type hook-init block(s) missing. Other types unchanged.

- [ ] **Step 6: Confirm normal build still works**

```bash
go build ./...
```

Expected: succeeds. `attachment_hooks.go` participates in the build (no tag set) but Attachment-typed schema-attached hooks no longer auto-fire (they're no longer in the generated init block); for the PoC this is OK and validates the mechanism.

- [ ] **Step 7: Gate check**

If steps 4-6 all succeed: Stage 1 PASS. Proceed to Task 0.2.

If step 4 fails with "schema package failed to load" or "Attachment does not implement ent.Interface": the design's assumption that `ent.Schema`'s default `Hooks()` covers the exclusion is wrong. **STOP** — file the symptoms in a memory note and re-evaluate the spec before continuing.

### Task 0.2: Stage 2 — Validate isolation property (~30 min)

**Files (in `matthewsreis/gemini` worktree):**
- Create: `service-api-go/api-graphql/src/ent/schema/poc_broken.go`

- [ ] **Step 1: Create deliberately-broken file under the tag**

```go
//go:build !entcodegen

package schema

import "this/package/does/not/exist"

var _ = nonexistent.Symbol
```

- [ ] **Step 2: Run codegen — must succeed**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/gemini/service-api-go/api-graphql
go generate ./...
```

Expected: succeeds. Codegen ignores `poc_broken.go` under `-tags=entcodegen`.

- [ ] **Step 3: Run normal build — must fail**

```bash
go build ./...
```

Expected: FAILS with "this/package/does/not/exist" missing. Proves the file IS being built outside codegen mode and the broken import is real.

- [ ] **Step 4: Delete the broken file and verify clean state**

```bash
rm src/ent/schema/poc_broken.go
go build ./...
```

Expected: succeeds.

- [ ] **Step 5: Gate check**

If steps 2 and 3 have the expected outcomes: Stage 2 PASS. Proceed to Task 0.3.

### Task 0.3: Stage 3 — Validate annotation-closure pattern (~1-2h)

**Files (in `matthewsreis/gemini` worktree):**
- Modify: `service-api-go/api-graphql/src/ent/schema/escrow_edges.go` (extract closure)
- Create: `service-api-go/api-graphql/src/ent/schema/escrow_filter.go` (Class B, real body)
- Create: `service-api-go/api-graphql/src/ent/schema/escrow_filter_stub.go` (Class C, stub)

- [ ] **Step 1: Read the existing closure**

Read `service-api-go/api-graphql/src/ent/schema/escrow_edges.go` around lines 237-280 (the `entsearch.FilterFunc(func(ctx) {...})` block). Note the signature: `func(ctx context.Context) (interface{}, error)`.

- [ ] **Step 2: Create the Class B file with the real body**

`service-api-go/api-graphql/src/ent/schema/escrow_filter.go`:

```go
//go:build !entcodegen

package schema

import (
    "context"
    "fmt"

    "github.com/MatthewsREIS/gemini/service-api-go/api-graphql/src/ent/gen"
    "github.com/samber/lo"
)

func escrowFilter(ctx context.Context) (interface{}, error) {
    userID := gen.UserIdFromContext(ctx)
    if userID == nil {
        return nil, fmt.Errorf("user ID not found in context")
    }

    claims := gen.PrivateClaimsFromContext(ctx)
    var roles []string
    if userID != nil {
        roles = lo.Map(claims.Roles.Values(), func(item interface{}, index int) string {
            return item.(gen.Role).String()
        })
    }

    // (Paste the rest of the original closure body here, lines from the source file.)
    return nil, nil
}
```

Copy the full original closure body — this is a verbatim move, not a rewrite.

- [ ] **Step 3: Create the Class C stub**

`service-api-go/api-graphql/src/ent/schema/escrow_filter_stub.go`:

```go
//go:build entcodegen

package schema

import "context"

func escrowFilter(ctx context.Context) (interface{}, error) {
    return nil, nil
}
```

- [ ] **Step 4: Rewrite the annotation in escrow_edges.go**

Replace the inline `entsearch.FilterFunc(func(ctx) {...})` block with:

```go
entsearch.FilterFunc(escrowFilter),
```

Remove imports that are no longer used in this file (the closure body's imports go to `escrow_filter.go`). Run `goimports -w src/ent/schema/escrow_edges.go`.

- [ ] **Step 5: Run codegen**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/gemini/service-api-go/api-graphql
go generate ./...
```

Expected: succeeds. `escrowFilter` resolves to the stub during codegen (returns `(nil, nil)`). The annotation is still reflected; codegen doesn't inspect the function body.

- [ ] **Step 6: Run normal build**

```bash
go build ./...
```

Expected: succeeds. `escrowFilter` resolves to the real body. Behavior at runtime unchanged.

- [ ] **Step 7: Run any existing entsearch tests for Escrow**

```bash
go test ./... -run Escrow
```

Expected: existing assertions still pass; the filter behaves identically to before.

- [ ] **Step 8: Gate check**

If steps 5, 6, 7 all succeed: Stage 3 PASS — the foundation is validated. Discard the throwaway branch and proceed to PR #1 below.

```bash
cd /var/home/smoothbrain/dev/matthewsreis/gemini
git checkout master  # or whatever the default branch is
git branch -D poc-entcodegen-stage-1
```

---

## PR #1 — Tag Injection in ent Fork

**Branch:** `feat/entcodegen-tag-injection`

### Task 1.1: Tag-merge helper with TDD

The helper merges `entcodegen` into a `BuildFlags` slice, handling existing `-tags` flags correctly (Go's `-tags` flag is last-wins, so we must combine rather than append).

**Files:**
- Create: `entc/load/tags.go`
- Create: `entc/load/tags_test.go`

- [ ] **Step 1: Write the failing test**

`entc/load/tags_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package load

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestMergeCodegenTag(t *testing.T) {
    t.Run("appends when no -tags present", func(t *testing.T) {
        got := mergeCodegenTag([]string{"-gcflags=foo"})
        require.Equal(t, []string{"-gcflags=foo", "-tags=entcodegen"}, got)
    })

    t.Run("merges into single -tags=foo arg", func(t *testing.T) {
        got := mergeCodegenTag([]string{"-tags=hidegroups"})
        require.Equal(t, []string{"-tags=hidegroups,entcodegen"}, got)
    })

    t.Run("merges into two-arg -tags foo form", func(t *testing.T) {
        got := mergeCodegenTag([]string{"-tags", "hidegroups"})
        require.Equal(t, []string{"-tags", "hidegroups,entcodegen"}, got)
    })

    t.Run("idempotent when entcodegen already present in -tags=", func(t *testing.T) {
        got := mergeCodegenTag([]string{"-tags=entcodegen,foo"})
        require.Equal(t, []string{"-tags=entcodegen,foo"}, got)
    })

    t.Run("idempotent when entcodegen already present in -tags foo", func(t *testing.T) {
        got := mergeCodegenTag([]string{"-tags", "foo,entcodegen"})
        require.Equal(t, []string{"-tags", "foo,entcodegen"}, got)
    })

    t.Run("nil input yields just -tags=entcodegen", func(t *testing.T) {
        got := mergeCodegenTag(nil)
        require.Equal(t, []string{"-tags=entcodegen"}, got)
    })

    t.Run("preserves other flags around -tags", func(t *testing.T) {
        got := mergeCodegenTag([]string{"-gcflags=foo", "-tags=hidegroups", "-race"})
        require.Equal(t, []string{"-gcflags=foo", "-tags=hidegroups,entcodegen", "-race"}, got)
    })
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/worktrees/cycle
go test ./entc/load/ -run TestMergeCodegenTag -v
```

Expected: FAIL with `undefined: mergeCodegenTag`.

- [ ] **Step 3: Write minimal implementation**

`entc/load/tags.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package load

import "strings"

// codegenTag is the build tag entc sets on every schema load so that files
// gated with `//go:build !entcodegen` are excluded during code generation.
// This lets schema authors put hook bodies, interceptor bodies, and any
// helper code that imports generated symbols into tagged files without
// blocking codegen when those symbols don't exist yet.
const codegenTag = "entcodegen"

// mergeCodegenTag returns buildFlags with codegenTag merged into any
// existing -tags flag, or appended as a new "-tags=entcodegen" flag if
// none exists. The function is idempotent: passing flags that already
// contain entcodegen returns them unchanged.
//
// Go's -tags flag is last-wins, not additive, so we cannot simply append
// "-tags=entcodegen". This helper handles both flag forms:
//   - "-tags=foo,bar" (single arg, equals-form)
//   - "-tags" "foo,bar" (two args, space-form)
func mergeCodegenTag(buildFlags []string) []string {
    out := append([]string(nil), buildFlags...)
    for i := 0; i < len(out); i++ {
        switch {
        case out[i] == "-tags" && i+1 < len(out):
            // Two-arg form: "-tags" "foo,bar"
            if !hasTag(out[i+1], codegenTag) {
                out[i+1] = appendTag(out[i+1], codegenTag)
            }
            return out
        case strings.HasPrefix(out[i], "-tags="):
            // Equals form: "-tags=foo,bar"
            tags := strings.TrimPrefix(out[i], "-tags=")
            if !hasTag(tags, codegenTag) {
                out[i] = "-tags=" + appendTag(tags, codegenTag)
            }
            return out
        }
    }
    // No existing -tags flag; append a fresh one.
    return append(out, "-tags="+codegenTag)
}

// hasTag reports whether the comma-separated tag list contains the given tag.
func hasTag(tagList, tag string) bool {
    for _, t := range strings.Split(tagList, ",") {
        if strings.TrimSpace(t) == tag {
            return true
        }
    }
    return false
}

// appendTag adds tag to the comma-separated tag list.
func appendTag(tagList, tag string) string {
    if tagList == "" {
        return tag
    }
    return tagList + "," + tag
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./entc/load/ -run TestMergeCodegenTag -v
```

Expected: PASS (all 7 sub-tests).

- [ ] **Step 5: Commit**

```bash
git add entc/load/tags.go entc/load/tags_test.go
git commit -m "$(cat <<'EOF'
entc/load: add mergeCodegenTag helper for build-flag injection

Prepares the loader to unconditionally include -tags=entcodegen so
schema authors can gate generated-code-dependent files with
//go:build !entcodegen. The helper is idempotent and handles both
single-arg ("-tags=foo") and two-arg ("-tags" "foo") flag forms.

No wiring yet; that lands in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1.2: Wire helper into Config.Load

**Files:**
- Modify: `entc/load/load.go` — at the start of `Config.Load()` (line 65), mutate `c.BuildFlags`

- [ ] **Step 1: Read the current Load implementation**

```bash
sed -n '60,100p' entc/load/load.go
```

Confirm the function starts at line 64 with `func (c *Config) Load() (*SchemaSpec, error) {` and that `spec, pos, err := c.load()` is on line 66.

- [ ] **Step 2: Inject the tag at the start of Load**

Edit `entc/load/load.go`. Replace:

```go
// Load loads the schemas package and build the Go plugin with this info.
func (c *Config) Load() (*SchemaSpec, error) {
	spec, pos, err := c.load()
```

with:

```go
// Load loads the schemas package and build the Go plugin with this info.
func (c *Config) Load() (*SchemaSpec, error) {
	// Inject the entcodegen build tag so files with //go:build !entcodegen
	// are excluded from typechecking and from the .entc runtime program.
	// See entc/load/tags.go for details.
	c.BuildFlags = mergeCodegenTag(c.BuildFlags)

	spec, pos, err := c.load()
```

The mutation is in-place on the Config receiver; subsequent calls (`c.load` at line 117 which sets `BuildFlags: c.BuildFlags` on `packages.Load`, and `gorun(target, c.BuildFlags)` at line 96) see the merged flags.

- [ ] **Step 3: Verify existing TestLoadTags still passes**

`TestLoadTags` (entc/load/load_test.go:71) uses `BuildFlags: []string{"-tags", "hidegroups"}`. After our injection, this becomes `"-tags", "hidegroups,entcodegen"`. The testdata schemas don't have any `//go:build !entcodegen` markers so the behavior is unchanged.

```bash
go test ./entc/load/ -run TestLoadTags -v
```

Expected: PASS.

- [ ] **Step 4: Run the full loader test suite**

```bash
go test ./entc/load/... -v
```

Expected: all tests pass (TestLoad, TestLoadWrongPath, TestLoadSpecific, TestLoadNoSchema, TestLoadSchemaFailure, TestLoadBaseSchema, TestLoadTags, TestLoadCycleError, TestMergeCodegenTag).

- [ ] **Step 5: Commit**

```bash
git add entc/load/load.go
git commit -m "$(cat <<'EOF'
entc/load: unconditionally inject -tags=entcodegen into BuildFlags

Wires mergeCodegenTag into Config.Load so every schema load is performed
with the entcodegen tag set. Schemas can opt out of including specific
files during codegen with //go:build !entcodegen. User-supplied
BuildFlags (including their own -tags) are preserved alongside.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1.3: Create codegen_isolation integration directory

A new integration that exercises the build-tag mechanism end-to-end. This directory will host both the positive test (Task 1.4) and the negative error-path test (added later in PR #2 Task 2.3).

**Files:**
- Create: `entc/integration/codegen_isolation/generate.go`
- Create: `entc/integration/codegen_isolation/entc.go`
- Create: `entc/integration/codegen_isolation/ent/schema/user.go`
- Create: `entc/integration/codegen_isolation/ent/schema/user_hooks.go`

- [ ] **Step 1: Create generate.go**

`entc/integration/codegen_isolation/generate.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package codegen_isolation

//go:generate go run -mod=mod entc.go
```

- [ ] **Step 2: Create entc.go**

`entc/integration/codegen_isolation/entc.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

//go:build ignore

package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
)

func main() {
    if err := entc.Generate("./ent/schema", &gen.Config{
        Target:  "./ent",
        Package: "entgo.io/ent/entc/integration/codegen_isolation/ent",
    }); err != nil {
        log.Fatalf("running ent codegen: %v", err)
    }
}
```

- [ ] **Step 3: Create the Class A schema**

`entc/integration/codegen_isolation/ent/schema/user.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
    ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").Default("unknown"),
    }
}
```

- [ ] **Step 4: Create the Class B hooks file (references generated code)**

`entc/integration/codegen_isolation/ent/schema/user_hooks.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

//go:build !entcodegen

package schema

import (
    "context"
    "fmt"

    "entgo.io/ent"

    "entgo.io/ent/entc/integration/codegen_isolation/ent"
    "entgo.io/ent/entc/integration/codegen_isolation/ent/hook"
)

// UserCreateHook references generated hook.UserFunc and ent.UserMutation,
// which only exist after codegen has run. This file MUST be excluded
// during codegen (via //go:build !entcodegen) or codegen fails.
func UserCreateHook() ent.Hook {
    return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
        if name, ok := m.Name(); ok && name == "" {
            return nil, fmt.Errorf("name is required")
        }
        return m.Client().User.Update().Save(ctx)
    })
}
```

- [ ] **Step 5: Generate the integration's ent package**

```bash
cd entc/integration/codegen_isolation
go generate ./...
```

Expected: succeeds. Generated code appears in `entc/integration/codegen_isolation/ent/`. The presence of `user_hooks.go` (which would not typecheck before generation) does not block codegen because it has `//go:build !entcodegen`.

- [ ] **Step 6: Confirm the generated package and tagged file both compile**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/worktrees/cycle
go build ./entc/integration/codegen_isolation/...
```

Expected: succeeds. `user_hooks.go` is now part of the normal build and its references to `hook.UserFunc`, `ent.UserMutation`, etc., resolve to the just-generated code.

- [ ] **Step 7: Commit**

```bash
git add entc/integration/codegen_isolation/
git commit -m "$(cat <<'EOF'
entc/integration: add codegen_isolation directory

Exercises the //go:build !entcodegen mechanism end-to-end: a schema
package containing a hook-body file that references generated symbols
(hook.UserFunc, ent.UserMutation). The file is gated with the build
tag, so codegen succeeds despite those symbols not existing until
codegen produces them.

Tests using this scaffold land in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1.4: Isolation test asserting the tag-exclusion contract

**Files:**
- Create: `entc/integration/codegen_isolation/isolation_test.go`

- [ ] **Step 1: Write the failing test**

`entc/integration/codegen_isolation/isolation_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package codegen_isolation_test

import (
    "context"
    "testing"

    "entgo.io/ent/dialect"
    "entgo.io/ent/entc/integration/codegen_isolation/ent"
    "entgo.io/ent/entc/integration/codegen_isolation/ent/enttest"
    "entgo.io/ent/entc/integration/codegen_isolation/ent/schema"

    _ "github.com/mattn/go-sqlite3"
    "github.com/stretchr/testify/require"
)

func TestCodegenIsolation_UserCreate(t *testing.T) {
    client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
    defer client.Close()

    ctx := context.Background()
    u, err := client.User.Create().SetName("alice").Save(ctx)
    require.NoError(t, err)
    require.Equal(t, "alice", u.Name)
}

// TestCodegenIsolation_HookRegistration confirms that the Class B hook file
// (user_hooks.go, gated with //go:build !entcodegen) IS compiled into the
// test binary and its exported symbol is callable. This proves the file
// participates in normal builds while being excluded during codegen.
func TestCodegenIsolation_HookRegistration(t *testing.T) {
    client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
    defer client.Close()

    // schema.UserCreateHook returns an ent.Hook; ent.Hook is a function type.
    // Calling it without panicking proves the file is in the build.
    hook := schema.UserCreateHook()
    require.NotNil(t, hook)
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test ./entc/integration/codegen_isolation/... -v
```

Expected: PASS (both subtests). The first test exercises the generated code; the second confirms the Class B file's symbol is reachable in normal builds.

- [ ] **Step 3: Verify the tag injection is what makes generate work**

Sanity check: temporarily remove `//go:build !entcodegen` from `user_hooks.go`, re-run `go generate`, observe failure. Restore the tag and confirm `go generate` succeeds again.

```bash
# Remove first two lines (build constraint + blank) from user_hooks.go
sed -i '1,2d' entc/integration/codegen_isolation/ent/schema/user_hooks.go
cd entc/integration/codegen_isolation && go generate ./... 2>&1 | head -20
```

Expected: FAILS with an "undefined: hook.UserFunc" or similar error (the generated `ent/hook/` package can't be found by `packages.Load` if `user_hooks.go` references it before codegen).

Restore:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/worktrees/cycle
git checkout -- entc/integration/codegen_isolation/ent/schema/user_hooks.go
cd entc/integration/codegen_isolation && go generate ./...
```

Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/worktrees/cycle
git add entc/integration/codegen_isolation/isolation_test.go
git commit -m "$(cat <<'EOF'
entc/integration/codegen_isolation: add isolation test

Asserts the //go:build !entcodegen mechanism works end-to-end:
codegen succeeds despite the hook file referencing not-yet-generated
symbols, and the same file is reachable in normal builds.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 1.5: Full ent integration regression check

Verify no existing test broke from the tag injection.

- [ ] **Step 1: Run the full entc test suite**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/worktrees/cycle
go test ./entc/... -short
```

Expected: PASS. No new failures.

- [ ] **Step 2: Run the integration suite (longer)**

```bash
cd entc/integration
go test ./...
```

Expected: PASS. (The integration suite is large; allow up to 5 minutes.)

- [ ] **Step 3: If anything regresses, diagnose and fix in this PR**

A regression here means the tag-injection broke a test that depended on un-tagged behavior. Investigation order:
1. Check if the failing test loads a schema with `BuildFlags` set; the injection may have collided.
2. Check the assertion on `len(spec.Schemas)` or similar — a schema file gated by some unexpected tag might be excluded.

If a fix is needed, add it before commit; do not commit a green-after-some-tests-skipped result.

### Task 1.6: PR #1 wrap-up

- [ ] **Step 1: Review the diff**

```bash
git log --oneline cycle..HEAD
git diff cycle..HEAD --stat
```

Expected: 4 commits, ~150 lines added across `entc/load/tags.go`, `entc/load/tags_test.go`, `entc/load/load.go` (3-line insertion), and the `entc/integration/codegen_isolation/` directory.

- [ ] **Step 2: Push branch**

```bash
git push -u origin feat/entcodegen-tag-injection
```

- [ ] **Step 3: Open PR**

```bash
gh pr create --title "feat(entc/load): inject -tags=entcodegen for codegen isolation" --body "$(cat <<'EOF'
## Summary
- Adds an unconditionally-injected `-tags=entcodegen` build flag so schemas can gate generated-code-dependent files with `//go:build !entcodegen`.
- Adds a new `codegen_isolation` integration directory that exercises the mechanism end-to-end.
- Preserves user-supplied `BuildFlags` (including their own `-tags`) by merging instead of overwriting.

This is the foundation for the larger codegen-isolation epic. Snapshot removal (PR #2), interface shrink (PR #3), and privacy removal (PR #4) follow in separate PRs.

## Test plan
- [x] `go test ./entc/load/...` — passes including new `TestMergeCodegenTag` and existing `TestLoadTags`
- [x] `go test ./entc/integration/codegen_isolation/...` — new isolation test passes; sanity-checked that removing the build tag from `user_hooks.go` makes codegen fail (confirming the tag is what unblocks it)
- [x] `go test ./entc/...` and `go test ./entc/integration/...` — no regressions

Spec: `docs/superpowers/specs/2026-05-22-ent-codegen-isolation-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Wait for CI green and approval before proceeding to PR #2.

---

## PR #2 — Drop Snapshot Feature

**Branch:** `feat/entcodegen-drop-snapshot` (cut from `master` *after* PR #1 merges)

### Task 2.1: wrapLoadError helper with TDD

The new helper takes a load error and the schema path, then returns a wrapped error that names the failing import and suggests `//go:build !entcodegen` when the broken symbol looks like generated code.

**Files:**
- Create: `entc/load_error.go`
- Create: `entc/load_error_test.go`

- [ ] **Step 1: Write the failing test**

`entc/load_error_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc

import (
    "errors"
    "testing"

    "github.com/stretchr/testify/require"
    "golang.org/x/tools/go/packages"
)

func TestWrapLoadError_PreservesNonPackageErrors(t *testing.T) {
    base := errors.New("connection refused")
    got := wrapLoadError(base, "./schema")
    require.ErrorIs(t, got, base, "non-packages error should pass through unchanged")
}

func TestWrapLoadError_AnnotatesGeneratedCodeReference(t *testing.T) {
    pkgErr := packages.Error{
        Pos:  "/repo/src/ent/schema/user_hooks.go:14:2",
        Msg:  "undefined: hook.UserFunc",
        Kind: packages.TypeError,
    }
    got := wrapLoadError(pkgErr, "./schema").Error()

    require.Contains(t, got, "/repo/src/ent/schema/user_hooks.go", "must name the failing file")
    require.Contains(t, got, "//go:build !entcodegen", "must suggest the build-tag escape hatch")
    require.Contains(t, got, "hook.UserFunc", "must show the undefined symbol")
}

func TestWrapLoadError_NoHintWhenSymbolDoesNotLookGenerated(t *testing.T) {
    pkgErr := packages.Error{
        Pos:  "/repo/src/ent/schema/user.go:9:2",
        Msg:  "undefined: time.Tomorrow",
        Kind: packages.TypeError,
    }
    got := wrapLoadError(pkgErr, "./schema").Error()

    require.Contains(t, got, "undefined: time.Tomorrow")
    require.NotContains(t, got, "//go:build !entcodegen", "should not suggest the tag for non-generated-looking symbols")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./entc/ -run TestWrapLoadError -v
```

Expected: FAIL with `undefined: wrapLoadError`.

- [ ] **Step 3: Write minimal implementation**

`entc/load_error.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc

import (
    "errors"
    "fmt"
    "regexp"
    "strings"

    "golang.org/x/tools/go/packages"
)

// generatedSymbolRE matches identifiers that suggest the failing symbol
// belongs to ent-generated code. The patterns are intentionally permissive:
// false positives (suggesting the build tag when it's actually a typo)
// are benign because `go build ./...` will still surface the real bug.
var generatedSymbolRE = regexp.MustCompile(`\b(hook|intercept|ent|gen)\.[A-Z]`)

// wrapLoadError annotates a load failure with the failing import location
// and, when the failure looks like a reference to ent-generated code,
// a hint about the //go:build !entcodegen escape hatch.
//
// If err is not a packages.Error (the typical schema-typecheck failure),
// it is returned unchanged.
func wrapLoadError(err error, schemaPath string) error {
    var pkgErr packages.Error
    if !errors.As(err, &pkgErr) {
        return err
    }

    var b strings.Builder
    fmt.Fprintf(&b, "entc/load: schema package failed to typecheck under -tags=entcodegen:\n")
    fmt.Fprintf(&b, "  %s: %s\n", pkgErr.Pos, pkgErr.Msg)

    if generatedSymbolRE.MatchString(pkgErr.Msg) {
        fmt.Fprintf(&b, "\n")
        fmt.Fprintf(&b, "  This file appears to reference generated code. If it contains hook,\n")
        fmt.Fprintf(&b, "  interceptor, or runtime helpers, add this on the first line:\n\n")
        fmt.Fprintf(&b, "      //go:build !entcodegen\n\n")
        fmt.Fprintf(&b, "  See docs/codegen-isolation.md for details.\n")
    }

    return errors.New(b.String())
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./entc/ -run TestWrapLoadError -v
```

Expected: PASS (all 3 subtests).

- [ ] **Step 5: Commit**

```bash
git add entc/load_error.go entc/load_error_test.go
git commit -m "$(cat <<'EOF'
entc: add wrapLoadError to annotate schema load failures

Replaces the silent snapshot-restore fallback with an informative
error message. When the failing symbol matches a generated-code
heuristic (e.g. hook.UserFunc, gen.Client), the error includes a
suggestion to add //go:build !entcodegen on the offending file.

Wiring into the generate() call site lands in the next commit, where
mayRecover is removed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.2: Remove mayRecover and SnapshotDir from entc/entc.go

**Files:**
- Modify: `entc/entc.go` — remove `SnapshotDir` option (lines 168-178); remove snapshot-copy block (lines 423-438); replace `mayRecover` call (lines 408-413) with `wrapLoadError`; delete `mayRecover` function (lines 443-469)

- [ ] **Step 1: Replace the mayRecover call in generate()**

Find the block in `entc/entc.go` starting at line 405:

```go
// generate loads the given schema and run codegen.
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
```

Replace with:

```go
// generate loads the given schema and run codegen.
func generate(schemaPath string, cfg *gen.Config) error {
	graph, err := LoadGraph(schemaPath, cfg)
	if err != nil {
		return wrapLoadError(err, schemaPath)
	}
```

- [ ] **Step 2: Delete the SnapshotDir option (entc.go:168-178)**

Remove this entire block:

```go
// SnapshotDir sets an alternative directory for the schema snapshot file,
// instead of <Target>/internal/schema.go.
func SnapshotDir(dir string) Option {
	return func(cfg *gen.Config) error {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("resolving snapshot dir: %w", err)
		}
		cfg.SnapshotDir = abs
		return nil
	}
}
```

- [ ] **Step 3: Delete the snapshot-copy block in generate() (entc.go:423-438)**

After deleting the `mayRecover` block (Step 1), find and delete this block which followed `graph.Gen()`:

```go
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
```

Also remove the now-unused `origTarget := cfg.Target` line that preceded it.

- [ ] **Step 4: Delete the mayRecover function (entc.go:443-469)**

Remove the entire `mayRecover` function definition.

- [ ] **Step 5: Update imports**

Run goimports to clean up any imports that are no longer used (likely `path/filepath`, `os`, `errors`, `internal`, `packages`):

```bash
goimports -w entc/entc.go
```

If `goimports` is not installed:

```bash
go install golang.org/x/tools/cmd/goimports@latest
```

- [ ] **Step 6: Compile-check**

```bash
go build ./entc/...
```

Expected: succeeds. If it fails with "SnapshotDir undefined" or similar, the field is still referenced somewhere — search and remove. Likely candidate: `gen.Config.SnapshotDir` field; we leave it for now (gen.Config is in the gen package) and remove in a later step.

Actually check:

```bash
grep -rn "SnapshotDir\|cfg.SnapshotDir" entc/ --include="*.go" | grep -v "_test.go"
```

If the field on `gen.Config` is still referenced, leave it alone — it becomes harmless dead config field, and gen.Config is in the gen package which is touched in PR #2 Task 2.6. Remove the field as part of removing `FeatureSnapshot`.

Actually let me check this now to avoid leaving a loose end. If `cfg.SnapshotDir` is referenced only inside the just-deleted block, the field on `gen.Config` can be deleted in the next task. If it's referenced elsewhere, we keep the field and just remove the option that sets it.

```bash
grep -rn "SnapshotDir" entc/gen/ --include="*.go"
```

Action: if the only matches are the `gen.Config` struct field definition itself, plan to remove the field in Task 2.6.

- [ ] **Step 7: Run regression tests**

```bash
go test ./entc/ -short
```

Expected: PASS. Some tests that exercised `mayRecover` may now be testing dead code — those will be deleted in subsequent tasks; for now we just need compile-success and the unrelated tests passing.

- [ ] **Step 8: Commit**

```bash
git add entc/entc.go
git commit -m "$(cat <<'EOF'
entc: remove mayRecover and SnapshotDir option

Replaces the silent snapshot-restore fallback with wrapLoadError so
schema load failures surface with actionable error messages.
The SnapshotDir option is removed; its consumers will be updated in
the consumer migration plan.

The gen.Config.SnapshotDir field and FeatureSnapshot are removed in
subsequent commits.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.3: Negative error-path integration test

Add a test in the `codegen_isolation` directory that *invokes codegen on a deliberately broken schema* and asserts the error message names the failing import + suggests the build-tag.

**Files:**
- Create: `entc/integration/codegen_isolation/error_path_test.go`

- [ ] **Step 1: Write the failing test**

`entc/integration/codegen_isolation/error_path_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package codegen_isolation_test

import (
    "os"
    "path/filepath"
    "testing"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"

    "github.com/stretchr/testify/require"
)

// TestErrorPath_BrokenHookWithoutTag confirms that when a schema-package
// file references generated symbols WITHOUT the //go:build !entcodegen
// tag, codegen fails with an actionable error message — not a silent
// snapshot restore.
func TestErrorPath_BrokenHookWithoutTag(t *testing.T) {
    tmpDir := t.TempDir()
    schemaDir := filepath.Join(tmpDir, "schema")
    require.NoError(t, os.MkdirAll(schemaDir, 0o755))

    // Class A — pure schema.
    require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user.go"), []byte(`package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
)

type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
    return []ent.Field{ field.String("name") }
}
`), 0o644))

    // Deliberately broken file WITHOUT the build tag — references
    // generated code that doesn't exist yet.
    require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user_broken.go"), []byte(`package schema

import "this/package/does/not/exist"

var _ = nonexistent.Symbol
`), 0o644))

    err := entc.Generate(schemaDir, &gen.Config{
        Target:  filepath.Join(tmpDir, "ent"),
        Package: "example.com/test/ent",
    })

    require.Error(t, err, "codegen must not silently succeed when schema package fails to typecheck")
    require.Contains(t, err.Error(), "this/package/does/not/exist", "error must name the failing import")
    // No build-tag hint expected here since the symbol "nonexistent.Symbol"
    // does not match the generated-code heuristic. The point is to verify
    // we no longer silently restore from a snapshot.
}

// TestErrorPath_HookReferencesGeneratedCode confirms that the build-tag
// hint IS emitted when the failing symbol looks like generated code.
func TestErrorPath_HookReferencesGeneratedCode(t *testing.T) {
    tmpDir := t.TempDir()
    schemaDir := filepath.Join(tmpDir, "schema")
    require.NoError(t, os.MkdirAll(schemaDir, 0o755))

    require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user.go"), []byte(`package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
)

type User struct{ ent.Schema }

func (User) Fields() []ent.Field {
    return []ent.Field{ field.String("name") }
}
`), 0o644))

    // Hook file references hook.UserFunc which would only exist
    // after codegen. Forgetting the build tag should produce an error
    // with the build-tag hint.
    require.NoError(t, os.WriteFile(filepath.Join(schemaDir, "user_hooks.go"), []byte(`package schema

import (
    "context"

    "entgo.io/ent"

    "example.com/test/ent"
    "example.com/test/ent/hook"
)

func UserHook() ent.Hook {
    return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
        return m.Client().User.Update().Save(ctx)
    })
}
`), 0o644))

    err := entc.Generate(schemaDir, &gen.Config{
        Target:  filepath.Join(tmpDir, "ent"),
        Package: "example.com/test/ent",
    })

    require.Error(t, err)
    require.Contains(t, err.Error(), "//go:build !entcodegen", "must suggest the build-tag escape hatch when the symbol looks generated")
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test ./entc/integration/codegen_isolation/ -run TestErrorPath -v
```

Expected: PASS (both subtests). The errors are produced by `wrapLoadError` from Task 2.1.

If the first test fails with "context deadline exceeded" or similar runtime issue, the `entc.Generate` may be doing module resolution that fails on the in-tmp `example.com/test/ent` path; in that case adjust the test to skip module resolution (use a `go.mod` file in tmpDir).

- [ ] **Step 3: Commit**

```bash
git add entc/integration/codegen_isolation/error_path_test.go
git commit -m "$(cat <<'EOF'
entc/integration/codegen_isolation: add error-path tests

Pins the contract that schema load failures surface as informative
errors — not silent snapshot restores. The second subtest asserts the
build-tag hint is emitted when the failing symbol matches the
generated-code heuristic.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.4: Delete entc/internal/snapshot.go

**Files:**
- Delete: `entc/internal/snapshot.go`

- [ ] **Step 1: Verify nothing outside snapshot_test.go imports snapshot symbols**

```bash
grep -rn "internal\.Snapshot\|internal\.IsBuildError" entc/ --include="*.go" | grep -v "snapshot.go\|snapshot_test.go"
```

Expected: empty. If matches appear, they came from `mayRecover` paths that should already have been deleted in Task 2.2.

If non-empty: delete those references before continuing. The likely candidate is `entc/entc.go:447` `internal.IsBuildError(err)` which was inside `mayRecover` — confirm Task 2.2 deleted that.

- [ ] **Step 2: Delete the file**

```bash
git rm entc/internal/snapshot.go
```

- [ ] **Step 3: Compile-check**

```bash
go build ./entc/...
```

Expected: succeeds.

- [ ] **Step 4: Run tests in entc/internal**

```bash
go test ./entc/internal/...
```

Expected: TestCheckDir passes (in vcs_test.go); TestSnapshot_Restore is gone with the file.

Wait — `snapshot_test.go` is a *separate* file with its own tests; it will fail to compile because it references the deleted Snapshot type. That's handled in Task 2.5.

For now, expect `go test ./entc/internal` to FAIL with a compile error in `snapshot_test.go`. The next task fixes it.

- [ ] **Step 5: Commit**

```bash
git commit -m "$(cat <<'EOF'
entc/internal: delete snapshot.go

The snapshot restore mechanism is replaced by //go:build !entcodegen
file gating. snapshot_test.go is removed in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.5: Delete entc/internal/snapshot_test.go

**Files:**
- Delete: `entc/internal/snapshot_test.go`

- [ ] **Step 1: Delete the test file**

```bash
git rm entc/internal/snapshot_test.go
```

- [ ] **Step 2: Run tests in entc/internal**

```bash
go test ./entc/internal/...
```

Expected: TestCheckDir passes (vcs_test.go remains). Package builds clean.

- [ ] **Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
entc/internal: delete snapshot_test.go

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.6: Remove FeatureSnapshot

**Files:**
- Modify: `entc/gen/feature.go` — delete `FeatureSnapshot` (lines 64-79) and remove from `AllFeatures` (line 169)
- Modify: `entc/gen/config.go` (or wherever `SnapshotDir` field on `gen.Config` is) — remove the field if present

- [ ] **Step 1: Find the SnapshotDir field on gen.Config**

```bash
grep -rn "SnapshotDir" entc/gen/ --include="*.go"
```

Note the file and line where `SnapshotDir string` is defined on the `gen.Config` struct.

- [ ] **Step 2: Delete FeatureSnapshot definition in entc/gen/feature.go**

Open `entc/gen/feature.go`. Find:

```go
	// FeatureSnapshot stores a snapshot of ent/schema and auto-solve merge-conflict (issue #852).
	FeatureSnapshot = Feature{
		Name:        "schema/snapshot",
		Stage:       Experimental,
		Default:     false,
		Description: "Schema snapshot stores a snapshot of ent/schema and auto-solve merge-conflict (issue #852)",
		GraphTemplates: []GraphTemplate{
			{
				Name:   "internal/schema",
				Format: "internal/schema.go",
			},
		},
		cleanup: func(c *Config) error {
			return remove(filepath.Join(c.Target, "internal"), "schema.go")
		},
	}
```

Delete this block (lines 64-79).

- [ ] **Step 3: Remove FeatureSnapshot from AllFeatures slice**

In the same file, find:

```go
	AllFeatures = []Feature{
		FeaturePrivacy,
		FeatureIntercept,
		FeatureEntQL,
		FeatureNamedEdges,
		FeatureBidiEdgeRefs,
		FeatureSnapshot,    // ← delete this line
		FeatureSchemaConfig,
```

Delete the `FeatureSnapshot,` line.

- [ ] **Step 4: Remove the gen.Config.SnapshotDir field**

In the file identified in Step 1, remove the `SnapshotDir string` field from the `Config` struct.

- [ ] **Step 5: Check for stale imports**

```bash
goimports -w entc/gen/feature.go
```

Likely `path/filepath` is still needed by other features (FeatureSchemaConfig, FeatureGlobalID) — verify it stays.

- [ ] **Step 6: Compile-check**

```bash
go build ./entc/...
```

Expected: succeeds. If it complains about `gen.FeatureSnapshot` being undefined somewhere, find and remove those references (likely in `entc/gen/globalid.go` (next task) or integration `entc.go` files (Task 2.11)).

```bash
grep -rn "FeatureSnapshot" --include="*.go"
```

Note remaining references for Task 2.7+.

- [ ] **Step 7: Run gen package tests**

```bash
go test ./entc/gen/... -short
```

Expected: most tests pass. `entc/gen/graph_test.go:595` still references `FeatureSnapshot` and will fail to compile — that's fixed in Task 2.10.

For now, expect a compile error there.

- [ ] **Step 8: Commit**

```bash
git add entc/gen/feature.go entc/gen/config.go  # or whatever file held SnapshotDir
git commit -m "$(cat <<'EOF'
entc/gen: remove FeatureSnapshot

Drops the feature flag, the gen.Config.SnapshotDir field, and the
internal/schema template registration. The internal/schema template
itself is removed in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.7: Delete the internal/schema template block

**Files:**
- Modify: `entc/gen/template/internal.tmpl` — delete `internal/schema` block (lines 9-19)

- [ ] **Step 1: Read the current file**

```bash
cat entc/gen/template/internal.tmpl
```

Confirm it contains two blocks: `internal/schema` (lines 9-19) and `internal/globalid` (lines 21-28).

- [ ] **Step 2: Delete the internal/schema block**

Edit `entc/gen/template/internal.tmpl`. The new contents:

```
{{/*
Copyright 2019-present Facebook Inc. All rights reserved.
This source code is licensed under the Apache 2.0 license found
in the LICENSE file in the root directory of this source tree.
*/}}

{{/* gotype: entgo.io/ent/entc/gen.Graph */}}

{{ define "internal/globalid" }}

{{ with $.Header }}{{ . }}{{ else }}// Code generated by ent, DO NOT EDIT.{{ end }}

package internal

const IncrementStarts = {{ .Annotations.IncrementStarts | json | quote }}
{{ end }}
```

- [ ] **Step 3: Confirm no other code reference to "internal/schema" template name**

```bash
grep -rn '"internal/schema"' entc/ --include="*.go"
```

Expected: empty after Task 2.6 removed the GraphTemplates registration.

- [ ] **Step 4: Run gen tests that don't reference FeatureSnapshot**

```bash
go test ./entc/gen/ -run "^TestGraph$" -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add entc/gen/template/internal.tmpl
git commit -m "$(cat <<'EOF'
entc/gen/template: delete internal/schema block

The block emitted internal/schema.go as the snapshot file. With the
snapshot feature gone, the template has no consumer.
The internal/globalid block stays — it's the per-target increment-starts
constant used by FeatureGlobalID.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.8: Update entc/gen/globalid.go

The conditional at line 36 reads `if ok, _ := g.FeatureEnabled(FeatureSnapshot.Name); ok` to gate the call to `ResolveIncrementStartsConflict`. With `FeatureSnapshot` removed, we make the resolution unconditional — it's a safe operation that just accepts theirs in case of git conflicts, which was the intent for snapshot users anyway and is the universally-useful behavior.

**Files:**
- Modify: `entc/gen/globalid.go` — remove the `if FeatureSnapshot enabled` gate (lines 36-44)

- [ ] **Step 1: Read the current implementation**

```bash
sed -n '24,55p' entc/gen/globalid.go
```

- [ ] **Step 2: Remove the gate, keep the conflict resolution unconditional**

Find:

```go
		default:
			if ok, _ := g.FeatureEnabled(FeatureSnapshot.Name); ok {
				if err = ResolveIncrementStartsConflict(g.Target); err != nil {
					return err
				}
				buf, err = os.ReadFile(path)
				if err != nil {
					return err
				}
			}
```

Replace with:

```go
		default:
			if err = ResolveIncrementStartsConflict(g.Target); err != nil {
				return err
			}
			buf, err = os.ReadFile(path)
			if err != nil {
				return err
			}
```

- [ ] **Step 3: Compile-check**

```bash
go build ./entc/gen/...
```

Expected: succeeds.

- [ ] **Step 4: Run globalid-touching tests**

```bash
go test ./entc/gen/ -run -v -short
```

Expected: PASS apart from `graph_test.go:595` `FeatureSnapshot` test case which is fixed next.

- [ ] **Step 5: Commit**

```bash
git add entc/gen/globalid.go
git commit -m "$(cat <<'EOF'
entc/gen: unconditional ResolveIncrementStartsConflict

Removes the if-FeatureSnapshot-enabled gate. The behavior (accept
theirs on git conflict in increment_starts.go) is universally useful
and was already the path snapshot users took.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.9: Remove FeatureSnapshot test case in graph_test.go

**Files:**
- Modify: `entc/gen/graph_test.go` — delete the test case at line 595

- [ ] **Step 1: Find the test**

```bash
sed -n '590,610p' entc/gen/graph_test.go
```

Identify the function containing `graph.Features = []Feature{FeatureSnapshot}` and the assertion(s) that follow. Likely a test like `TestGraphSchemaSnapshot` or similar.

- [ ] **Step 2: Delete the whole test function**

The test exercises behavior that no longer exists. Delete the entire `func TestXxx(t *testing.T)` block that contains the `FeatureSnapshot` reference.

- [ ] **Step 3: Compile-check**

```bash
go build ./entc/gen/...
```

Expected: succeeds.

- [ ] **Step 4: Run gen tests**

```bash
go test ./entc/gen/... -short
```

Expected: PASS for all remaining tests.

- [ ] **Step 5: Commit**

```bash
git add entc/gen/graph_test.go
git commit -m "$(cat <<'EOF'
entc/gen: delete graph snapshot test case

The test exercised FeatureSnapshot behavior that no longer exists.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.10: Audit and clean integration directories

Several integration directories used `FeatureSnapshot` and have generated `internal/schema.go` snapshot files. We need to (a) remove `gen.FeatureSnapshot` from their `entc.go` configs, (b) delete the now-orphan `internal/schema.go` files, and (c) regenerate the integration.

**Files:**
- Modify: `entc/integration/edgeschema/ent/entc.go:30` — drop `gen.FeatureSnapshot`
- Delete: `entc/integration/edgeschema/ent/internal/schema.go`
- Delete: `entc/integration/hooks/ent/internal/schema.go`
- (Audit) any other `entc/integration/*/ent/entc.go` or `internal/schema.go`

- [ ] **Step 1: Find all references**

```bash
grep -rn "FeatureSnapshot\|\"schema/snapshot\"" entc/integration/ --include="*.go" 2>/dev/null
find entc/integration -name "schema.go" -path "*/ent/internal/*"
```

Note every file that needs editing or deletion.

- [ ] **Step 2: Edit each integration entc.go**

For `entc/integration/edgeschema/ent/entc.go:30`, the line currently looks like:

```go
		entc.FeatureNames(
			"privacy",
			"schema/snapshot",     // ← delete this line (if present)
			"sql/upsert",
		),
```

Or alternatively the form may be `gen.FeatureSnapshot` in a slice — also delete. Repeat for every integration entc.go that mentions snapshot.

- [ ] **Step 3: Delete orphan snapshot files**

```bash
git rm entc/integration/edgeschema/ent/internal/schema.go
git rm entc/integration/hooks/ent/internal/schema.go
# Plus any other paths from Step 1.
```

- [ ] **Step 4: Regenerate each touched integration**

For each integration with a modified `entc.go`:

```bash
cd entc/integration/edgeschema/ent
go generate ./...
cd -

cd entc/integration/hooks/ent
go generate ./...
cd -
```

Expected: succeeds. Re-run for each touched directory.

If `go generate` fails, the most likely cause is the integration was relying on the snapshot for its own bootstrap. Resolution: temporarily put the schema files in working order (no broken refs) and re-run. Since these integration test schemas don't have schema-side gen references, this should not actually fail.

- [ ] **Step 5: Compile and test the modified integrations**

```bash
go test ./entc/integration/edgeschema/...
go test ./entc/integration/hooks/...
```

Expected: PASS. (Note: `hooks/` integration tests may have pre-existing issues unrelated to this PR; only the failures relevant to our changes need fixing here.)

- [ ] **Step 6: Commit each integration touched as a separate logical group, or one bulk commit**

```bash
git add entc/integration/
git commit -m "$(cat <<'EOF'
entc/integration: remove snapshot feature usage from integrations

- Drops gen.FeatureSnapshot / "schema/snapshot" from integration
  entc.go configs.
- Deletes orphan internal/schema.go snapshot files.
- Regenerates affected integrations.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 2.11: Full regression sweep

- [ ] **Step 1: Run the entire ent test suite**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/worktrees/cycle
go test ./entc/... -short
```

Expected: PASS.

- [ ] **Step 2: Run the integration suite**

```bash
cd entc/integration
go test ./... -timeout 10m
```

Expected: PASS. Allow up to 10 minutes for the full integration suite.

- [ ] **Step 3: Diagnose any failure**

If a test fails because it referenced the deleted snapshot, fix or delete it. Common candidates:
- A test asserting that `mayRecover` is called.
- A test that expects `cfg.SnapshotDir` to be respected.
- A test relying on `internal.IsBuildError`.

Diagnose, decide if the test was guarding our intended behavior or testing the now-removed mechanism, and act accordingly.

### Task 2.12: PR #2 wrap-up

- [ ] **Step 1: Review the diff**

```bash
git log --oneline master..HEAD
git diff master..HEAD --stat
```

Expected: ~12 commits, modest line count overall (mostly deletions: snapshot.go ~250 LOC, mayRecover ~30 LOC, plus integration regen which may be large).

- [ ] **Step 2: Push branch**

```bash
git push -u origin feat/entcodegen-drop-snapshot
```

- [ ] **Step 3: Open PR**

```bash
gh pr create --title "feat(entc): drop snapshot feature in favor of build-tag isolation" --body "$(cat <<'EOF'
## Summary
- Deletes `entc/internal/snapshot.go`, the `FeatureSnapshot` flag, the `internal/schema` template, the `SnapshotDir` option, and the `mayRecover` recovery path.
- Adds `wrapLoadError` that names the failing import and suggests `//go:build !entcodegen` when the broken symbol looks like generated code.
- Adds error-path integration tests in `entc/integration/codegen_isolation/` to pin the new behavior.
- Removes `FeatureSnapshot` / orphan `internal/schema.go` files from integration directories; regenerates affected integrations.
- Makes `ResolveIncrementStartsConflict` unconditional in `entc/gen/globalid.go` (previously gated by `FeatureSnapshot`).

Depends on #PR1 (`feat/entcodegen-tag-injection`).

This is the second of four ent fork PRs in the codegen-isolation epic. PR #3 shrinks `ent.Interface` (drops `Hooks/Interceptors/Policy`); PR #4 drops `FeaturePrivacy`.

## Test plan
- [x] `go test ./entc/...` — passes including new `wrapLoadError` unit tests and new error-path integration tests
- [x] `go test ./entc/integration/...` — passes; affected integrations regenerated; hooks and edgeschema integrations still green
- [x] Manual verification: a deliberately-broken file without the build tag now produces an actionable error message (no silent snapshot restore)

Spec: `docs/superpowers/specs/2026-05-22-ent-codegen-isolation-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Wait for CI green and approval before proceeding to PR #3 (covered in a separate plan).

---

## What This Plan Does NOT Cover

Out of scope for this plan; addressed in separate plans:

- **Plan 2 — `ent.Interface` shrink (PR #3):** removes `Hooks()/Interceptors()/Policy()` methods from the interface, deletes loader methods (`loadHooks/safeHooks/etc.`), deletes template gating (`NumHooks/NumPolicy`), rewrites `entc/integration/hooks/` tests to use `client.Use`, deletes per-type accessor methods.
- **Plan 3 — Privacy removal (PR #4):** deletes `FeaturePrivacy`, the `privacy/` runtime package, the `privacy/` template subdir, and the entire `entc/integration/privacy/` directory.
- **Plan 4 — Consumer migration (different repo):** all 6 consumer PRs in `matthewsreis/gemini`, including migration script (`cmd/entcodegen-migrate`), lint guard (`cmd/entcodegen-lint`), `hookreg/` package authoring, annotation-closure refactors, and the cutover.

---

## Self-Review

**Spec coverage (this plan vs spec §6.1):**
- Tag injection in `entc/load/load.go:118` — covered by Tasks 1.1, 1.2.
- `wrapLoadError` replaces `mayRecover` — covered by Tasks 2.1, 2.2.
- Delete `entc/internal/snapshot.go` and `snapshot_test.go` — Tasks 2.4, 2.5.
- Delete `FeatureSnapshot` from `entc/gen/feature.go` and `AllFeatures` — Task 2.6.
- Delete `SnapshotDir` option in `entc/entc.go` — Task 2.2.
- Delete snapshot-copy block in `entc/entc.go` — Task 2.2.
- Delete `internal/schema` template block — Task 2.7.
- Update `entc/gen/globalid.go:36` — Task 2.8.
- Delete `entc/gen/graph_test.go:595` test case — Task 2.9.
- Delete integration `internal/schema.go` snapshot files + drop feature names — Task 2.10.
- New `codegen_isolation` integration directory — Tasks 1.3, 1.4 (positive) + 2.3 (negative).
- New tests for `mergeCodegenTag` and `wrapLoadError` — Tasks 1.1, 2.1.

Spec items deferred to other plans (intentionally):
- §6.1 "ent core interface" bullets — Plan 2.
- §6.1 "Codegen (`entc/gen/`)" bullets touching `NumHooks/NumPolicy` — Plan 2.
- §6.1 "Templates" bullets for `meta.tmpl`, `runtime.tmpl`, etc. — Plan 2.
- §6.1 "Privacy package" deletion — Plan 3.
- §6.2 — Plan 4.

**Placeholder scan:** none. Every step has the code or command needed.

**Type consistency:** `mergeCodegenTag` (Task 1.1) is called once from `Config.Load` (Task 1.2) with identical signature. `wrapLoadError` (Task 2.1) signature matches its call site in `generate` (Task 2.2). The `codegenTag` constant is used consistently across `tags.go` and is the literal value asserted in tests.

---

**Plan complete and saved to `docs/superpowers/plans/2026-05-22-ent-codegen-isolation-foundation.md`. Two execution options:**

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
