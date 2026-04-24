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

(Filled in at Task 16 after regenerating under POC where.tmpl.)

## Decision

(Filled in at Task 17.)
