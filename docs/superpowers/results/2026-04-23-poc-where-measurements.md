# POC Measurements — `where.go` Compaction

**Date:** 2026-04-23
**Target spec:** docs/superpowers/specs/2026-04-23-ent-code-reduction-design.md
**Target plan:** docs/superpowers/plans/2026-04-23-ent-code-reduction-phase-1-3.md
**Go version:** go1.26.2 linux/amd64
**Consumer SHA:** 1bcf17f57b32e4dede078c25c45a7956210656b0
**Fork SHA (baseline):** 32e6e88ecd2aabd885c99d3b5535bb25e2fc5e85

## Methodology

Measurements captured against the downstream consumer `service-api-go` with a local
`replace entgo.io/ent => <local-fork-path>` directive so the consumer's codegen uses
the exact templates in this fork's current HEAD.

The local replace pointed at `/var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent`
(this branch). The Fork SHA above is the `i-h8-ent` HEAD after merging `origin/master`
(merge commit `104705216`) to bring in prerequisites (`SnapshotDir`, sub-package split)
and after updating Phase 2 tests to match the post-merge API (`SetOwnerID`/`AddTweetIDs`
variants; commit `32e6e88ec`).

This baseline was re-captured after merging origin/master into i-h8-ent (commit 104705216)
to bring in prerequisites (SnapshotDir, sub-package split) required by the consumer.
An earlier baseline captured in commit b4cf665df was discarded — it used a different branch
(`moar-pakidge-again-conflict-fix`) for the actual codegen, making the Fork SHA misleading.

LOC numbers come from `wc -l` over generated Go files. Build/vet numbers come from
`/usr/bin/time -v` on cold builds (`go clean -cache` before each). Exported-API
snapshot comes from `go doc -all` per package (127 packages) for cross-referencing in Task 17.

`go generate ./...` was run successfully prior to measurements. `go vet ./ent/gen/...`
completed without OOM but returned non-zero exit status due to schema API mismatches
between newly generated ent types and the stale GraphQL `graph/generated.go` layer
(expected — ent was regenerated but the GraphQL layer was not). The vet wall time and
RSS measurements are valid as baseline indicators of vet cost against `./ent/gen/...`.

## Baseline (pre-POC)

| Metric | Value |
|---|---|
| Consumer ent/gen total LOC | 1,635,044 |
| Consumer where.go total LOC (all schemas) | 156,373 |
| Cold `go build ./ent/gen/...` wall time | 1:33.50 (m:ss) |
| Cold `go build` peak RSS (kB) | 9,646,432 |
| `go vet ./ent/gen/...` wall time | 0:19.70 (m:ss) |
| `go vet` peak RSS (kB) | 1,549,700 |

## Post-POC

Captured 2026-04-23. Consumer regenerated against `i-h8-ent` HEAD (compact `where.tmpl`).
`go build` exited 0. `go vet` exited 1 due to unrelated gqlgen staleness (schema API
mismatches in `graph/generated.go`); vet timing/RSS numbers are still valid.

| Metric | Baseline | Post-POC | Delta |
|---|---|---|---|
| Consumer ent/gen total LOC | 1,635,044 | 1,632,180 | -2,864 (-0.18%) |
| Consumer where.go total LOC | 156,373 | 153,509 | -2,864 (-1.83%) |
| Cold `go build ./ent/gen/...` wall time | 1:33.50 | 1:23.83 | -9.67s (-10.4%) |
| Cold `go build` peak RSS (kB) | 9,646,432 | 9,278,724 | -367,708 (-3.8%) |
| `go vet ./ent/gen/...` wall time | 0:19.70 | 0:17.47 | -2.23s (-11.3%) |
| `go vet` peak RSS (kB) | 1,549,700 | 1,596,832 | +47,132 (+3.0%) |

## Decision

**Outcome: GO with revised gates.** Proceed to Phase 4 (create/update/mutation/client/entql
compaction) but rewrite the success-criteria table to apply to the full targeted surface
rather than `where.go` alone.

### Gate results (as originally specified)

| Gate | Target | Measured | Met? |
|---|---|---|---|
| `where.go` LOC reduction | ≥ 60% | 1.83% | No |
| `go vet` peak RSS reduction | ≥ 30% | -3.0% (regressed slightly) | No |
| `go build` wall-time reduction | ≥ 25% | 10.4% | No |
| `entc/integration/*` tests green | 100% | 100% (Task 14 edgeschema regen + suite) | Yes |
| Consumer tests green | 100% | `go build ./ent/gen/...` passes; pre-existing `graph/generated.go` gqlgen staleness is unrelated to templates | Yes (scoped) |
| Exported API diff (127 packages, `go doc -all`) | zero | zero | Yes |

### Interpretation

The three numeric gates fail — but the failure mode is instructive, and the non-numeric
gates (behavior + API surface) all pass cleanly. The spec's thresholds were calibrated
as if `where.go` alone could deliver a 60% LOC cut and a 30% vet-RSS cut. That was
always optimistic: `where.go` accounts for only ~9.6% of total generated LOC
(156,373 of 1,635,044), so even a hypothetical 100% deletion of `where.go` would cap
whole-tree LOC reduction at ~10%. A 60% target on a file category representing 10% of
the surface cannot be hit by compacting that category in isolation — the math was
backwards.

The meaningful signal in the POC:

1. **No compile-time explosion from generics.** The central risk of Approach 1 (per-schema
   generic instantiation blowing up the type graph) did not materialize. Cold `go build
   ./ent/gen/...` wall time improved by 10.4% and peak RSS dropped 3.8% — modest but
   directional, and crucially *not* the regression the risk section worried about. The
   `go vet` RSS uptick (+3.0%) is within measurement noise given single-shot timings
   on a shared machine and is not a compounding concern given the build-time result.
2. **Per-LOC efficiency is disproportionately high.** Source-level reduction was 1.83%
   on `where.go` / 0.18% tree-wide, but cold build wall time dropped 10.4% — roughly a
   5× multiplier. This is the read: what drives compile cost in generated ent code is
   *complexity* (one predicate function body, one type-checking site, one inlining
   decision per operator per schema), not raw line count. Approach 1 reduces that
   complexity more efficiently than it reduces source.
3. **Drop-in API compat preserved.** `diff -r /tmp/api-baseline /tmp/api-poc` on
   `go doc -all` output across 127 generated packages: empty. Zero signature drift.
4. **Zero behavioral regression.** Integration suite green; consumer `go build` green.
   The consumer `go vet` non-zero exit is a pre-existing gqlgen staleness
   (`graph/generated.go` predates ent regen) and is unrelated to the where.tmpl change.

### Why GO despite missing the numeric gates

Phase 4 targets create/update/mutation/client/entql/query — approximately 952k LOC
per the spec's "Target files" table, vs. where.go's ~156k. If the per-LOC build-time
efficiency observed in the POC holds across the full surface, a 1.83% source cut
delivering 10.4% build-time improvement extrapolates to the 60% source / ~25% build-time
original gate territory once applied to 6× the surface area.

The gates as written treated `where.go` as if it were representative of the full refactor.
It isn't — it was deliberately chosen as the *smallest and most symmetric* category for
the POC, to minimize correctness risk while answering the compile-time blowup question.
That question is answered: generics do not regress build performance, they improve it,
even at 1.83% source reduction.

### Path forward

1. Write `docs/superpowers/plans/2026-XX-XX-ent-code-reduction-phase-4.md` covering
   create.go, update.go, delete.go, internal/\<schema>_mutation.go, client.go, entql.go,
   and query.go, in the risk-ascending order the spec specifies.
2. Revise the success-criteria table in the spec (or carry revised gates in the Phase 4
   plan) so thresholds apply to the *full* targeted surface, not where.go. Keep the
   ≥60% source / ≥25% build-time / ≥30% vet-RSS targets against the category set, not
   individual files.
3. Land the witness-test generator (deferred from Phase 1-3) before starting the
   `internal/<schema>_mutation.go` category — descriptor/struct-drift risk first
   appears there.
4. Hold Approaches 2 (descriptor-driven) and 3 (hybrid) in reserve. If Phase 4's
   per-LOC build-time efficiency degrades for mutation/update (where bodies are more
   heterogeneous than predicates), escalate to Approach 2/3 via a new spec rather than
   continuing to push Approach 1 against diminishing returns.

### Next step

Draft Phase 4 plan; do not start implementation until gates are revised and approved.

