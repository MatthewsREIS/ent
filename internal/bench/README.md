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
