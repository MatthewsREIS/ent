# bench-codegen

Measures ent codegen cost (generation wall + peak RSS, `go build` wall + peak RSS,
LOC) across in-repo fixtures and arbitrary downstream consumer schemas. Output is
newline-delimited JSON (NDJSON), one `Run` per fixture.

## In-repo fixtures (default)

```
go run ./cmd/bench-codegen -out /tmp/bench.jsonl
```

Runs the bench against the three in-repo fixtures defined in `fixtures.go`
(privacy, hooks, edgeschema). Each line of `/tmp/bench.jsonl` is one `Run`.

To run a single fixture:

```
go run ./cmd/bench-codegen -fixture privacy
```

The baseline for these fixtures (as of PR 0) lives at `internal/bench/baseline.jsonl`.
Subsequent epic PRs include a `## Measurement` section in the PR description with the
delta against this file.

## Consumer-scale measurement

Point the same tool at any directory that contains an `ent.Schema`-implementing Go
package. The bench will:

1. Copy the schema directory to a fresh temp module
2. Add a `replace entgo.io/ent => /path/to/this/repo` directive
3. Run `go run ./gen.go` to generate the ent package
4. Run `go build ./...` on the generated package
5. Record gen wall + RSS, build wall + RSS, total LOC, file count, top-20 largest files

```
go run ./cmd/bench-codegen \
  -schema /absolute/path/to/service/ent/schema \
  -label service-api-go \
  -out /tmp/service-baseline.jsonl
```

The `-label` flag is the `fixture` field in the output — pick a stable name so cross-PR
comparisons stay aligned.

## Interpreting the output

Each line is a JSON object:

```json
{
  "timestamp": "2026-05-15T18:30:00Z",
  "git_sha": "fe4d71fd8...",
  "fixture": "privacy",
  "gen_wall_ns": 12345678901,
  "gen_peak_rss_bytes": 234567890,
  "build_wall_ns": 56789012345,
  "build_peak_rss_bytes": 1234567890,
  "total_loc": 45000,
  "file_count": 78,
  "top_files": [
    {"path": "/tmp/bench-privacy-.../ent/internal/task_mutation.go", "loc": 1234}
  ]
}
```

Notes:
- `top_files` paths are inside the temp module, not the in-repo fixture. Compare by
  base name or by relative position from the temp module's `ent/` directory.
- Peak RSS units are normalized to bytes (Linux's `Maxrss * 1024`, macOS's native bytes).
- Non-POSIX platforms produce `peak_rss_bytes: 0` — the wall time and LOC numbers are
  still meaningful.

## Comparing two runs

Diff JSONL with `jq`:

```
jq -s '.[0] as $a | .[1] as $b | $b | {fixture, total_loc_delta: (.total_loc - $a.total_loc), gen_wall_delta_ms: ((.gen_wall_ns - $a.gen_wall_ns)/1000000|round)}' baseline.jsonl new.jsonl
```

(Adapt by fixture if running multiple — the snippet above assumes one-line files.)

## PR 6 measurement (per-entity sub-packages)

Fixture-scale numbers captured the same way as prior PRs:

```
go run ./cmd/bench-codegen -fixture privacy    > /tmp/pr6-privacy.jsonl
go run ./cmd/bench-codegen -fixture hooks      > /tmp/pr6-hooks.jsonl
go run ./cmd/bench-codegen -fixture edgeschema > /tmp/pr6-edgeschema.jsonl
grep -h '^{' /tmp/pr6-*.jsonl > internal/bench/pr6.jsonl
```

Comparison against `pr5.jsonl` (git_sha `97573bbcb`):

| Fixture    | Metric                    | PR 5     | PR 6     | Δ      |
|------------|---------------------------|----------|----------|--------|
| privacy    | total_loc                 | 9,878    | 10,515   | +6.4%  |
| privacy    | build_peak_rss (MB)       | 87.0     | 67.3     | -22.7% |
| privacy    | build_wall (ms)           | 661.2    | 674.6    | +2.0%  |
| hooks      | total_loc                 | 10,560   | 11,190   | +6.0%  |
| hooks      | build_peak_rss (MB)       | 81.8     | 70.7     | -13.6% |
| hooks      | build_wall (ms)           | 659.4    | 693.1    | +5.1%  |
| edgeschema | total_loc                 | 47,144   | 50,541   | +7.2%  |
| edgeschema | build_peak_rss (MB)       | 255.2    | 138.4    | -45.8% |
| edgeschema | build_wall (ms)           | 2,463.5  | 2,388.0  | -3.1%  |

(Full precision in the JSONL; MB = 1,048,576 bytes.)

### Fixture-scale findings

- **LOC up ~6–7%** across all three fixtures. Expected: each entity now ships
  in its own leaf sub-package, which adds per-package boilerplate (package
  clause, imports, facade re-exports) that is amortized poorly at low entity
  counts (3–13 entities here).
- **build_wall stays in the noise** at fixture scale — small fixtures fit in
  one compile process, so the parallel-compile win from sub-packages does
  not materialize.
- **build_peak_rss drops 14–46%** — the headline PR 6 win at fixture scale.
  Each sub-package is compiled independently, so the Go compiler holds far
  less per-package state in memory at any one time. The edgeschema fixture
  (largest, 13 entities) sees the biggest reduction because peak RSS in PR 5
  was dominated by a single large `ent` package.

### Consumer-scale measurement (deferred)

Consumer-scale numbers require:

1. Migrate consumer source via `go run ./cmd/ent-codegen-migrate -gen-package <pkg> <consumer-dir>`
2. Regenerate consumer ent via the consumer's `go generate`
3. Run `bash /tmp/bench-pr6.sh post-pr6` to capture the JSONL

This is deferred pending two follow-up items outside PR 6 scope:

- **Migration tool gap**: `.With<Edge>()` chains in the schema package need
  a syntactic fallback when type resolution can't load the schema package
  (mirrors the chain walker added for `Query<Edge>()` in commit `5de8a3cde`).
  Without this, schema files referencing the stale gen API can't be migrated.
- **External dependency**: `entgo.io/contrib/entgql`'s `gql_collection_*.go`
  template emits methods on `*<Entity>Query`, but those types now live in
  per-entity sub-packages. Fixing this requires a template update in the
  `entgql` repo.

PR 6 codegen correctness is independently proven via the `entc/integration`
fixture suite (Task 20).
