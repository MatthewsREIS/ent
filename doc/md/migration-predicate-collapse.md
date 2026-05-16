# Predicate Collapse Migration Guide

Codegen-epic PR 3 introduces a new generic predicate package and shrinks the
per-entity `where.go` files. This page covers what changed, what you need to
do (if anything), and the timeline.

## What changed

### New package: `entgo.io/ent/where`

```go
import "entgo.io/ent/where"
import "myapp/ent/user"

// Before:
users, err := client.User.Query().
    Where(user.NameEQ("alice")).
    Where(user.AgeGT(18)).
    All(ctx)

// After (new style — preferred for new code):
users, err := client.User.Query().
    Where(where.EQ(user.FieldName, "alice")).
    Where(where.GT(user.FieldAge, 18)).
    All(ctx)
```

The `where` package exposes generic helpers for every standard scalar operator:
`EQ`, `NEQ`, `In`, `NotIn`, `GT`, `GTE`, `LT`, `LTE`, `IsNull`, `NotNull`,
`Contains`, `ContainsFold`, `EqualFold`, `HasPrefix`, `HasPrefixFold`,
`HasSuffix`, `HasSuffixFold`, plus combinators `And`, `Or`, `Not`.

Edge predicates (`HasGroups`, `HasGroupsWith`) and ID predicates (`ID`, `IDEQ`,
`IDIn`, …) stay on their per-entity packages — they need entity-specific
metadata that can't be generic.

### Per-entity wrappers are now deprecated

The existing `user.NameEQ(v)`, `user.AgeGT(v)`, etc. still work but are marked
`// Deprecated:` in the generated code. Each wrapper is a one-line delegate to
the corresponding `where.XX` function. They'll be removed in the release
*after* the next one.

### `predicate.<Entity>` is now a type alias

`type predicate.User = func(*sql.Selector)` (was: `type predicate.User
func(*sql.Selector)`). This is what lets `where.EQ(...)` flow directly into
`Where(...predicate.User)` without an explicit cast.

**Caveat:** predicate types lose nominal identity. Hand-written code that
relied on the type system to prevent cross-entity predicate assignment
(`var p predicate.User = someOrderPredicate`) no longer gets that compile-time
guard. In practice this rarely happens; if you find a real call site, file an
issue and we'll discuss a typed-predicate helper.

## Migration timeline

| Release | Status |
|---|---|
| Current (PR 3) | New `where` package shipped. Per-entity wrappers deprecated but functional. Both APIs work side by side. |
| Next | No change. Migrate consumer call sites to `where.XX` at your own pace. |
| Release after next | Deprecated per-entity wrappers removed. Only `where.XX` and the per-entity edge/ID predicates remain. |

## Mechanical migration recipe

Most call sites can be migrated with a regex codemod:

```
# user.NameEQ("x")    → where.EQ(user.FieldName, "x")
# user.AgeGT(18)      → where.GT(user.FieldAge, 18)
# user.NameIn("a","b")→ where.In(user.FieldName, "a", "b")

s/(\w+)\.(\w+)(EQ|NEQ|In|NotIn|GT|GTE|LT|LTE|Contains|HasPrefix|HasSuffix|EqualFold|ContainsFold|HasPrefixFold|HasSuffixFold)\(/where.\3(\1.Field\2, /g
```

Test the substitution on a single file before running it globally. Edge and ID
predicates won't match and stay as-is.

## What didn't change

- `Where(...)` method signatures
- `And`/`Or`/`Not` per-entity helpers (still available; `where.And`/`Or`/`Not`
  are equivalent)
- Query/Update/Delete builder shapes
- Generated mutation code
