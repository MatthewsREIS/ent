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

(filled in Task 20.)
