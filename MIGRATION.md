# Sub-Package Migration Guide

This document covers the breaking API changes introduced by the sub-package split. After regenerating your code with the updated `ent` code generator, follow these steps to update your application code.

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

## Migration Checklist

1. **Regenerate code:** Run `go generate ./ent` to regenerate with the new templates.

2. **Fix compilation errors** using the patterns above. Common search-and-replace patterns:

   | Find | Replace With |
   |------|-------------|
   | `entity.QueryFoo()` | `client.Entity.QueryFoo(entity)` |
   | `entity.Update()` | `client.Entity.UpdateOne(entity)` |
   | `entity.Unwrap()` | `client.Entity.GetX(ctx, entity.ID)` |
   | `.SetOwner(entity)` | `.SetOwnerID(entity.ID)` |
   | `.AddPets(pets...)` | `.AddPetIDs(ids...)` |
   | `m.Client()` | `ent.FromContext(ctx)` |
   | `m.Tx()` | `ent.TxFromContext(ctx)` |

3. **Inject context** in tests and entry points:

   ```go
   client := ent.Open(...)
   ctx := ent.NewContext(context.Background(), client)
   ```

4. **Update custom templates** that reference `*config` (lowercase) to use `*Config` (uppercase), since `Config` is now an exported type from `internal/`.

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
