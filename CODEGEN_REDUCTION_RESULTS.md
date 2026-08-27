# Codegen Footprint Reduction — Results

Tracks the measured impact of replacing per-entity generated code with generic,
reflection/descriptor-driven runtime helpers. Each stage appends a section below.

## Stage 1: Generic Upsert

Replaced per-entity generated `XUpsert`/`XUpsertOne`/`XUpsertBulk` implementations
(one full copy of the setter/updater/adder/clearer method set per entity, per
field) with generic `entbuilder.Upsert`/`UpsertOne[ID]`/`UpsertBulk[ID]` builders
that take column identifiers instead of per-field methods. Generated per-entity
surface is now just: type aliases (`XUpsert`, `XUpsertOne`, `XUpsertBulk`), an
`xUpsertMeta` var, and `OnConflict`/`OnConflictColumns` shims.

Measured against the `gemini` consuming app (`models/` Go module — the largest
real-world ent schema available for this fork), before and after pointing
`models/go.mod`'s `entgo.io/ent` replace at the post-Stage-1 fork commit.

### Benchmark commands

Gemini's own codegen/build entrypoints (`api/Taskfile.yaml`), run identically
before and after:

```bash
# LOC
cd models && find gen -name '*.go' -print0 | xargs -0 cat | wc -l

# generation time + peak RSS (task api:generate-go runs `go generate .` in
# models/ through profile.sh, which wraps /usr/bin/time -v)
task api:generate-go

# clean build time + peak RSS (task api:validate-go runs `go build ./...` in
# models/ through profile.sh)
go clean -cache && task api:validate-go
```

### Results

| Metric | Before | After | Delta |
|---|---|---|---|
| Generated LOC (`models/gen`) | 2,035,883 | 1,821,380 | **-214,503 (-10.5%)** |
| Generation wall time | 62.53s | 63.91s | +1.38s (+2.2%) |
| Generation peak RSS | 8.6 GB | 11.0 GB | +2.4 GB (+27.9%) |
| Clean build wall time | 65.70s | 61.78s | -3.92s (-6.0%) |
| Clean build peak RSS | 2.8 GB | 2.7 GB | -0.1 GB (-3.6%) |

Single-run measurements (not averaged over multiple trials); generation and
build times in particular carry normal system-load noise on a shared box.
Generated LOC is exact (`wc -l` over `models/gen/**/*.go`).

Generation peak RSS rose non-trivially. It is not from the upsert change's own
generated output shrinking (fewer files/lines generated, if anything, should
lower generator RSS) — the likely driver is `go run entc.go` doing a
from-source rebuild of the local-filesystem-`replace`d fork (including its new
`entbuilder` package and the added generics/reflection surface) rather than
pulling a cached module build, which is expected for local-path testing and
would not apply once gemini switches back to a pushed pseudo-version. This
should be re-checked once the fork branch is pushed and gemini's `replace` is
swapped from a local path to a real module version.

### Migration

Grepped gemini for call sites (`grep -rln "Upsert\|OnConflict" --include='*.go' . | grep -v '/gen/'`),
found ~60 matches, and rebuilt/vetted every affected module
(`models`, `api`, `workers`, `shared`, `db-optimizer`) after swapping the
`entgo.io/ent` replace to the local fork worktree. Only one file needed actual
migration — the rest used `OnConflict`/`OnConflictColumns`/`Ignore`/`DoNothing`/
`UpdateNewValues`/`Exec`/`ID`, all of which are unchanged shims, or were
type/variable names/comments that happened to match the grep pattern:

- `workers/wrike_workflow_status_sync_worker.go` (2 call sites): chained
  `.UpdateName()` on a `WrikeWorkflowStatusUpsertOne` → `.UpdateFields(wrikeworkflowstatus.FieldName)`.

`go build ./...` and `go vet ./...` (which also type-checks `_test.go` files)
are clean across every gemini module that references `entgo.io/ent`.

### Semantic deviations

- **Upsert setters are now untyped, column-based calls validated by the
  database, not the compiler.** `Set`/`Add`/`Clear`/`Update`/`UpdateFields`
  take a `field.Column`-style identifier (e.g. `x.FieldName`) plus an `any`
  value instead of a generated `SetName(v string)` method. A wrong type or a
  column that doesn't belong to the entity is no longer a compile error — it's
  a runtime error from the SQL driver/database at `Exec`/`ExecX` time.
- **`ExecX` now runs validation that upstream's generated `ExecX` bypassed.**
  Upstream's per-entity `ExecX` skipped checks for missing required
  conflict-target options and child-entity conflict shapes that `Exec` (the
  error-returning form) performed. The generic builder runs that validation
  unconditionally, so a call that previously silently fell through to a plain
  `INSERT` (masking a misconfigured upsert) now panics fail-fast via `ExecX`
  instead.

### Test results

- `go test ./...` in `models/` — all pass (`entmerge`, `entmultipicklist`,
  `entorderindex`, `entscopes`, `entsearch`, `entsoftdelete`, `entui`,
  `fieldsecurity`, `enthubspot`, `migrate/indexreorder`, `salescompsnapshot`,
  `schema`, `searchsynonyms`, `txutil`, `utils`).
- `go test ./...` in `workers/jobs` — pass.
- `go test ./...` in `workers` (root package) — one pre-existing failure,
  unrelated to this change: `TestProvisionWorkspaceWorker_NoGithubClient_DoesNotCreateWorkspace`
  failed with `pq: could not extend file "base/44341/43421": No space left on
  device`. This is a workspace-provisioning test with no upsert involvement;
  the failure is sandbox/infra (see below), not a regression from the
  migration.
- `go build ./...` and `go vet ./...` clean (including test files) in
  `models`, `api`, `workers`.
- api integration tests that specifically exercise `OnConflict` upsert paths
  (`api/integration/chatter/chatter_on_conflict_integration_test.go`,
  `api/integration/pinned/pinned_and_field_selection_on_conflict_integration_test.go`,
  `api/integration/timeline/timeline_on_conflict_integration_test.go`) could
  **not** be run to completion: the local IntegreSQL container
  (`mreis-test-integresql`) returned `500 Internal Server Error` on template
  creation for every test, and `task api:test-db-prune` (the documented reset
  for exactly this state) also failed with a 500 from the same service. This
  reproduced consistently across repeated attempts and is a pre-existing,
  session-local test-infra problem (the IntegreSQL/Postgres test containers
  were already running from a prior session, in a degraded state) — not
  something introduced by this migration. It could not be resolved without
  restarting containers, which was out of scope for this task.

### Concerns / follow-ups for the user

- The generation peak-RSS increase (+27.9%) should be re-measured once
  gemini's `go.mod` points at a pushed pseudo-version of the fork branch
  instead of a local filesystem `replace`, to rule out from-source rebuild
  overhead as the cause.
- The on-conflict integration tests could not be exercised end-to-end in this
  session due to broken local test infra (IntegreSQL 500s); re-run them once
  the test containers are healthy to get direct confirmation of upsert
  runtime behavior, not just compile-time parity.
- `models/go.mod`, `api/go.mod`, and `workers/go.mod` currently have a local
  filesystem `replace entgo.io/ent => .../worktrees/say-less` left in place
  for this benchmarking/migration pass, per this task's scope (gemini changes
  are intentionally left uncommitted for review). Reverting to a pushed
  pseudo-version, and committing the gemini-side changes, is deferred to the
  user/next task.
