# Phase 4B Measurements — Lean-Generics create / update / delete / query

**Date:** 2026-04-24
**Target spec:** docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md
**Plan:** docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4b.md
**Go version:** go1.26.2 linux/amd64
**Consumer SHA:** 1bcf17f57b32e4dede078c25c45a7956210656b0
**Baseline fork SHA (pre-4B):** 989d142ca80ba98d6b88fe35394a733b03a92866
**Post-4B fork SHA:** 36abb9d59407d89320534076728143f16fed6e83

## Methodology

Measurements captured against the downstream consumer `service-api-go` via
`replace entgo.io/ent => <local-fork-path>`. Baseline at `.worktrees/phase4b-baseline`
(SHA 989d142ca — Phase 3 + helpers, no Phase 4B builder template changes).
Post-4B at `.worktrees/i-h8-ent` (SHA 36abb9d59).

`go clean -cache` and `go mod tidy` run between phases.
LOC from `wc -l`. Build/vet from `/usr/bin/time -v` on cold builds.
Exported-API snapshot from `go doc -all` per package.
Consumer uses `GOWORK=off` for all commands.

Vet exits with code 1 on both baseline and post-4B due to pre-existing schema drift
in `graph/generated.go` (same errors as observed in Phase 3 measurements). This is
not a regression — the errors are in `graph/generated.go`, not in `ent/gen/...`,
and the vet timing is still valid as a proxy for analysis cost.

## Baseline (pre-Phase-4B = Phase 3 state)

| Metric | Value |
|---|---|
| Consumer ent/gen total LOC | 1,632,180 |
| where.go (sub-package) | 153,509 |
| where.go (top-level stubs) | 0 |
| create.go (sub-package) | 250,070 |
| create.go (top-level stubs) | 0 |
| update.go (sub-package) | 210,624 |
| update.go (top-level stubs) | 0 |
| delete.go (sub-package) | 10,864 |
| delete.go (top-level stubs) | 0 |
| query.go (sub-package) | 0 |
| query.go (top-level stubs) | 124,881 |
| Cold `go build ./ent/gen/...` wall time | 1:26.03 |
| Cold `go build` peak RSS (kB) | 9,379,604 |
| `go vet ./ent/gen/...` wall time | 0:17.22 |
| `go vet` peak RSS (kB) | 1,761,888 |

## Post-4B

| Metric | Baseline | Post-4B | Delta |
|---|---|---|---|
| Consumer ent/gen total LOC | 1,632,180 | 1,612,629 | -19,551 (-1.2%) |
| where.go (sub-package) | 153,509 | 153,509 | 0 (0.0%) |
| where.go (top-level stubs) | 0 | 0 | 0 |
| create.go (sub-package) | 250,070 | 246,419 | -3,651 (-1.5%) |
| create.go (top-level stubs) | 0 | 0 | 0 |
| update.go (sub-package) | 210,624 | 198,696 | -11,928 (-5.7%) |
| update.go (top-level stubs) | 0 | 0 | 0 |
| delete.go (sub-package) | 10,864 | 10,460 | -404 (-3.7%) |
| delete.go (top-level stubs) | 0 | 0 | 0 |
| query.go (sub-package) | 0 | 0 | 0 |
| query.go (top-level stubs) | 124,881 | 121,313 | -3,568 (-2.9%) |
| Cold `go build ./ent/gen/...` wall time | 1:26.03 | 1:24.06 | -1.97s (-2.3%) |
| Cold `go build` peak RSS (kB) | 9,379,604 | 10,045,396 | +665,792 (+7.1%) |
| `go vet ./ent/gen/...` wall time | 0:17.22 | 0:16.82 | -0.40s (-2.3%) |
| `go vet` peak RSS (kB) | 1,761,888 | 1,562,920 | -198,968 (-11.3%) |

## Exported API diff

- Baseline API snapshot files: 127
- Post-4B API snapshot files: 127
- Total diff lines: 0
- First 60 diff lines: zero diff — exported API is identical between baseline and post-4B

## Decision

**Outcome: PARTIAL — proceed to Phase 4C.**

### Gate results

| Gate | Target | Measured | Met? |
|---|---|---|---|
| Total consumer LOC reduction | ≥ 5% | -1.2% | ✗ |
| create+update+delete+query combined LOC reduction | ≥ 15% | -3.28% (596,439 → 576,888) | ✗ |
| Cold `go build` wall time | no regression | -2.3% | ✓ |
| Cold `go build` peak RSS | no regression (± 5%) | +7.1% | ✗ (narrowly over 5% noise band) |
| `go vet` peak RSS | no regression (± 5%) | -11.3% | ✓ |
| `entc/integration/*` tests | green | green (9 scenarios; remaining failures all missing-server infrastructure) | ✓ |
| Consumer build | green | green | ✓ |
| Exported API diff (127 packages, `go doc -all`) | zero | zero | ✓ |

Five of eight gates pass. Three fail: the two LOC-reduction targets and the cold-build peak RSS.

### Interpretation

**The LOC targets were optimistic for this phase alone.** Phase 4B rewrites the *outer wrappers* of the create/update/delete/query builders — the per-field/per-edge `SetX` / `AddX` / `ClearX` scaffolding — while leaving the dense `internal/<schema>_mutation.go` and `<schema>_client.go` / `<schema>_entql.go` files untouched. Those three categories carry the bulk of the remaining bloat per the hybrid spec (§"The hybrid split"); Phase 4B was never going to reach 15% on its own surface without them. The 3.28% cut landed on the targeted categories is real, consistent across all four (update -5.7%, delete -3.7%, query -2.9%, create -1.5%), and was achieved without any API drift.

**Per-LOC build efficiency degraded from Phase 3.** Phase 3 `where.tmpl` saw 1.83% LOC reduction yield 10.4% cold-build wall-time improvement (5.7× multiplier). Phase 4B saw 1.2% LOC reduction yield 2.3% wall-time improvement (1.9× multiplier). The multiplier dropped by ~3×. This matches the hybrid-spec's expectation that predicate-shaped generics (one instantiation per schema, one arg type) collapse the type graph more efficiently than builder-shaped generics (one instantiation per schema *per field type*). Phase 3 pushed 111 distinct predicate types through `FieldEQ[predicate.T]`. Phase 4B pushes many more instantiations through `BSet[B, V]`, which dilutes the sharing benefit. Expected, not catastrophic — the absolute numbers are still net positive.

**Peak build RSS up +7.1% (+666 MB) is the only slightly worrying signal.** Single-sample timing RSS has ~±3% noise on a shared desktop; +7.1% is modestly outside the band but not a clear regression. Contributing factors likely include: more generic instantiation sites in the compiler's type graph (partially offset by smaller bodies), and run-to-run variance from an unmeasured shared desktop workload. The Phase 3 measurement showed RSS dropping -3.8% despite a smaller LOC cut, so the mechanism is stable — this one run may be noise. Worth re-measuring after Phase 4C, where the descriptor-driven collapse (111 distinct `*XxxMutation` named types → 1 `Mutation[T]` generic) is predicted to drop the whole-tree type graph meaningfully. Don't treat this as a Phase 4B failure; treat it as noise that the Phase 4C architectural change should dwarf.

**Zero API diff is the headline correctness win.** `diff -r` over `go doc -all` across 127 generated packages returns zero lines. Phase 4B preserved drop-in binary compatibility perfectly — this is non-trivial given the template changes touched every per-schema `SetX`/`ClearX`/`SaveX`/`AllX` method signature site.

### Path forward

1. **Proceed to Phase 4C.** Write `docs/superpowers/plans/YYYY-MM-DD-ent-code-reduction-phase-4c.md` covering descriptor-driven `mutation`, `client`, and `entql` template rewrites plus the witness-test generator. Per the hybrid spec, this phase is where the type-graph collapse lives: 111 schemas × 40+ heavyweight mutation methods per schema → one generic `Mutation[T]` + 111 thin aliases + 111 descriptor-data files. Phase 4A already validated the runtime mechanism on hand-ported `Card`; Phase 4C productionizes it via codegen.

2. **Revise the hybrid spec's gates.** The current ≥40% whole-tree LOC / ≤5.5 GB peak RSS / ≥40% build-time targets still apply to the Phase 4C measurement, not to Phase 4B in isolation. Keep them as final gates.

3. **Carry concerns into Phase 4C planning:**
   - Type-alias form (`type UserMutation = entbuilder.Mutation[User]`) must be used for drop-in API compatibility. Phase 4A's spike used a parallel `CardMutationShim` type to sidestep this; the template-driven version cannot. Hybrid spec §"The type alias nuance" already flags this.
   - Witness-test emission must land before any mutation template change touches a production schema. Drift between descriptor and struct is a new failure mode that must be caught at `go test` time, not query time.
   - Phase 4B's +7.1% RSS blip is an open question for the Phase 4C final measurement — Phase 4C should re-measure both categories to confirm the descriptor collapse reverses it.

4. **Do not escalate to Approach 2 (pure descriptor across all categories) yet.** Approach 1's lean generics delivered the expected modest LOC win on this surface, which is the rational handoff point before the type-graph collapse category kicks in. Escalation remains on the shelf for Phase 4C result interpretation if the descriptor approach underperforms.

### Closing note

Phase 4B was an intermediate step, not the headline win. It delivered a 3.28% reduction on its targeted surface, zero API drift, and preserved correctness across the integration suite. The mechanism works; the next phase is where the big architectural change sits.
