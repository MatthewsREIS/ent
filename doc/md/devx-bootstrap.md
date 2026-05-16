---
id: devx-bootstrap
title: Bootstrap mode (hook chicken-and-egg)
---

## When you need it

You added a new field to a schema and an existing hook references the
not-yet-generated mutation method:

```go
// ent/schema/user.go
func (User) Hooks() []ent.Hook {
    return []ent.Hook{
        func(next ent.Mutator) ent.Mutator {
            return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
                if um, ok := m.(*ent.UserMutation); ok {
                    um.SetNewField("x") // doesn't exist yet -- you just added the field
                }
                return next.Mutate(ctx, m)
            })
        },
    }
}
```

When you try to regenerate:

```
$ go generate ./ent
ent/schema/user.go:13:8: um.SetNewField undefined (type *ent.UserMutation has no field or method SetNewField)
```

The loader needs to type-check the schema package, but the schema package
imports the generated `ent` package, which doesn't have `SetNewField` yet.
Without bootstrap mode, you have to comment out the hook, regenerate, then
uncomment.

## How bootstrap mode works

Bootstrap mode AST-strips the bodies of `Hooks()`, `Policy()`, and
`Interceptors()` methods on every Schema-implementing type in a **temp
copy** of your schema package. The original source is never touched. The
loader runs against the stripped copy, codegen produces the new methods
(including `SetNewField`), and your real schema package compiles cleanly
on the next build.

## Two ways to enable it

### Automatic (recommended)

As of codegen-epic PR 1, `entc.Generate` automatically falls back to
bootstrap mode when the loader fails with a type-checker error class
(`undefined:`, `has no field or method`, `undeclared name`, `cannot use`).
Snapshot restore is tried first; bootstrap kicks in only if snapshot
recovery also fails.

For most users, this is invisible — codegen just works.

### Explicit

Pass `entc.SkipHookCompilation()` in your generator main, or use the
`--skip-hook-compilation` flag on the CLI:

```go
// ent/generate.go
package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
)

func main() {
    if err := entc.Generate("./schema", &gen.Config{}, entc.SkipHookCompilation()); err != nil {
        log.Fatalf("ent codegen: %v", err)
    }
}
```

```bash
ent generate --skip-hook-compilation ./ent/schema
```

Use the explicit form when you know your schemas always reference
generated types from hooks and you don't want the auto-fallback overhead
of failing once before bootstrap kicks in.

## Limitations

**Hooks in subpackages.** Bootstrap only strips methods in the schema
package itself. If your hooks live in a subpackage that imports the
generated ent package, that subpackage will still fail to type-check.
The standard fix is to keep generated-type-touching hooks in a package
that is *not* imported by your schema package.

**Helpers in the schema package that reference generated types.** The
stripper only touches method bodies on Schema-implementing types. A
top-level helper function in the schema package — say,
`func myValidator(s string) error` — that references generated types
will still fail to compile. Move such helpers to a subpackage that is
not imported by your schema package.

**Schemas with computed Fields()/Edges().** If `Fields()` or `Edges()`
calls a helper that references generated types, bootstrap can't help —
the loader needs those methods to evaluate, and we can't strip them
without losing the schema definitions.
