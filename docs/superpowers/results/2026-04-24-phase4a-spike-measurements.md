# Phase 4A Spike Measurements

**Date:** 2026-04-24
**Spec:** docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md
**Plan:** docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4a-spike.md
**Fork HEAD:** 312a256f7a80dae878af40106cb2d4435eab9fb4

## Spike results

| Metric | Value |
|---|---|
| Spike source LOC (card_descriptor.go + card_mutation_shim.go) | 266 (103 + 163) |
| Generated card_mutation.go LOC (the hypothetical replacement target) | 680 |
| LOC ratio (spike / generated) | spike/gen = 39% |
| Spike build wall time | 6.35s |
| Spike build peak RSS | 337352 kB |
| Integration tests (Phase 4A tasks 17+18) | green (8/8 PASS) |
| Descriptor validates against ent.Card | green (TestCardDescriptor_Valid PASS) |

## Gate decision

**Outcome:** GO

### Gate results

| Gate | Target | Measured | Met? |
|---|---|---|---|
| Spike tests green | 100% | 8/8 PASS | ✓ |
| Descriptor validates against ent.Card | green | TestCardDescriptor_Valid PASS | ✓ |
| OldField via real client works | green | TestShim_OldName_ThroughHooksClient PASS | ✓ |
| Shim LOC < 50% of generated | < 50% | 39% (266/680) | ✓ |
| No runtime panics | zero | zero | ✓ |

### Interpretation

The spike validates the descriptor-driven approach end-to-end. Key signals:

1. **61% LOC reduction on one schema's mutation file** (680 → 266 lines). Extrapolated across
   the consumer's 111 schemas, the same ratio applied to the ~150k LOC of mutation code
   would cut roughly 90k lines, far larger than Approach 1's where.go-only win.

2. **OldField via schema closure works correctly.** The most load-bearing test —
   `TestShim_OldName_ThroughHooksClient` — exercises the full path: seed a row, open a
   mutation via the shim, set a new value, and read the pre-mutation value via
   `entbuilder.Mutation[T].OldField` → schema descriptor closure → real SQLite query.
   This is the mechanism that Phase 4C's template-driven code will use.

3. **Descriptor validation catches drift at boot time**, not query time. A schema field
   whose struct type doesn't match the descriptor fails `ValidateSchema` at
   client-construction, satisfying the correctness-first constraint from the spec.

4. **No compile-time regression concerns.** Building the spike package from cold cache
   takes 6 seconds at 330 MB RSS — well within budget. The generic Mutation[T] type
   instantiates once per schema rather than requiring 40+ named methods per schema,
   which is exactly the type-graph collapse targeted by the hybrid spec.

### Path forward

**Phase 4B** — lean-generics templates for create.go / update.go / delete.go / query.go.
These are the easy-win categories; the mechanism is validated by Phase 3's where.go
result. New plan: `docs/superpowers/plans/YYYY-MM-DD-ent-code-reduction-phase-4b.md`.

**Phase 4C** — descriptor-driven template rewrite for mutation.go / client.go / entql.go.
This phase productionizes the spike: entc generates per-schema descriptors + thin
façades instead of hand-rolling them. Witness-test generator lands in this phase.

**Phase 4D** — final measurement on the 111-schema consumer. Gates per the hybrid spec:
consumer `go build ./ent/gen/...` peak RSS ≤ 5.5 GB, ≥ 40% LOC reduction.

Approach 2 (pure descriptor) and pure Approach 1 remain filed as fallbacks if the
template work reveals gaps in Phase 4C that the spike didn't anticipate.
