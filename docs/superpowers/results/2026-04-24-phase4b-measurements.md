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

(To be filled in by the controller.)
