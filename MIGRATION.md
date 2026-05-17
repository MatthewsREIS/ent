# Migration Guide

<!-- NEW SECTIONS GO AT THE TOP; older sections below -->

## Per-Entity Sub-Packages (codegen-epic PR 6)

PR 6 splits the monolithic `ent` package so that each entity's query / client / mutation / create / update / delete builders live in their own sub-package (`ent/internal/<entity>`). The top-level `ent` package retains type aliases (`type TaskQuery = task.Query`, `type TaskClient = task.Client`, …) so existing variable types keep working, but **cross-entity edge methods can no longer live on the alias** — they have been moved to package-level facade functions in `ent`.

### What changed (consumer-visible)

Cross-entity edge access — the four call shapes — must move from method calls on `*ent.TaskQuery` / `*ent.TaskClient` to facade functions in `ent`:

| Before (method on the alias) | After (facade function in `ent`) |
|---|---|
| `client.Task.Query().WithTeams(func(q *ent.TeamQuery) { ... })` | `ent.WithTaskTeams(client.Task.Query(), func(q *ent.TeamQuery) { ... })` |
| `client.Task.QueryTeams(t)` | `ent.QueryTaskTeams(client.Task, t)` |
| `q.QueryTeams()` (chained from another query) | `ent.QueryTaskTeamsFromQuery(q)` |
| `q.WithNamedTeams("foo", func(q *ent.TeamQuery) { ... })` | `ent.WithNamedTaskTeams(q, "foo", func(q *ent.TeamQuery) { ... })` |

Naming pattern is `<Verb><NodeName><EdgeName>` (and `<Verb><NodeName><EdgeName>FromQuery` for the chained form). The first argument is always the receiver that used to be the method's `this`.

### What did NOT change

- `*ent.TaskQuery`, `*ent.TaskClient`, `*ent.TaskMutation`, `*ent.TaskCreate`, `*ent.TaskUpdate`, `*ent.TaskUpdateOne`, `*ent.TaskDelete`, `*ent.TaskDeleteOne` are still importable from `ent`. They are now type aliases to the sub-package types, so existing function signatures, variable declarations, and type assertions keep compiling.
- Self-only chaining — `Where(...)`, `Limit(...)`, `Order(...)`, `Offset(...)`, `Unique(...)`, `All(ctx)`, `First(ctx)`, `Only(ctx)`, `Count(ctx)`, `Exist(ctx)`, `Clone()`, `Select(...)`, `GroupBy(...)`, `Aggregate(...)` — is unchanged. Anything that doesn't cross an entity boundary still works as a method.
- Hook / interceptor / policy / schema-extension APIs are untouched.

Note on **chained cross-entity traversals**: a single expression like `client.Task.Query().QueryTeams().QueryMembers()` becomes a step-by-step facade-call rewrite, because each chain link is now a separate function:

```go
tq  := client.Task.Query()
teq := ent.QueryTaskTeamsFromQuery(tq)
meq := ent.QueryTeamMembersFromQuery(teq)
```

The migration tool handles this automatically, but the resulting code is intentionally less dense than the original method chain.

### Why

Sub-packages are part of the codegen-reduction epic. Splitting per-entity code into its own Go package lets the Go compiler shard work across packages in parallel, cutting wall-clock build time on large schemas. The catch is that **edge methods are inherently cross-entity** (`TaskQuery.QueryTeams` returns `*TeamQuery`), so leaving them as methods on the alias would force `ent/internal/task` to import `ent/internal/team` and vice versa — re-introducing the import cycle the split exists to eliminate. Moving the edge surface to package-level facades in the top-level `ent` package is what makes the split viable.

### Migration

1. Regenerate ent code with the PR 6 generator:
   ```bash
   go generate ./internal/ent
   ```
2. Run the migration tool against your consumer source tree:
   ```bash
   go run entgo.io/ent/cmd/ent-codegen-migrate \
       -descriptors ./internal/ent/internal \
       ./internal/  ./pkg/...
   ```
3. Verify the build:
   ```bash
   go build ./...
   ```

The tool runs four passes over each file (mutation rewrites from PR 5 → predicate rewrites from PR 3 → edge-method rewrites → typed-edge-accessor rewrites). Each pass is idempotent and re-running the whole tool on already-migrated code is a no-op. The `-descriptors` flag points at the regenerated `internal/` directory so the tool can learn each entity's edge cardinalities and target types from the descriptor literals.

### Recovery from the broken pre-PR6 tool

An earlier revision of `ent-codegen-migrate` (shipped during PR 6 development) emitted malformed rewrites for the edge passes — e.g. dropped arguments, wrong receiver order, or partially-rewritten chains. If you ran that version and your tree no longer builds:

```bash
# 1. Restore the consumer source to its pre-migration state
git checkout -- ./internal ./pkg

# 2. Re-run the now-fixed tool (idempotent; safe on a clean tree)
go run entgo.io/ent/cmd/ent-codegen-migrate \
    -descriptors ./internal/ent/internal \
    ./internal/  ./pkg/...

# 3. Verify
go build ./...
```

If you'd already hand-edited some files before the tool was fixed, those hand edits are preserved by `git checkout` only if you committed them first — otherwise restore from your editor's history or re-do them after step 2.

### Additional API surface introduced

Beyond the four documented edge shapes, PR 6 introduces these symbols on the `ent` facade and on each sub-package query type:

- `ent.With<Node><Edge>(q, opts...)` — eager-load the edge from a base query (the `WithX` replacement above).
- `ent.WithNamed<Node><Edge>(q, name, opts...)` — named eager-load variant.
- `ent.Query<Node><Edge>(client.<Node>, instance)` — from-client edge query (the `client.Node.QueryX(inst)` replacement).
- `ent.Query<Node><Edge>FromQuery(q)` — chained edge query (the `q.QueryX()` replacement).
- On each `<entity>.Query` type: `SQLQuery(ctx)`, `PrepareQuery(ctx)`, and `IncludeForeignKeys(bool)` are now exported (they were package-private before the split). These are primarily for internal integrations (e.g. custom schema extensions); typical consumers don't need to call them.

---

## Mutation Collapse (codegen-epic PR 5)

PR 5 rewrites `internal_mutation.tmpl` so that generated entities no longer emit per-field typed mutation methods (`SetName`, `AddAge`, `Name`, `OldName`, etc.) or the backing struct fields (`name`, `oldName`, `addAge`, …). The `runtime/entbuilder.Mutation[T]` generic type becomes the sole mutation carrier.

### What changed

| Before (generated per-entity) | After (generic, in `runtime/entbuilder`) |
|---|---|
| `func (m *UserMutation) SetName(v string)` | `entbuilder.SetField(m, user.FieldName, v)` |
| `func (m *UserMutation) Name() (string, bool)` | `entbuilder.GetField[string](m, user.FieldName)` |
| `func (m *UserMutation) OldName(ctx) (string, error)` | `entbuilder.OldFieldAs[string](m, ctx, user.FieldName)` |
| `func (m *UserMutation) AddAge(v int)` | `entbuilder.AppendField(m, user.FieldAge, v)` |
| `func (m *UserMutation) AddedAge() (int, bool)` | `entbuilder.AppendedField[int](m, user.FieldAge)` |
| `func (m *UserMutation) ClearName()` | `m.ClearField(user.FieldName)` |
| `func (m *UserMutation) NameCleared() bool` | `m.FieldCleared(user.FieldName)` |

The CUD builders (`UserCreate`, `UserUpdate`, `UserUpdateOne`, `UserDelete`, `UserDeleteOne`) still exist as generated wrappers and their public API is unchanged.

### Hook / middleware migration

If you access mutation fields in `ent.Hook` or `ent.Policy` implementations, update to the generic API:

```go
import (
    "entgo.io/ent/runtime/entbuilder"
    "your-module/ent/user"
)

// Before
func myHook(next ent.Mutator) ent.Mutator {
    return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
        if um, ok := m.(*ent.UserMutation); ok {
            if name, exists := um.Name(); exists {
                // use name
            }
            um.SetName("overridden")
        }
        return next.Mutate(ctx, m)
    })
}

// After
func myHook(next ent.Mutator) ent.Mutator {
    return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
        if um, ok := m.(*ent.UserMutation); ok {
            if name, exists := entbuilder.GetField[string](um, user.FieldName); exists {
                // use name
            }
            entbuilder.SetField(um, user.FieldName, "overridden")
        }
        return next.Mutate(ctx, m)
    })
}
```

### Automated migration tool

`ent-codegen-migrate` (at `cmd/ent-codegen-migrate`) rewrites call sites mechanically. The `-descriptors` flag points at the consumer's regenerated `internal/` package so the tool can learn field types and edge cardinalities from the descriptor literals.

```bash
# Dry-run: print which files would change (no writes)
go run entgo.io/ent/cmd/ent-codegen-migrate \
    -descriptors ./path/to/your/ent/gen/internal \
    -dry-run \
    ./path/to/your/source/...

# Apply rewrites in-place
go run entgo.io/ent/cmd/ent-codegen-migrate \
    -descriptors ./path/to/your/ent/gen/internal \
    ./path/to/your/source/...
```

The tool handles:
- Typed setters: `m.SetTitle(v)` → `m.SetField("title", v)`
- Typed getters: `m.Title()` → `entbuilder.GetField[string](m, "title")`
- Typed old-value reads: `m.OldTitle(ctx)` → `entbuilder.OldFieldAs[string](ctx, m, "title")`
- Edge add/remove: `m.AddTeamIDs(ids...)` → `m.AddEdgeIDs("teams", entbuilder.ToAny(ids)...)`
- Edge state checks: `m.TeamsCleared()` / `m.OwnerCleared()` → `m.EdgeCleared("teams"|"owner")`
- Predicate wrappers (from PR 3): `user.NameEQ(x)` → `where.EQ(user.FieldName, x)` (ID helpers like `user.IDIn` are preserved)
- `m.Where(ps...)` on a mutation receiver → `m.WhereP(ps...)`

The dispatch is heuristic — the tool matches method-name shape and looks up the inferred field/edge name across all descriptors (first match wins). Patterns it can't auto-rewrite (e.g. passing a method value: `someFunc(m.SetTitle)`) are left unchanged; verify your build after running.

### Known regression: gremlin storage backend

PR 5's `internal_mutation.tmpl` rewrite did not include a `{{ if eq $.Storage.Name "sql" }}` guard. As a result, gremlin-storage entities now emit `func(*sql.Selector)` predicates that don't match gremlin's `func(*dsl.Traversal)` predicate type, causing `entc/integration/gremlin/...` to fail to build.

The matthewsreis fork doesn't use gremlin; this is documented as a known issue rather than fixed in PR 5. Restoring gremlin support requires either:
- An `{{ else }}` branch in `internal_mutation.tmpl` that emits the pre-PR-5 per-entity mutation template content for gremlin storage, or
- Making `runtime/entbuilder.Mutation[T]` dialect-agnostic (parameterising the predicate type)

Tracking: out-of-scope follow-up.

---

# Sub-Package Migration Guide

This document covers the breaking API changes introduced by the sub-package split. After regenerating your code with the updated `ent` code generator, follow these steps to update your application code.

## Predicate Collapse (codegen-epic PR 3)

New `entgo.io/ent/where` generic predicate package + deprecated per-entity
wrappers. See [doc/md/migration-predicate-collapse.md](doc/md/migration-predicate-collapse.md).

## Overview

Generated ent packages now use a 3-tier structure:

```
ent/
  internal/          # Real types, mutations, model structs
  user/              # CUD builders (create, update, delete) + shared type aliases
  pet/               # CUD builders + shared type aliases
  ...
  ent.go             # Root package with type aliases to internal/
  client.go          # Client with entity-scoped sub-clients
  config.go          # Shared configuration
```

The root `ent` package re-exports everything via type aliases, so most **read-only** code continues to work. The breaking changes affect code that calls methods on entity instances for mutations or traversals.

## Breaking Changes

### 1. Entity Query Methods Moved to Client

Entity methods like `user.QueryPets()` no longer exist. Use the client's entity-scoped sub-client instead.

```diff
- pets, err := user.QueryPets().All(ctx)
+ pets, err := client.Pet.Query().Where(pet.HasOwnerWith(user.ID(user.ID))).All(ctx)

// Or use the convenience method on the entity client:
- pets, err := user.QueryPets().All(ctx)
+ pets, err := client.User.QueryPets(user).All(ctx)
```

### 2. Entity Update Methods Moved to Client

```diff
- user.Update().SetName("new-name").Save(ctx)
+ client.User.UpdateOne(user).SetName("new-name").Save(ctx)
```

### 3. Entity Edge Setters Use IDs

Edge setters that previously accepted entity values now require IDs.

```diff
- client.Pet.Create().SetOwner(user).Save(ctx)
+ client.Pet.Create().SetOwnerID(user.ID).Save(ctx)

- client.User.Create().AddPets(pet1, pet2).Save(ctx)
+ client.User.Create().AddPetIDs(pet1.ID, pet2.ID).Save(ctx)
```

### 4. `m.Client()` and `m.Tx()` Removed from Mutations

In hooks and middleware, mutations no longer have `Client()` or `Tx()` methods. Use context-based accessors instead.

```diff
  func myHook(next ent.Mutator) ent.Mutator {
      return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
-         client := m.Client()
+         client := ent.FromContext(ctx)
          // use client...
          return next.Mutate(ctx, m)
      })
  }
```

For transactions:

```diff
- tx, err := m.Tx()
+ tx := ent.TxFromContext(ctx)
+ if tx == nil {
+     return nil, fmt.Errorf("not running in a transaction")
+ }
```

**Important:** You must inject the client/tx into the context for these accessors to work:

```go
// For client:
ctx = ent.NewContext(ctx, client)

// For transactions:
ctx = ent.NewTxContext(ctx, tx)
```

### 5. `entity.Unwrap()` Removed

`Unwrap()` was used to get the underlying entity after a transaction commit. Use a fresh query instead.

```diff
- clean := user.Unwrap()
- pets, err := clean.QueryPets().All(ctx)
+ fresh := client.User.GetX(ctx, user.ID)
+ pets, err := client.User.QueryPets(fresh).All(ctx)
```

### 6. `NotFoundError` Uses Keyed Fields

```diff
- &ent.NotFoundError{"user"}
+ &ent.NotFoundError{Label: "user"}
```

## Automated Migration Tool (Best-Effort)

The `ent` CLI now includes a best-effort rewrite command for this migration:

```bash
# Dry-run (default): scans and reports without changing files.
ent migrate split-api .

# Apply safe rewrites in-place and write a residual report.
ent migrate split-api --write --report migration-report.md .
```

What the command rewrites automatically:

- `entity.Update()` -> `client.<Entity>.UpdateOne(entity)`
- `entity.Unwrap()` -> `client.<Entity>.GetX(ctx, entity.ID)` (only when `client` and `ctx` are discoverable)
- `entity.QueryX()` -> `client.<Entity>.QueryX(entity)` (only when `client` is discoverable)
- Edge setters that pass entities:
  - `.SetOwner(user)` -> `.SetOwnerID(user.ID)`
  - `.AddPets(p1, p2)` -> `.AddPetIDs(p1.ID, p2.ID)`
- `NotFoundError{"label"}` -> `NotFoundError{Label: "label"}`

What the command intentionally leaves as residuals for manual follow-up:

- `m.Client()` and `m.Tx()` call sites where context requirements are application-specific
- query/update/unwrap sites where the needed `client` and/or `ctx` cannot be inferred safely
- any cases outside the known mechanical patterns above

When `--report` is set, unresolved sites are written to a Markdown report with file, line, pattern, and manual advice.

The tool is designed to be safe and mechanical, not complete. Always run your test/build pipeline after applying changes.

## Migration Checklist

1. **Regenerate code:** Run `go generate ./ent` to regenerate with the new templates.

2. **Run automated migration:**

   ```bash
   ent migrate split-api --write --report migration-report.md .
   ```

3. **Fix residual/manual items** from `migration-report.md` using the patterns above. Common search-and-replace patterns:

   | Find | Replace With |
   |------|-------------|
   | `entity.QueryFoo()` | `client.Entity.QueryFoo(entity)` |
   | `entity.Update()` | `client.Entity.UpdateOne(entity)` |
   | `entity.Unwrap()` | `client.Entity.GetX(ctx, entity.ID)` |
   | `.SetOwner(entity)` | `.SetOwnerID(entity.ID)` |
   | `.AddPets(pets...)` | `.AddPetIDs(ids...)` |
   | `m.Client()` | `ent.FromContext(ctx)` |
   | `m.Tx()` | `ent.TxFromContext(ctx)` |

4. **Inject context** in tests and entry points:

   ```go
   client := ent.Open(...)
   ctx := ent.NewContext(context.Background(), client)
   ```

5. **Update custom templates** that reference `*config` (lowercase) to use `*Config` (uppercase), since `Config` is now an exported type from `internal/`.

6. **Validate the migration:** Run `go test ./...` and your application-specific checks.

## AI-Agent Runbook

If you are using an AI coding agent, use this order:

1. Regenerate ent code:

   ```bash
   go generate ./ent
   ```

2. Run the automated migration tool with a residual report:

   ```bash
   ent migrate split-api --write --report migration-report.md .
   ```

3. Run the project test/build checks (at minimum `go test ./...`).

4. Read `migration-report.md` and apply only the listed manual rewrites.

5. Re-run tests/build, then do a final grep for known patterns (`.Unwrap(`, `.Client()`, `.Tx()`, `.Query` on entity values) to catch missed call sites.

## Hooks Requiring Context Injection

If your hooks use `ent.FromContext(ctx)` or `ent.TxFromContext(ctx)`, the client or transaction must be in the context. Ensure your application entry points inject them:

```go
// HTTP handler example
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := ent.NewContext(r.Context(), client)
    // ... handlers that trigger hooks can now access the client
}

// Transaction example
tx, err := client.Tx(ctx)
if err != nil { ... }
ctx = ent.NewTxContext(ctx, tx)
// ... operations within tx that trigger hooks can now access the tx
```
