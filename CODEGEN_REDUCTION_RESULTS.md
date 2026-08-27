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
would not apply once gemini switches back to a pushed pseudo-version.

**Re-measured after the swap** to the pushed pseudo-version
(`v0.0.0-20260827190939-1131aa8509a1`): generation was **60.38s wall /
9.1 GB peak RSS** — i.e. -3.4% wall and +5.8% RSS vs baseline. The +27.9%
figure was indeed dominated by the local-replace from-source rebuild; the
real generation cost of this change is roughly noise on wall time and a
small RSS increase.

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
  instead. `ID()`/`IDX()` likewise now error/panic on missing conflict options
  where upstream silently performed a plain `INSERT` and returned the ID.
- **Per-entity upsert aliases now collapse to identical types across
  entities.** `XUpsertOne = entbuilder.UpsertOne[ID]`, so every entity that
  shares an ID type (e.g. all `int`-ID entities) shares one concrete Go type;
  every `XUpsert` is the same `entbuilder.Upsert` type regardless of entity.
  Cross-entity compile-time distinctness is lost — a function declared as
  `func(*EscrowUpsertOne)` will happily accept a `*ParcelUpsertOne` argument.
  There is no runtime state-sharing hazard (each builder still holds its own
  `UpsertConfig` closure), only a loss of the compiler catching a
  wrong-entity-type mistake. A phantom-type parameter (`UpsertOne[E, ID]`,
  with `E` an uninstantiated marker type per entity) could restore that
  distinctness in a later stage if needed.

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
  **Resolved in a follow-up session**: after the test containers were
  recreated and gemini was regenerated against the pushed pseudo-version,
  `task api:test-integration -- -run OnConflict -count=1` ran **47 tests, all
  passing** (63.1s), and the workers suite (previously failing only on the
  dead container port) passed clean. Upsert runtime behavior is now
  confirmed end-to-end in gemini, not just compile-time parity.

### Concerns / follow-ups for the user

- ~~Re-measure generation peak RSS against a pushed pseudo-version~~ —
  done: 60.38s / 9.1 GB (see re-measurement note above); local-replace
  artifact confirmed.
- ~~Run the on-conflict integration tests once test infra is healthy~~ —
  done: 47/47 passing (see integration-test note above).
- ~~Swap the local filesystem `replace` to a pushed pseudo-version~~ —
  done: `models`/`api`/`workers` go.mod now replace to
  `github.com/MatthewsREIS/ent v0.0.0-20260827190939-1131aa8509a1`
  (branch `worktree-say-less`, PR #20).
- Remaining: gemini-side changes (go.mod replaces, regenerated `gen/`, one
  migrated worker file) are intentionally left uncommitted in
  `gemini/.worktrees/main` for user review and commit.
