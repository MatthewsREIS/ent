# Card Descriptor-Driven Spike

This directory is a throwaway scratch space for Phase 4A's spike. It hand-rolls
a replacement for the generated `*CardMutation` that delegates to
`entbuilder.Mutation[Card]`, to validate that the descriptor-driven approach
is feasible before committing to a template rewrite.

Contents (all hand-written, not generated):

- `card_descriptor.go` — the Card schema descriptor + its OldFieldFetcher.
- `card_mutation_shim.go` — façade methods that map the existing CardMutation
  API onto the generic Mutation[Card].
- `spike_test.go` — a copy (or harness) of the relevant hooks_test.go cases,
  adapted to use the shim.

On gate success: a subsequent plan replaces the generated CardMutation
entirely. On gate failure: the spike is dropped and a new spec is written
for a different approach.

---

## CardMutation surface — shim coverage checklist

Generated file reference: `../ent/internal/card_mutation.go`.

The public `ent.CardMutation` type is a type-alias (`type CardMutation =
internal.CardMutation`) so every method on `*internal.CardMutation` is
directly the public surface.

### Fields (Number, Name, CreatedAt, InHook, ExpiredAt)

For each field, the shim must provide:

- [ ] `SetNumber(s string)` — record value (immutable after Create; no-op guard TBD)
- [ ] `Number() (string, bool)` — read set value
- [ ] `OldNumber(ctx context.Context) (string, error)` — pre-mutation value
- [ ] `ResetNumber()` — clear all state
  > Note: `Number` is **immutable** — no `SetNumber` after create. The generated
  > code nonetheless includes `SetNumber` / `ResetNumber` (used by the builder
  > during the Create path), so the shim must include them too.

- [ ] `SetName(s string)`
- [ ] `Name() (string, bool)`
- [ ] `OldName(ctx context.Context) (string, error)`
- [ ] `ClearName()` — mark cleared (optional field)
- [ ] `NameCleared() bool`
- [ ] `ResetName()`

- [ ] `SetCreatedAt(t time.Time)`
- [ ] `CreatedAt() (time.Time, bool)`
- [ ] `OldCreatedAt(ctx context.Context) (time.Time, error)`
- [ ] `ResetCreatedAt()`

- [ ] `SetInHook(s string)`
- [ ] `InHook() (string, bool)`
- [ ] `OldInHook(ctx context.Context) (string, error)`
- [ ] `ResetInHook()`

- [ ] `SetExpiredAt(t time.Time)`
- [ ] `ExpiredAt() (time.Time, bool)`
- [ ] `OldExpiredAt(ctx context.Context) (time.Time, error)`
- [ ] `ClearExpiredAt()` — mark cleared (optional field)
- [ ] `ExpiredAtCleared() bool`
- [ ] `ResetExpiredAt()`

### Owner edge (unique inverse to User)

- [ ] `SetOwnerID(id int)`
- [ ] `ClearOwner()`
- [ ] `OwnerCleared() bool`
- [ ] `OwnerID() (int, bool)`
- [ ] `OwnerIDs() []int` — legacy; returns a singleton slice for unique edges
- [ ] `ResetOwner()`

### Introspection (generic ent.Mutation interface)

These are the methods hooks_test.go calls directly on `*ent.CardMutation` or
relies on via the generic `ent.Mutation` interface:

- [ ] `Op() Op`
- [ ] `SetOp(op Op)`
- [ ] `Type() string`
- [ ] `ID() (int, bool)`
- [ ] `IDs(ctx context.Context) ([]int, error)`
- [ ] `Fields() []string`
- [ ] `Field(name string) (ent.Value, bool)`
- [ ] `OldField(ctx context.Context, name string) (ent.Value, error)`
- [ ] `SetField(name string, value ent.Value) error`
- [ ] `AddedFields() []string`
- [ ] `AddedField(name string) (ent.Value, bool)`
- [ ] `AddField(name string, value ent.Value) error`
- [ ] `ClearedFields() []string`
- [ ] `FieldCleared(name string) bool`
- [ ] `ClearField(name string) error`
- [ ] `ResetField(name string) error`
- [ ] `AddedEdges() []string`
- [ ] `AddedIDs(name string) []ent.Value`
- [ ] `RemovedEdges() []string`
- [ ] `RemovedIDs(name string) []ent.Value`
- [ ] `ClearedEdges() []string`
- [ ] `EdgeCleared(name string) bool`
- [ ] `ClearEdge(name string) error`
- [ ] `ResetEdge(name string) error`

### Where / predicate support

hooks_test.go (TestMutatorClient) calls `m.Where(card.ExpiredAtIsNil())` and
`m.IDs(ctx)` on a `*CardMutation` inside a Delete/DeleteOne hook, then calls
`m.SetOp(...)` to pivot the operation. These are load-bearing for that test.

- [ ] `Where(ps ...predicate.Card)`
- [ ] `WhereP(ps ...func(*sql.Selector))` — storage-level predicate escape hatch

### Internal / builder helpers

These are used by the generated builders (not by hooks_test.go directly), but
must be present so the existing builder code continues to compile:

- [ ] `SetMutationID(id *int)` — called by the create builder after INSERT
- [ ] `SetDone()` — called after the mutation completes to fence OldXxx
- [ ] `MutationPredicates() []predicate.Card` — used by Update executor
- [ ] `AddPredicate(pred func(s *sql.Selector))` — used by entql filter injection

### Embedded Config field

`m.Config` is accessed directly (not via a method) in TestMutationClient,
TestMutatorClient, TestMutationTx, and TestRuntimeTx to construct new clients.
The shim struct must embed or expose the `internal.Config` struct at the same
field name.

---

## hooks_test.go load-bearing CardMutation calls

The following methods are called explicitly on `*ent.CardMutation` in
hooks_test.go. Every one must be present and correct in the shim:

| Method | Test(s) |
|--------|---------|
| `m.Name()` | TestSchemaHooks |
| `m.OwnerID()` | TestMutationClient |
| `m.SetName(...)` | TestMutationClient |
| `m.Config` (field) | TestMutationClient, TestMutatorClient, TestMutationTx, TestRuntimeTx |
| `m.ID()` | TestMutatorClient, TestPostCreation |
| `m.Op()` | TestMutatorClient, TestConditions |
| `m.Where(...)` | TestMutatorClient |
| `m.IDs(ctx)` | TestMutatorClient |
| `m.SetOp(...)` | TestMutatorClient |
| `m.SetExpiredAt(...)` | TestMutatorClient |
| `m.OldNumber(ctx)` | TestOldValues (post-mutation error path) |
| `m.Fields()` | TestMutationStateSurface |
| `m.ResetName()` | TestMutationStateSurface |

No CardMutation call in hooks_test.go is left uncovered by the checklist above.

---

## Deferred (not needed for hooks_test.go green)

These exist on the generated type but are not exercised by hooks_test.go
CardMutation code paths:

- `OldCreatedAt`, `OldInHook`, `OldExpiredAt` — OldXxx for non-name fields;
  present on the type but never called in test hooks.
- `CreatedAt()`, `InHook()`, `ExpiredAt()` — field readers not called in hooks.
- `ClearName()`, `NameCleared()`, `ClearExpiredAt()`, `ExpiredAtCleared()` —
  tested via the hook condition combinator (hook.HasClearedFields) but only
  indirectly; the shim must expose them so the hook framework's introspection
  works, so they are NOT truly deferrable if we want TestConditions to pass.
- `AddedFields()`, `AddedField()`, `AddField()` — Card has no numeric fields
  so these are no-ops, but the `ent.Mutation` interface requires them.
- `ClearField()`, `ResetField()`, `ClearEdge()`, `ResetEdge()` — used by
  internal ent machinery, not directly by test hooks; required for compilation.
