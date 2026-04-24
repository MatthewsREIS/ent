# POC Measurements — `where.go` Compaction

**Date:** 2026-04-23
**Target spec:** docs/superpowers/specs/2026-04-23-ent-code-reduction-design.md
**Target plan:** docs/superpowers/plans/2026-04-23-ent-code-reduction-phase-1-3.md
**Go version:** go1.26.2 linux/amd64
**Consumer SHA:** 1bcf17f57b32e4dede078c25c45a7956210656b0
**Fork SHA (baseline):** 429e20e95df84143bc68158ed21ef4e8c35628fb

## Methodology

Measurements captured against the downstream consumer `service-api-go` with a local
`replace entgo.io/ent => <local-fork-path>` directive so the consumer's codegen uses
the exact templates in this fork's current HEAD.

The consumer required features from `moar-pakidge-again-conflict-fix` (which includes
`SnapshotDir` + `trimPrefix` template func) that are not yet merged into `i-h8-ent`.
The local replace used `/var/home/smoothbrain/dev/matthewsreis/ent-worktrees/moar-pakidge-again-conflict-fix`
for generation. The Fork SHA above is the `i-h8-ent` HEAD — the branch where POC
code-reduction changes will land. The fork architecture is additive: `i-h8-ent` will
incorporate those prerequisite features before the POC where.tmpl changes are applied.

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
| Cold `go build ./ent/gen/...` wall time | 1:27.48 (m:ss) |
| Cold `go build` peak RSS (kB) | 10,035,668 |
| `go vet ./ent/gen/...` wall time | 0:17.85 (m:ss) |
| `go vet` peak RSS (kB) | 1,582,396 |

## Post-POC

(Filled in at Task 16 after regenerating under POC where.tmpl.)

## Decision

(Filled in at Task 17.)
