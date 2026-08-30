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

## Stage 2a: Field Handles (predicate/order funcs → `F`/`E`)

Replaced every per-field predicate/order func (`NameEQ`, `IDIn`, `HasXWith`,
`ByX`, `NameContains`, …) and per-edge `Has`/`HasWith`/`ByCount` func in
generated entity packages with two typed struct values per package: `F`
(`F.<Field>.<Op>`, e.g. `F.Name.EQ(v)`, `F.CreatedAt.GT(t)`, `F.ID.In(ids...)`)
and `E` (`E.<Edge>.Has()` / `.HasWith(...)` / `.OrderByCount(...)` /
`.OrderBy(term, ...)`). `And`/`Or`/`Not`, `predicate.X`, field/edge constants,
and `OrderOption` (now an alias of `entfield.Order`) are unchanged. A
manifest-driven call-site rewriter (`tools/handlerewrite`, scope-shadowing
aware) migrates consumers automatically.

Measured against gemini again — this time the dedicated worktree
`gemini/.worktrees/codegen-reduction` (fresh off `origin/main`, since the
worktree used for Stage 1 review, `.worktrees/main`, had been reclaimed by
unrelated concurrent work and its uncommitted Stage 1 state was lost; the
lost mechanical migration — go.mod replaces plus the two `UpdateName()` →
`UpdateFields(...)` call sites in `workers/wrike_workflow_status_sync_worker.go`
— was reapplied from the Stage 1 record before starting Stage 2a).

**Environment confound, separate from and additional to the local-replace
caveat below:** because `.worktrees/main`'s state was lost, Stage 2a's
measurements come from a *different gemini worktree checked out at a newer
`origin/main` commit* than whatever `origin/main` snapshot Stage 1 was
measured against — normal `main`-branch development landed in between, not
anything this task changed. **All of the deltas below are therefore
confounded by ordinary app-code (and potentially schema) drift on top of the
codegen change itself, not just the codegen change in isolation.** This
matters most for the clean-build wall-time/RSS deltas (+14.0%/+15.2%): build
cost scales with how much application code exists to compile, and some
unknown share of that increase is almost certainly newer gemini code added
to `origin/main` since Stage 1's snapshot, not the field-handle migration.
**Those two numbers must not be read as a regression caused by this stage's
codegen change.** The generated-LOC and generation-time/RSS deltas share the
same confound to a lesser degree (schema drift between the two `origin/main`
snapshots would shift generated LOC independent of the F/E handle change),
though the LOC drop and RSS drop are large enough that the codegen change is
plausibly still the dominant driver there.

### Benchmark commands

Same entrypoints as Stage 1:

```bash
# LOC
cd models && find gen -name '*.go' -print0 | xargs -0 cat | wc -l

# generation time + peak RSS
task api:generate-go

# clean build time + peak RSS
go clean -cache && /usr/bin/time -v go build ./...   # in models/
```

### Results

**Baseline is Stage 1's recorded post-migration numbers** (LOC 1,821,380; gen
60.38s/9.1GB re-measured against the *pushed* pseudo-version; build
61.78s/2.7GB), not a fresh "before" build in this worktree — this worktree
never held a pre-Stage-2a `gen/` tree to measure (it's a clean checkout, and
`gen/` is gitignored), so there is nothing to "clean-build before" here that
would be honest. **Comparison-basis caveat, carried forward from Stage 1:**
unlike that 60.38s/9.1GB Stage-1 number, the Stage 2a "after" generation
numbers below are measured under a **local filesystem `replace`** of both
`entgo.io/ent` and `entgo.io/contrib` at their worktree/clone paths (this
fork branch is unpushed) — Go rebuilds the fork/contrib from source on every
`go generate`/build rather than pulling a cached module, which inflates wall
time and (in Stage 1's case) RSS. No pushed-pseudo-version re-measurement was
possible this round.

| Metric | Before (Stage 1 recorded) | After (Stage 2a, local-replace) | Delta |
|---|---|---|---|
| Generated LOC (`models/gen`) | 1,821,380 | 1,577,213 | **-244,167 (-13.4%)** |
| Generation wall time | 60.38s | 120.11s | +59.7s (+98.9%, local-replace artifact — see caveat) |
| Generation peak RSS | 9.1 GB | 3.1 GB | **-6.0 GB (-65.9%)** |
| Clean build wall time | 61.78s | 70.41s | +8.6s (+14.0%) |
| Clean build peak RSS | 2.7 GB | 3.11 GB | +0.41 GB (+15.2%) |

Single-run measurements, same caveats as Stage 1 (shared-box noise, not
averaged), **plus the newer-`origin/main`/environment confound described
above** — every delta in this table compares two different gemini worktree
checkouts, not just two ent forks against the same app snapshot. The
generation-RSS drop is the standout number, and — being measured under the
*more expensive* local-replace path (the scenario Stage 1 showed inflates
RSS) — is if anything a conservative floor on the real improvement; a
pushed-pseudo-version re-measurement would likely show an equal or larger
drop. Generation wall time and clean-build wall/RSS upticks are plausibly
dominated by the same local-replace from-source-rebuild effect Stage 1
identified, or by the environment confound (more/different app code to
generate against and compile), or both — not necessarily by the F/E handle
change itself — but this round had no pushed commit and no same-commit
"before" build to re-measure against to split those apart. **In particular,
the clean-build wall-time/RSS deltas (+14.0%/+15.2%) must not be read as a
regression from the codegen change alone.**

**Re-measured on a quiet machine** (after the original benchmarking session's
concurrent build/test workloads triggered an OOM crash — the crash-era numbers
above were taken under that load): generation **112.4s wall / 3.2 GB peak
RSS**, clean build **63.8s wall / 3.17 GB peak RSS**. The generation-RSS drop
(~3 GB vs Stage 1's 9.1 GB) reproduces exactly; the clean-build wall delta
shrinks from +14.0% to +3.3% vs Stage 1's confounded baseline; generation
wall stays ~2 min under the local-replace from-source path (still to be
re-measured against a pushed pseudo-version after merge).

### Migration

Built the rewriter from the fork (`go build -o handlerewrite
./tools/handlerewrite`, run from the say-less worktree) and ran it over
gemini's non-generated Go:

```
handlerewrite -manifest models/gen/handle_manifest.json \
  -pkgprefix github.com/MatthewsREIS/gemini/models/gen/ \
  models api workers shared
```

Touched **418 non-generated `.go` files** in one pass; a second identical
invocation made zero further changes (idempotent, full first-pass coverage
for everything it targets). The remaining work was two categories:

**1. Gemini-owned template fixes** (in scope — `models/templates/**` and
`models/extensions/**`, hand-written generators that emit into `models/gen`
using the now-deleted funcs; these aren't call sites the rewriter touches,
they're the templates generating new call sites on every run). Ten templates
needed fixes, all the same swap shape (`pkg.Field(v)` → `pkg.F.Field.EQ(v)`,
`pkg.HasXWith(v)` → `pkg.E.X.HasWith(v)`, `pkg.ByID()` → `pkg.F.ID.Asc()`,
etc. — mirroring the pattern contrib's own `entgql` template fix used):

- `models/templates/entcontext.tmpl` — 6 call sites (RLS/auth-context
  helpers: office lookup by city, office-user physical-office query, 4 user
  lookups by email/okta_id/fusionauth_id).
- `models/extensions/entsearch/templates/indexing.tmpl` — 1 (single-entity
  reindex lookup by ID).
- `models/extensions/entsearch/templates/reindex_entities.tmpl` — 3 (keyset
  pagination `IDGT`+`ByID()`, batch `IDIn`, single-entity `ID`).
- `models/extensions/entsearch/templates/search_pagination.tmpl` — 1
  (`IDIn` for Meilisearch-result-ID batch fetch).
- `models/extensions/enthubspot/templates/inbound_workflow_association_workers.tmpl`
  — 5 (UUID/HubSpot-ID entity resolution, two `HasXWith` association-exists
  checks used twice each for create/delete).
- `models/extensions/enthubspot/templates/outbound_validation.tmpl` — 1.
- `models/extensions/enthubspot/templates/validation_helpers.tmpl` — 3
  (duplicate-hubspot-id check, duplicate-field check, duplicate-field
  check-excluding-self).
- `models/extensions/enthubspot/templates/inbound_standard_workers.tmpl` — 1.
- `models/extensions/enthubspot/templates/validation.tmpl` — 4 (same shape
  as `validation_helpers.tmpl`, separate per-node vs `$.` template scope).
- `models/extensions/enthubspot/templates/outbound_workers.tmpl` — 14 (by
  far the largest: entity-by-ID lookups across 5 distinct call shapes, two
  `IDIn` association batch fetches, two `HubspotIDNotNil` niladic checks).

Also caught **one contrib gap** the migration exposed but that gemini
doesn't own: `entgql/template/node.tmpl` / `node_entity.tmpl` (the Relay
Node-lookup emissions used by *every* Node-interface entity) still referenced
the deleted `Package.ID(id)`/`Package.IDIn(ids...)` — missed by contrib's own
F/E migration commit, which only touched `where_input.tmpl`/`pagination.tmpl`
(+ `_subpkg` variants). This broke all 157 generated `gql_node_*.go` files
until it was fixed upstream in contrib (`11cd13c6`, applied by the user
between rounds of this task — not by this session, contrib being read-only
here).

**2. Rewriter misses on already-migrated files** (hand-fixed directly, not
template-driven — genuine gaps in the rewriter's static analysis):

- **Cross-sibling-closure false-positive shadowing.** The rewriter's
  scope-shadowing guard is coarser than real Go block scoping: a local
  variable named e.g. `property` declared inside one `if` block (or one
  `t.Run(func(t *testing.T) {...})` closure) was treated as shadowing the
  `property` package import for the rest of the *enclosing function* —
  including sibling blocks/closures where the shadow has already gone out of
  scope and the identifier is genuinely still the package. Two occurrences:
  `api/resolvers/transactional_create.resolvers.go` (`property.IDEQ(...)`,
  `company.IDEQ(...)`, both after their respective shadowing `if` blocks had
  closed) and `api/integration/commission/commission_integration_test.go`
  (`commission.ID(...)` ×2, in a `t.Run` subtest with no local `commission`
  var, while sibling subtests elsewhere in the same file do declare one).
- **A JSON-typed field's niladic predicate has no `F` handle at all** (the
  F/E handle scheme doesn't generate a handle for JSON fields — see semantic
  deviations below) — `marketagentregion.MaterializedPropertyWhereNotNil()`
  in `models/schema/property_hooks.go` had no drop-in replacement; hand-wrote
  it as `predicate.MarketAgentRegion(sql.FieldNotNull(marketagentregion.FieldMaterializedPropertyWhere))`
  (the same primitive the deleted func's generated body used internally).
- **Edge `HasXWith` calls the rewriter simply missed** in one file:
  `api/integration/notification/notification_rls_integration_test.go`
  (`notification.HasUserWith(...)`, `notification.HasChatterNotificationWith(...)`)
  — both edges exist on the package's `E` struct with matching
  `StructField` names; no shadowing or JSON-type explanation found, this
  looks like a plain miss (unlike the other two categories, no root cause
  was identified beyond "the rewriter didn't touch this file's `HasXWith`
  calls" — everything else in the file, including the `F.ID.EQ` calls on the
  same lines, was correctly migrated).

All four hand-fixed non-template files were caught by `go build ./...` /
`go vet ./...` (the latter also compiles `_test.go` files, which is how the
notification and commission test-file misses surfaced — plain `go build`
does not compile test files and would have missed both).

### Semantic deviations

- **`IsNil`/`NotNil` are exposed on every handle, including non-optional
  (`NOT NULL`) fields.** The old codegen only emitted nil-check predicates
  for nullable fields; the generic `F.<Field>` handle types all carry
  `IsNil()`/`NotNil()` regardless. Calling one against a `NOT NULL` column is
  a always-true/always-false predicate at the SQL level, not a compile error
  — narrower than before, not wider, so no existing call site breaks, but a
  new caller no longer gets a compile-time signal that a field can't be null.
- **The full string-operator set (`Contains`/`HasPrefix`/`HasSuffix`/
  `EqualFold`/`ContainsFold`) is now exposed on every `String[T]` handle**,
  not gated per-field by codegen annotations the old per-field-func generator
  respected. Same direction as the `IsNil`/`NotNil` point — a superset, not a
  narrowing.
- **`Value[T]` (used for UUID/opaque-typed fields, including every `ID`
  field) loses the string-substring ops** (`Contains`/`HasPrefix`/etc.) that
  a `~string`-underlying custom type could get from the old per-field
  generator; `Value[T]` only exposes `EQ`/`NEQ`/`In`/`NotIn`/`GT`/`GTE`/`LT`/
  `LTE`/`IsNil`/`NotNil`/`Order`/`Asc`/`Desc`. No gemini call site used
  string ops against a `Value`-typed field (all such fields are UUIDs), so
  this is a theoretical narrowing with no observed impact.
- **`OrderOption` is now a type alias of `entfield.Order`** (itself
  `func(*sql.Selector)`), not a gemini/ent-generated named type. Assignable
  everywhere the old `OrderOption` was; no call-site changes needed for
  ordinary `.Order(pkg.F.Field.Asc())` usage.
- **Predicate/order funcs are gone; `F`/`E` handles replace them** — every
  call site changes shape (`pkg.NameEQ(v)` → `pkg.F.Name.EQ(v)`,
  `pkg.HasOwnerWith(p)` → `pkg.E.Owner.HasWith(p)`, `pkg.ByName()` →
  `pkg.F.Name.Asc()`), migrated automatically by the manifest-driven
  rewriter for ordinary call sites; see Migration above for the categories
  that needed hand-fixing.
- **JSON-typed fields have no `F` handle at all** — not even `IsNil`/
  `NotNil`, which the old codegen *did* emit for a nullable JSON column (its
  bare `<Field>NotNil()`/`<Field>IsNil()` funcs). This is a real (if narrow)
  loss of generated surface, not a superset like the two points above: a
  caller that needs a JSON-field null check has to hand-write
  `predicate.X(sql.FieldNotNull(x.FieldY))` (or `sql.FieldIsNull`) — see the
  `property_hooks.go` hand-fix above, the one gemini call site affected.
- **Spec's "deterministic suffix on collision" rule is superseded.** The
  original stage-2 spec called for a deterministic disambiguating suffix if
  a field/edge name collided with something else in a package; the actual
  `F`/`E`-struct nesting design makes that impossible by construction (field
  names live under `pkg.F.*`, edge names under `pkg.E.*`, so a field and an
  edge can share a bare name with zero collision risk). No gemini schema
  exercises this either way, but it's a deviation from the spec worth
  recording.
- **Contrib's `entgql` Relay Node templates were an incomplete migration**
  (see Migration above) — not a fork/gemini deviation per se, but a gap this
  task's regen surfaced and that had to be closed upstream in contrib before
  gemini could build at all. Recorded here since it's the kind of gap the
  next consumer of this fork should expect to hit if they diff against an
  older contrib commit.
- **`BasicType` conversion loss for non-`Valuer` custom Go types.** The old
  per-field codegen converted a field's custom Go type to its underlying
  basic type before handing the value to the sql helper (`time.Time(v)` for
  a custom time type, `v.String()` for a `Stringer`-backed string type,
  `v[:]` for an array-of-bytes type) whenever that type didn't itself
  implement `driver.Valuer`. `Value[T]`/`Number[T]` pass the declared Go
  type straight through to the driver instead — for a hypothetical
  non-`Valuer` custom time/`Stringer`-string/array-bytes `GoType` field,
  that's a runtime driver error (`sql: converting argument ... unsupported
  type`) where the old codegen silently normalized it to something the
  driver accepts. No call site in gemini or the fork uses such a `GoType`
  today (confirmed by grep), so this is a theoretical narrowing, not an
  observed regression — worth a test if a schema ever adds one.
- **`sql/schemaconfig` (multischema) routing was initially dropped, then
  restored.** The first cut of the `where.tmpl`/`entfield.Edge` rewrite lost
  the `internal.SchemaConfigFromContext(s.Context())` /
  `step.To.Schema` / `step.Edge.Schema` routing that base's generated
  `Has<Edge>()`/`Has<Edge>With()` inlined for graphs with the
  `sql/schemaconfig` feature — a silent wrong-schema-query bug for any
  consumer using `AlternateSchema`, caught in stage 2a's final review
  (not by any test, since the fork's own `multischema` integration package
  is pre-existing-broken for unrelated reasons and so never ran). Fixed by
  adding `entfield.NewEdgeSteps` (a `stepMods []func(*sql.Selector,
  *sqlgraph.Step)` hook applied to the freshly built step before use) and
  having `where.tmpl` emit a stepMod that reproduces
  `dialect/sql/feature/schemaconfig.tmpl`'s own schema-name resolution
  verbatim when the feature is enabled; parity with base was verified line
  for line across every edge in `entc/integration/multischema/ent/user/where.go`.
  One deliberate deviation from base: the stepMods also run for
  `OrderByCount`/`OrderBy` (edge-neighbor ordering), so those now get
  schema routing too — base never applied it there. Strictly more correct
  for schemaconfig graphs, not a narrowing.

### Test results

- `go build ./...` and `go vet ./...` (includes `_test.go` type-checking)
  clean across all four gemini modules that reference `entgo.io/ent`:
  `models`, `api`, `workers`, `shared`.
- `go test ./...` in `models/` — all pass (same package set as Stage 1).
- `go test ./...` in `workers/` (root + `jobs`) — all pass; the Stage-1
  `TestProvisionWorkspaceWorker_NoGithubClient_DoesNotCreateWorkspace`
  disk-space infra failure did not recur this round.
- `task api:test-integration -- -run 'OnConflict' -count=1` — **47 tests, 47
  passing** (same count as Stage 1's post-migration re-run).
- `task api:test-integration -- ./integration/ent_resolvers/... -count=1`
  (where-input-heavy resolver-integration package, chosen as the
  where-input-surface exerciser per this task's brief) — **286 tests, 285
  passing, 1 pre-existing skip, 0 failures**.

### Concerns / follow-ups for the user

- Generation-time/RSS numbers above are local-replace-only this round (no
  pushed pseudo-version existed to re-measure against, unlike Stage 1) — the
  wall-time regression in particular should be re-checked once this fork
  branch (or a version pinning it) is pushed, the way Stage 1 did.
- The contrib `entgql` node-template gap (now fixed upstream at `11cd13c6`)
  is worth a note to whoever next diffs/rebases contrib's field-handle
  branch: it was not caught by contrib's own test suite before this gemini
  regen surfaced it, meaning contrib's coverage of the Relay Node-lookup
  path may be worth strengthening.
- **The rewriter's shadowing guard needs tightening before stage 2b trusts
  it over a larger call-site set.** It is deliberately conservative, but at
  whole-*function* granularity rather than real Go block scoping — a local
  variable shadowing a package name inside one `if` block (or one
  `t.Run(func(t *testing.T) {...})` closure) gets treated as shadowing the
  package for the rest of the enclosing function, including sibling
  blocks/closures where the shadow has already gone out of scope. This
  produced "cross-sibling-closure false shadowing" misses this round (e.g.
  `api/resolvers/transactional_create.resolvers.go`, where `property`/
  `company` locals declared in one `if` block caused genuinely-still-package
  usages later in the function to be skipped) — silent under-migration
  rather than a build break at rewrite time, only caught by the subsequent
  `go build`/`go vet` pass. Worth fixing to real block-level scope tracking
  before stage 2b runs the rewriter over a set large enough that these
  misses aren't all caught by a full build+vet pass.
- Remaining: gemini-side changes (go.mod replaces, regenerated `gen/`, the
  reapplied Stage 1 worker migration, 10 gemini template fixes, and the
  handful of hand-fixed call sites listed above) are intentionally left
  uncommitted in `gemini/.worktrees/codegen-reduction` for user review and
  commit. Nothing was committed in gemini or contrib by this task.

## Stage 2b: Assignment Handles (`With(F.X.Set(...), E.X.SetID(...), ...)`)

Completes stage 2: every per-field/per-edge builder setter (`SetX`,
`SetNillableX`, `AddX`/`AppendX`, `ClearX`, and the edge `SetXID`/
`SetNillableXID`/`AddXIDs`/`RemoveXIDs`/`ClearX` family) is gone from
generated builders. The replacement is a single `With(as
...entfield.Assignment) *XCreate/XUpdate/XUpdateOne` method per builder,
fed by the same `F`/`E` handles Stage 2a introduced for predicates/order —
`F.<Field>.Set(v)`/`.SetNillable(p)`/`.Add(v)`/`.Append(v)`/`.Clear()`,
`E.<Edge>.SetID(v)`/`.SetNillableID(p)`/`.AddIDs(vs...)`/`.RemoveIDs(vs...)`/
`.Clear()`. JSON fields now live in `F` too (`entfield.NewJSON`); an FK
column exposed as its own field (e.g. `ContactID` backing the `contact`
edge) gets an `entfield.EdgeField` under `F` that routes to the edge
internally — only edges with **no** matching FK-column field (`RecordType`,
`Seller`, `BrokerOfRecord`, `CapitalMarketsAgent`, …) use the plain
`E.<Edge>.SetID(...)` form. Measured the same way as 2a: against the
`gemini` app, this fork's largest real-world consumer.

### Benchmark commands

Same entrypoints as Stage 1/2a:

```bash
# LOC
cd models && find gen -name '*.go' -print0 | xargs -0 cat | wc -l

# generation time + peak RSS
task api:generate-go

# clean build time + peak RSS
go clean -cache && /usr/bin/time -v go build ./...   # in models/
```

### Results

**Baseline is Stage 2a's recorded quiet-machine numbers** (LOC 1,577,213;
gen 112.4s/3.2 GB; clean build 63.8s/3.17 GB) — same worktree as 2a (no
environment/commit confound this round, unlike 2a vs Stage 1).

| Metric | Before (Stage 2a, quiet-machine) | After (Stage 2b, re-measured) | Delta |
|---|---|---|---|
| Generated LOC (`models/gen`) | 1,577,213 | 1,393,985 | **-183,228 (-11.6%)** |
| Generation wall time | 112.4s | 126.0s | +13.6s (+12.1%) |
| Generation peak RSS | 3.2 GB | 3.2 GB (3,312,820 KB) | flat |
| Clean build wall time | 63.8s | 72.9s | +9.1s (+14.2%) |
| Clean build peak RSS | 3.17 GB | ~3.0 GB (3,164,820 KB) | flat-to-down |

Measurement history for this row, in the order taken (all single runs on
the shared dev box, not averaged):

1. **First pass (discarded as load-confounded)**: gen 134.8s, build 103.6s
   (+62% build wall) — taken immediately after the heavy Docker/Postgres/
   testcontainers test phase on the same box.
2. **Second pass (also discarded)**: gen 155.2s, build 79.9s — load average
   was still 7–14 at start (residual test-phase churn).
3. **Third pass (the table numbers)**: taken after waiting for load to
   settle to ~2–3, test containers idle but resident. This is the best
   measurement available this round, comparable to (though not quite as
   idle as) 2a's quiet-machine recheck.

Reading: the generation-time increase (+12%) is plausibly real — the
`With`/`F`/`E`-handle emission adds per-field decision logic to
`setter.tmpl` where the old codegen just stamped out one setter method per
field. The clean-build increase (+14%) showed the same direction across
all three runs, so some of it is likely real too, despite the 11.6% LOC
drop — the deleted per-field setter methods were among the cheapest lines
in the package to compile (tiny monomorphic methods), while what remains
is denser per line. Both RSS numbers are effectively flat (the earlier
"3.09 GB" figure in this section's first draft was a mis-conversion of
3,164,552 KB, which is the same ~3.0–3.2 GB plateau every 2b run landed
on). Net across stage 2 as a whole vs the Stage 1 numbers, generation
memory is the big win (9.1 GB → 3.2 GB) and LOC is down 31.5% cumulative;
wall-clock times have not improved: clean build is ~62→73s (+18%, part
environment), and generation ~60→126s — though most of that gen-wall gap
is the local-replace measurement artifact 2a already documented (`go run
entc.go` rebuilds the fork from source under a `replace` directive instead
of pulling a cached module; re-check against a pushed pseudo-version).
The compile-time payoff, if it comes, is expected from the later stages'
further LOC removal rather than from stage 2's per-line densification.

### Migration

Built the rewriter fresh from the fork worktree
(`go build -o handlerewrite ./tools/handlerewrite`) and ran it in `-chains`
mode separately per Go module (each module's own `go.mod` scopes its
`go/packages` load):

```
handlerewrite -manifest models/gen/handle_manifest.json \
  -pkgprefix github.com/MatthewsREIS/gemini/models/gen/ \
  -chains ./schema/...        # from models/    — 21 files
  -chains ./dealhooks/...     # from models/    —  3 files
  -chains ./...                # from api/       — 397 files
  -chains ./...                # from workers/   —  46 files
```

467 non-generated `.go` files rewritten across the four modules; **zero
ambiguity refusals** this round (no `SetNillable<X>` collisions between a
plain `<X>` field and a `Nillable<X>` field anywhere in gemini's schema).
The tool correctly resolved the F-vs-E routing for every FK-column-backed
edge it touched (verified against the generated `where.go` handle structs
throughout), including cases this session's own hand-fixes initially got
wrong (see below) — the manifest-driven tool is more reliable than manual
inference for that specific judgment call.

**Template fixes** (the "~11 known" emitters the brief called out, all
found and fixed; a fresh sweep of every `models/extensions/*/templates` and
`models/templates` file for `.Set[A-Z]`/`.Clear[A-Z]`/`.Add[A-Z]`/
`SetNillable` turned up no further offenders beyond this list):

- `models/templates/entcontext.tmpl` — user email/okta-id update chain,
  Okta-field-sync closures (rewritten from per-field `setter func(string)
  *UserUpdateOne` to `assign func(string) entfield.Assignment`, batched into
  one `With(assignments...)`), office-user upsert.
- `models/extensions/entfake/fake.tmpl` — per-field fake-data setter loop
  → per-field `create.With(pkg.F.X.Set(...))` (nillable and non-nillable
  branches).
- `models/extensions/entmerge/templates/merge_go.tmpl` — pre-clear/create
  chains for merge; also required a **non-template** fix in
  `entmerge/extension.go` (added `EntityPackage` to `MergeConfig`, sourced
  from `entc/gen.Type.Package()` — a method, not a field, so the template's
  `{{ .Package }}` auto-call needed an explicit `()` in the plain-Go
  `extension.go` caller) so the template has an import path for the F/E
  handles.
- `models/extensions/enthubspot/templates/outbound_workers.tmpl`,
  `inbound_workflow_association_workers.tmpl`, `inbound_standard_workers.tmpl`,
  `owner_discovery.tmpl` — HubSpot-ID store/clear chains, association-junction
  creates (edge-name-derived, not field-derived — reverted an initial
  over-complicated approach back to the simpler `F.<FieldToStructField>.Set`
  form once the field/edge distinction below was nailed down).
- `models/extensions/enthubspot/templates/outbound_workers_test.tmpl`,
  `inbound_workflow_association_workers_test.tmpl`,
  `sync_initial_associations_test.tmpl` — the three fixed-shape (non-`$node`-
  looped) HubSpot integration-test templates; `sync_initial_associations_test.tmpl`
  alone had ~35 `Create()`/`UpdateOne()` chains across ContactList/Contact/
  ContactListContact, machine-transformed with a small one-off Python script
  driven by a field→(kind,name) table rather than hand-edited one at a time.

**A same-name-shadow hazard is baked into HubSpot's test-template package
convention specifically**: several of these templates create entities named
`contact`/`contactList` while importing the `contact`/`contactlist` F/E
packages under their bare names — every `x, err := pkg.Entity.Create()`
declaration is safe (Go's short-var-decl scope begins *after* the
declaring statement, so the RHS still sees the package), but a *second*
`With(pkg.F...)` call reusing an already-declared identically-named local
is not. Fixed via a second aliased import of the same package
(`contactfield "…/gen/contact"`) scoped to just the ambiguous call sites,
leaving the bulk of same-file, same-line-declared usages untouched.

**Rewriter miss classes** (hand-fixed; none were data-corrupting, all were
either compile errors — safe — or, in one case below, silently wrong code
caught immediately by the type checker):

1. **Conditional reassignment after the initial chain** (by far the most
   common miss, dozens of sites across `models/schema`, `models/dealhooks`,
   `api/resolvers`, `api/cmd`, and `workers`). `-chains` folds the *initial*
   fluent `Create()`/`UpdateOne(...).With(...)` chain correctly, but a
   later `builder = builder.SetX(v)` inside an `if`/`else` (a very common
   idiom for optional-field carry-over in this codebase's `convert_*`
   resolvers) is a separate statement the tool doesn't touch. Fixed by hand
   as `builder = builder.With(pkg.F.X.Set(v))` at each site;
   `api/resolvers/convert_escrow.resolvers.go`,
   `convert_helpers.go`, and `convert_proposal_helpers.go` alone accounted
   for ~90 of these (large Escrow/Listing/Proposal→Deal/Transaction field
   carry-over resolvers with dozens of `if src.X != nil { builder =
   builder.SetNillableX(...) }` blocks each).
2. **Local variable/parameter shadows the field-handle package** — the
   exact hazard Stage 2a's results doc flagged as needing a real
   block-scope fix before 2b trusted the rewriter at a larger scale. It
   recurred, but not from the *rewriter* mis-firing this time (0
   corruptions from the tool itself) — it showed up in **hand-written code
   the rewriter correctly left alone** because the shadow made the call
   site a plain builder-method call, not a chain: a range-loop variable
   `contact`/`company`, a function parameter `listing *gen.Listing`, or a
   `:=`-declared local `marketing`/`app`/`user`/`workspace` already in
   scope when a *later* statement needed `pkg.F`/`pkg.E`. Fixed per-site by
   renaming the local (preferred — no import changes) or, where the name
   was load-bearing across many call sites in the same function/file,
   adding a second aliased import of the same package scoped to the
   colliding lines only (Go permits importing one path under two names in
   the same file). Representative sites:
   `api/cmd/generate/generate_data.go` (`contact`/`company` loop vars),
   `api/resolvers/listing.resolvers.go` (`listing *gen.Listing` local,
   renamed to `lst` for the whole function), `api/resolvers/listing_workflow.go`
   (`listing` parameter, renamed to `lst`), `api/testharness/fixtures/auth.go`
   (`user` parameter, renamed to `u`), `workers/okta_workers_test.go` /
   `workers/app_share_worker_test.go` / `workers/workspace_branch_cleanup_hook_test.go`
   (aliased second imports).
3. **Function-parameter/type-param receivers** — a builder passed through
   as `*gen.XUpdateOne` (the plain top-level-package type alias, not the
   entity subpackage) with `SetX` calls inside the callee; same underlying
   cause as #2 (the parameter name matches or the type doesn't carry enough
   context for the tool's static pass) but worth naming separately since
   it's a signature shape, not a shadow. `models/schema/property_field_selection_hooks.go`,
   `api/scim/apply.go`.
4. **A standalone builder-method call reached through a function call
   expression**, not a plain variable — `create().SetUserID(...)` where
   `create` is itself a `func() *gen.DealTeamCreate` parameter
   (`models/schema/deal_owner_runtime.go`). The chain-folding pass needs a
   simple identifier or selector on the left to recognize a chain; a call
   expression receiver falls outside that.

**One transcription bug this session made and self-corrected via compiler
feedback**, worth flagging as a style gotcha for future migrators — **not**
a compiler safety net, per the final whole-branch review's M7 finding: the
first pass through several hand-fixed edge-adjacent sites
(`officeuser.Office`/`.User`, `contactlistcontact.ContactList`/`.Contact`,
`contactlist.CreatedBy`) used the edge form (`E.<Edge>.SetID(...)`) by
analogy with genuinely edge-only relations, when these particular columns
are *field*-backed (`OfficeID`, `UserID`, `ContactListID`, `ContactID`,
`CreatedByID` all have their own `F` handle of type `entfield.EdgeField`,
mirroring the deleted `SetOfficeID`/`SetContactID`/etc. setters exactly).
This session's `go build` did catch every instance immediately — but not
because `E.<Edge>.SetID` doesn't exist for an `F`-only column. It exists
either way: `where.tmpl` emits an `E` entry for **every** edge regardless of
whether the manifest also emitted the `F`-form `EdgeField`
(`entc/integration/edgeschema/ent/friendship/where.go` carries both
`F.UserID` and `E.User` for the same FK column), and both forms route
through the same `SetEdgeID` call — they are interchangeable, not
type-distinct. What actually failed to build in this codebase was
incidental: gemini's `-chains` manifest happened to omit the `F`-form for
these specific columns for an unrelated reason, so only `E` compiled there.
On a schema where both forms are manifested (the common case, per the
integration corpus above), guessing wrong between `E.<Edge>.SetID` and
`F.<EdgeField>.Set` is **not** a compile error — both compile and both
write the same edge. Rule of thumb: if the old codegen's method was
literally `Set<Something>ID(...)`, reach for the package's `where.go` `F`
struct for style consistency with the old call site, not because `E` would
fail to compile.

**Immutability + ID-write runtime enforcement (Task 3's structural→runtime
move): zero collisions.** No hook, extension template, or hand-written call
site in gemini calls `m.SetField`/`m.ClearField` on an immutable field
during an update op, and none write the ID field on an update op — the new
runtime guard closed on nothing (grep-confirmed before the full test pass,
then confirmed again by 0 unexpected failures in `go test`/integration
runs).

**Left un-migrated at task time — since closed by a follow-up pass**: 21
of the ~85 `api` test packages (mostly `api/integration/*`) still failed to
`go vet` (compile) when this task's report was written — every failure was
one of miss-classes 1–2 above, just not hand-fixed in the first pass given
the volume (199 files still contained `Set[A-Z]`/`Clear[A-Z]`/`AddXIDs`-shaped
text after the rewriter pass, against 283 files the rewriter *did*
successfully migrate in `api/integration` alone). The post-review follow-up
pass (42 files) fixed all 21, plus one additional broken package it found
(`api/cmd/go_migrations/migrations`) and one miss-class-4 site
(call-expression receiver in `escrow_trs_hook_integration_test.go`) —
`go vet ./...` is now clean across all three gemini modules, and two of the
previously-broken write-heavy packages pass their full suites
(`transaction` 117/117, `escrow` 158/158). The plan's Task 5 test list itself names four items (models+workers
suites, `-run OnConflict`, `ent_resolvers`, one mutation-heavy integration
package) — all satisfied. Beyond that, the packages actually exercised and
clean this round: `models`, `workers`, `api/resolvers`, `api/scim`,
`api/crexiimport`, `api/cmd/generate`, `api/hubspot_email_recipients`,
`api/hubspot_email_stats`, `api/testharness`, and the integration packages
`chatter`, `office`, `pinned`, `ringcentral`, `soft_delete`, `timeline`,
`ent_resolvers`, and `contact`. `contact` stood in for the brief's
"contact or transaction" write-heavy pick since `api/integration/transaction`
is one of the 21 still-broken packages (`transaction_total_outstanding_integration_test.go`,
miss-class 1). Broken package list: `app`, `app_data`, `box`, `commission`,
`contract`, `dashboard`, `dataimport`, `deal`, `email`, `escrow`,
`marketing`, `misc`, `offer`, `property`, `scopes`, `search`, `sendgrid`,
`tloxp`, `transaction`, `workspace_git_token`, `wrike`.

### Semantic deviations

All deviations recorded in stage 2a's section still hold at gemini scale;
this stage's own deviations, all inherited rulings from Tasks 1–3 of the
2b plan and re-verified against the real consumer app rather than newly
discovered here:

- **Clear-on-all-handles.** Every `F`/`E` handle exposes `.Clear()`
  regardless of whether the underlying field/edge is nullable at the schema
  level — a deliberate simplification (one method shape for every handle)
  over the old codegen's nullable-only `ClearX` emission. No gemini call
  site clears a non-nullable field/edge (would fail at the DB/constraint
  layer exactly as before). For a plain non-nullable field/edge this is
  additive surface, not a narrowing — but the final whole-branch review's
  C1 finding caught a real gap this claim missed: **immutable** edges had
  no runtime enforcement at all (`EdgeSpec` lacked the `Immutable` flag
  `FieldSpec` already carried), so `E.<Edge>.SetID`/`.Clear()` and the
  matching `F.<EdgeField>.Set` on an `Update`/`UpdateOne` builder compiled,
  returned a `nil` error from `Save`, and wrote nothing — a silent no-op,
  not a DB/constraint-layer failure. Fixed in the post-review wave:
  `EdgeSpec.Immutable` plus a `checkEdgeImmutable` guard in
  `SetEdgeID`/`AddEdgeIDs`/`RemoveEdgeIDs`/`ClearEdge` now reject the write
  on `Update`/`UpdateOne` with a loud error, mirroring `checkImmutable` on
  the field side. Net effect vs the old codegen: strictly better on
  enforcement *location* (a runtime error at `Save`, always reachable, vs a
  compile error only where the old per-edge setter was structurally
  omitted) and the same on *outcome* (the write is rejected either way).
- **`Number[T].Set` is Reset-then-Set, not a raw overwrite** — matches the
  old codegen's create-path semantics exactly; harmless on create, and on
  update it means a `Set` after an `Add`/`AppendX` in the same `With(...)`
  call list keeps last-write-wins ordering by argument position (verified:
  no gemini call site relies on `Add` position after `Set` in the same
  assignment list).
- **Typed-value `Set`, no implicit string conversion** — `F.<Enum>.Set(v)`
  takes the enum's declared Go type directly; nothing in gemini's schema
  hits the `BasicType`-conversion-loss deviation stage 2a already recorded
  (no non-`Valuer` custom time/`Stringer`-string/array-bytes `GoType`
  field exists in this app, confirmed by grep as in 2a).
- **`(col, name)` split for `StorageKey`-diverging fields** carries a real
  assignment-side case in gemini (`file.fsize`-shaped columns, per Task 3's
  note) — confirmed no gemini schema has a field whose Go name and SQL
  column name diverge in a way that would have silently written to the
  wrong column under the old single-string scheme; nothing to migrate here
  beyond what generation already produces correctly.
- **Immutability + update-op ID-write enforcement moved from structural
  (deleted setter didn't exist) to runtime (`entbuilder` checks
  `FieldSpec.Immutable` / `Descriptor.IDField` in `SetField`/`AddField`/
  `AppendField`/`ClearField` and rejects ID writes on `OpUpdate`/
  `OpUpdateOne`)** — see the "zero collisions" note above; this is the
  deviation the brief specifically flagged to watch for extension/hook
  breakage, and none materialized.
- **`Number[T].Add` cannot express a negative delta on an unsigned field**
  (permanent narrowing vs. the old codegen, which took a signed delta on
  any numeric column) — no gemini schema field hits this (grep-confirmed:
  every unsigned numeric column in gemini's schema is only ever `Set`, not
  `Add`, in both old call sites and the migrated ones).
- **`With(...)` defers the first assignment error to `Save`** rather than
  panicking or erroring mid-chain — matches every gemini call site's
  existing error-handling shape (`_, err := builder.Save(ctx); if err !=
  nil { ... }`), so this is transparent to every consumer.
- **Contrib's monolithic `mutation_input.tmpl`** (the `entgql` sibling of
  the per-field-emitting template Task 2 fixed) was already found and fixed
  in Task 4 (`contrib` `72a92819`, routing through the generic
  `entbuilder.Mutation` `SetField`/`SetEdgeID` API instead of the deleted
  per-field mutation methods) — this task's full gemini regen is the
  validation pass for that fix, and it passes (`ent_resolvers`'s 286 tests
  exercise the GraphQL mutation-input path directly).
- **Gremlin "zero diff" claim, tightened wording**: Task 3's migration
  produced Immutable markers on internal mutation files with no behavioral
  change to any already-broken line (gremlin generation in this fork is
  pre-existing-broken for unrelated reasons, per Task 3's note) — this
  gemini regen doesn't exercise gremlin at all (gemini is SQL-only), so
  there is nothing further to confirm or contradict here.
- **String-typed edge-field columns lose string predicates** (recorded from
  the final whole-branch review's I4 finding). `where.tmpl` resolves any
  edge-backed FK column to `entfield.EdgeField[T]` before checking its
  underlying kind, and `EdgeField`'s predicate surface is the embedded
  `Value[T]` — method-for-method identical to `Number[T]` for a
  numeric/UUID FK column, but a *string*-typed FK column loses
  `Contains`/`HasPrefix`/`HasSuffix`/`EqualFold`/`ContainsFold`, which 2a's
  `String[T]` handle would have given it. Compile-error class (a call site
  using one of those methods on such a column simply won't build), and zero
  occurrences across both the integration corpus (40 `EdgeField` handles,
  all numeric/UUID) and gemini's schema.
- **Custom-GoType numeric-ish fields lose `Add` entirely** (recorded from
  the review's M3 finding). The manifest's `canAdd` gates on
  `HandleKind() == "Number"`; a field that supports `AddField` via
  `implementsAdder()` (a custom `GoType` with its own `Add` method, not a
  plain numeric kind) resolves to a non-`Number` handle, so `canAdd` is
  `false`, the old `Add<F>` setter is deleted from the manifest, and no
  `entfield` handle offers a replacement — the rewriter correctly leaves
  such a call site alone (compile error, safe direction), but the
  capability itself is gone with no substitute. Not present in the
  integration corpus (histogram: 85 `Number`-kind `canAdd` fields, 0
  non-`Number`) or gemini's schema.

### Test results

- `go vet ./...` (build + `_test.go` type-check) clean in `models/` (all
  packages) and `workers/` (root + `jobs`, including every `_test.go`).
- `go vet` clean in the `api` packages exercised this round: `resolvers`,
  `scim`, `crexiimport`, `cmd/generate`, `hubspot_email_recipients`,
  `hubspot_email_stats`, `testharness`, and integration packages `chatter`,
  `office`, `pinned`, `ringcentral`, `soft_delete`, `timeline`,
  `ent_resolvers`, `contact`. (21 other `api` integration packages were
  broken at the time of this task's report; the follow-up pass closed all
  of them — see Migration and Concerns.)
- `go test ./...` in `models/` — 27 tested packages, all pass.
- `go test ./...` in `workers/` (root + `jobs`) — all pass, **but only under
  restricted parallelism** (`-p 2 -parallel 4`); the default unbounded
  parallelism overwhelmed the shared dev box's IntegreSQL/Postgres
  testcontainer pool (`pq: could not write block ... wrote only 4096 of
  8192 bytes` — a Postgres I/O error from resource contention, not a code
  defect) and produced ~75 spurious failures that all pass individually and
  under the tuned run. `api:test-integration`'s own Taskfile comment
  already flags this exact tuning need for the IntegreSQL pool; `workers/`
  has no equivalent task-level tuning and needs a matching `-p`/`-parallel`
  cap if it's to be run unattended.
- `task api:test-integration -- -run 'OnConflict' -count=1` — **47 tests,
  47 passing** (same count as Stage 1 and Stage 2a).
- `task api:test-integration -- ./integration/ent_resolvers/... -count=1`
  — **286 tests, 285 passing, 1 pre-existing skip, 0 failures** (same count
  as Stage 2a).
- `task api:test-integration -- ./integration/contact/... -count=1`
  (write-heavy package per this task's brief — 2b changes every write
  path) — **109 tests, 109 passing, 0 failures**.

### Concerns / follow-ups for the user

- **Quiet-machine re-measurement: done** (third pass, table above). The
  first-pass +62.4% clean-build figure was indeed load; the settled numbers
  are gen +12.1% and build +14.2% vs the 2a quiet baseline, RSS flat. The
  build delta showed the same direction across all three runs so is
  partially real; a fully-idle box (containers stopped) would be needed to
  pin it more precisely, and the gen-wall figure remains inflated by the
  local-replace from-source artifact 2a documented.
- **The 21 broken `api/integration/*` packages: closed.** A post-review
  follow-up pass (42 files) applied the same documented miss-class fixes
  across all 21, plus one extra broken package it found
  (`api/cmd/go_migrations/migrations`). `go vet ./...` is now clean in all
  three gemini modules; `transaction` (117/117) and `escrow` (158/158)
  pass their full suites; a `gofmt` sweep over all 637 changed
  non-generated files is clean, and the two mid-chain-comment artifacts
  the review flagged were hand-tidied (gofmt does not normalize those).
- **The rewriter's cross-sibling-closure shadowing guard** (flagged by 2a
  as needing real block-scope tracking before 2b trusted it at scale) did
  not produce any *rewriter* corruption this round — 0 ambiguity refusals,
  0 observed mis-rewrites, and the tool's own F/E routing choices were
  verified correct throughout. The shadowing hazard that did recur this
  round was a different (adjacent) failure mode: hand-written code where a
  local variable already shadowed the package, which the tool correctly
  declined to touch (leaving a plain compile error, not silently wrong
  code) — see miss class 2 above. Tightening the guard remains worthwhile
  for the *false-shadowing* case 2a found, but this round found no evidence
  it under-fired.
- **Final-review fix wave, validated end to end.** The whole-branch review
  found two Criticals (immutable-edge writes silently dropped on update;
  `CreateBulk.Save` discarding a builder's `With()` error) and three
  Importants (GoType-backed bool/bytes fields unsettable; Immutable+
  UpdateDefault defaults dropped; IDField gate not covering delete ops) —
  all fixed (commits `0cb74cf2d..405379e23`), unit- and integration-tested,
  and the four failing `entc/integration` sub-trees were verified (via a
  throwaway worktree at the branch base) to fail identically before this
  branch — zero new breakage. Gemini was then regenerated against the fixed
  fork: all three modules vet clean, `transaction` 117/117 passes again,
  and generated LOC settles at **1,395,043** (the C1 edge-`Immutable`
  markers and C2 bulk guard add ~1k lines; the stage delta remains −11.5%,
  cumulative −31.5%).
- **One advisory `go vet` composites warning in generated code**
  (`models/gen/gql_node.go:21`: `&NotFoundError{"node"}` uses unkeyed
  fields): this fork aliases `NotFoundError = internal.NotFoundError`, so
  contrib's unchanged `node.tmpl` literal is now a composite of an imported
  struct. Cosmetic; fix belongs in contrib's template (keyed literal) when
  the contrib branch is next touched.
- Remaining: gemini-side changes (11 template fixes incl. one supporting
  `extension.go` change, 467 rewriter-migrated files, the hand-fixed
  miss-class sites documented above, the 42-file follow-up pass, and the
  post-fix-wave regen) are intentionally left uncommitted in
  `gemini/.worktrees/codegen-reduction` for user review and commit. Nothing
  was committed in gemini or contrib by this stage — only this results
  document, in the fork worktree.

## Stage 3: Reflection-Based Scan/Assign/String + Descriptor Dedupe (entql dropped in gemini)

Generated `sqlSave`/`createSpec` bodies are now generic
(`entbuilder.ApplyUpdateSpec`/`ApplyCreateSpec`, driven off each entity's
`Descriptor`); `ScanValues`/`AssignValues`/`String` delegate to a runtime
reflection scanner instead of per-entity generated switch statements; delete
builders are generic `entbuilder.Delete[T, I]` aliases (stage 5 of the fork
plan); and the fork now builds one shared `sqlgraph.Schema` off each
entity's descriptor instead of per-entity stub-graph literals — moot for
gemini specifically, since Task 6's investigation (`task-6-report.md`)
found gemini's only two entql consumers dead code (the mixin that called
them, `DealTeamScopedMixin`, was deleted a year ago when gemini's privacy
layer moved to Postgres RLS) and ruled **GO** on dropping the `entql`
feature outright rather than rebuilding them onto the shared graph. This
task migrates gemini onto that fork, executes the entql ruling gemini-side,
and re-measures.

### Benchmark commands

Same three entrypoints as every prior stage:

```bash
# LOC
cd models && find gen -name '*.go' -print0 | xargs -0 cat | wc -l

# generation time + peak RSS
MREIS_CODEGEN_ALLOW_WATCHER=1 /usr/bin/time -v task generate-go   # from api/

# clean build time + peak RSS
go clean -cache && /usr/bin/time -v go build ./...                # in models/
```

### Results

Baseline is Stage 2b's post-fix-wave settled numbers (LOC 1,395,043; gen
126.0s/3.2 GB; build 72.9s/~3.0 GB), same gemini worktree lineage.

| Metric | Before (Stage 2b) | After (Stage 3) | Delta |
|---|---|---|---|
| Generated LOC (`models/gen`) | 1,395,043 | 1,049,567 | **-345,476 (-24.8%)** |
| Generation wall time | 126.0s | 106.3s | -19.7s (-15.6%) |
| Generation peak RSS | 3.2 GB (3,312,820 KB) | 3.04 GB (3,189,264 KB) | -3.7% (flat-to-down) |
| Clean build wall time | 72.9s | 67.0s | -5.9s (-8.1%) |
| Clean build peak RSS | ~3.0 GB (3,164,820 KB) | 3.1-3.45 GB (3,257,848-3,614,892 KB) | up, see note |

Measurement history:

1. **LOC**: single count, deterministic (`find`/`cat`/`wc -l` over the
   regenerated `models/gen` tree) — 1,049,567.
2. **Generation**: two consistent runs — an informal check right after
   applying the entql feature-flag removal (106.57s wall, 3.0 GB) and the
   formal benchmark pass taken after the full test suite, on a box that had
   settled to load average 0.97/2.91/3.08 (1/5/15-min) at the start
   (106.32s wall, 3,189,264 KB peak). Both agree closely; no confound.
3. **Clean build**: first pass ran immediately after the generation
   benchmark, at load average 4.57 climbing to 6.00 — above the "wait for
   <2" bar, so treated as potentially confounded per the 2b three-pass
   protocol and re-run. A second pass, taken after waiting for load to
   settle back under 2 (1.59/3.73/3.53), landed within 0.16s of the first
   (66.96s vs. 66.80s) and at a similar-to-higher peak RSS (3,257,848 KB
   vs. 3,614,892 KB — the *quieter* run used *more* memory, the opposite of
   what ambient contention would predict). Conclusion: the elevated 1-min
   load average on the first pass was very likely an echo of the build's
   own 958-965% CPU utilization rather than third-party contention on this
   shared box (nothing else was consuming meaningful CPU in a `ps`
   snapshot taken mid-spike) — both passes are reported as consistent
   evidence, and the wall-clock delta above uses the second (quieter) pass.

Reading: LOC is the big, unambiguous win this stage (-24.8%, **-48.4%
cumulative vs Stage 1's original 2,035,883-line baseline** — the only
whole-codebase LOC figure this document ever gives: (2,035,883 -
1,049,567) / 2,035,883 = 48.44%) — the reflection-based scanner and generic
`Delete[T, I]` alias
remove the last large per-entity method bodies (`ScanValues`,
`AssignValues`, `String`, and every per-entity delete builder). Generation
time *dropped* this stage (-15.6%) rather than continuing stage 2's upward
trend — plausible, since this stage is pure deletion (no new per-field
template logic was added the way stage 2b's `With`/`F`/`E` machinery added
decision logic per field); the local-replace-from-source measurement
artifact 2a/2b documented still applies and would depress the *absolute*
gen-wall number for every stage equally, so the *direction* of this
stage's improvement should still be real. Clean build time also improved
(-8.1%) — consistent with dropping ~345K lines of monomorphic per-entity
methods, though build peak RSS moved the other way (up from the ~3.0 GB
baseline to a 3.1-3.45 GB range) — plausibly the reflection scanner's
`reflect.Type`/generic-instantiation-heavy code is denser per line for the
compiler than the deleted straight-line setter/getter bodies it replaced,
mirroring 2b's own build-RSS discussion. Separately, since Stage 2a's own
recorded LOC (1,577,213 — the baseline Stage 2b measured against, distinct
from and smaller than Stage 1's 2,035,883): LOC 1,577,213 → 1,049,567 is
**-33.5%** across Stage 2b + Stage 3 combined (not "all three stages" —
Stage 1 and 2a's own reductions are already baked into that 1,577,213
starting point). Build time: 72.9s → 67.0s is the first improvement in
build wall since Stage 1; generation memory holds at the ~3.0-3.2 GB
plateau every stage after Stage 1 has landed on.

### Migration

**Template sweep** (brief's Step 1): grepped every file under
`models/templates/` and `models/extensions/*/templates/` for
`\.Clear[A-Z]\w*\(\)`/`\.Remove[A-Z]\w*IDs\(` on update-builder receivers,
and for `schemaGraph`/`SchemaGraph`/`NodeIndex`/`entql` references anywhere
in template or extension Go source. **Zero hits** — 2b's own migration
already moved every extension emitter onto the `With(F.../E...)` handle
forms, so nothing in gemini's template surface still emits the deleted
edge-mutator methods or reaches into an entql-only symbol. The one action
this step actually required: delete `models/templates/delete_modifier_compat.tmpl`
(a leftover stage-5-era shim defining the now-nonexistent
`dialect/sql/delete/spec/modify` template hook with an already-`{{if
false}}`-disabled body). It had no separate registration line to remove —
gemini's `entc.go` loads every file in `models/templates/` via
`entc.TemplateDir("templates")`, so deleting the file is the whole fix.

**entql ruling execution** (brief's Step 3, Task 6's GO ruling): re-ran
Task 6's zero-caller grep against gemini's current `HEAD` per its own
"re-check before deleting" caveat — still zero callers outside
`models/schema/utils.go` itself. Applied the exact change list:

1. `models/entc.go` — dropped `"entql"` from `entc.FeatureNames(...)` and
   `gen.SplitInclude("entql*")` from `entc.Split(...)`.
2. `models/schema/utils.go` — deleted `IsUserOnEntityDealTeamPredicate` and
   `IsUserOnDealTeamPredicate` outright (dead code, zero callers), along
   with the now-unused `entql`/`sql`/`sqlgraph` imports and the
   `proposalEdge`/`listingEdge`/`escrowEdge` consts.
3. Regenerated. **334 stale `entql*.go` files did not disappear on their
   own** — `go generate` only writes what the current feature set asks
   for, it doesn't clean up files a prior generation with `entql` enabled
   left behind, and `models/gen` is entirely gitignored in gemini so `git
   status` never had a chance to show them as deleted the way the brief
   anticipated. Deleted them by hand (`find models/gen -iname '*entql*'
   -delete`); confirmed 0 remain after every subsequent regen. Also lost,
   as predicted: `internal.SchemaGraph`/`internal.NodeIndex` and the
   per-entity `XFilter`/`NewXFilterForMutation` facade aliases (all
   unreferenced once `utils.go` no longer needs them).
4. No fallout at the two `Filter()` call sites Task 6 flagged
   (`api/resolvers/closed_leases_invoices_summary.resolvers.go`,
   its integration test) — confirmed independent of entql (they call
   entgql's `WhereInput.Filter()`).

**Rewriter pass** (brief's Step 2/4): built `handlerewrite` from the fork,
ran `-chains` per module (`./schema/... ./dealhooks/...` from `models/`,
`./...` from `api/` and `workers/`) against the freshly-regenerated
`handle_manifest.json`. **Zero automated rewrites** in `models` and
`workers`; in `api` the tool correctly flagged, but could not
type-check-and-rewrite, exactly **one** call-site pattern duplicated across
two files — both instances of `TaskUpdateOne.ClearContactPhoneNumber()`
(the `contact_phone_number` edge), in `api/resolvers/task.resolvers.go`
(inside an `else if` branch: `upd = upd.ClearContactPhoneNumber()`) and its
matching integration test
(`api/integration/task/task_contact_phone_number_trigger_integration_test.go`,
a plain chain). Both are the **conditional-reassignment miss class** 2b's
report already named (the tool folds an initial fluent chain but not a
later `builder = builder.ClearX()` reassignment) — hand-fixed to
`upd.With(task.E.ContactPhoneNumber.Clear())` / `.With(task.E.ContactPhoneNumber.Clear())`
respectively. This confirms the brief's prediction almost exactly: a small
worklist, because 2b's rewriter pass already migrated nearly everything
else in gemini onto the `With(E...)` forms.

**Nondeterministic field/edge iteration order broke three gemini unit-test
files' `sqlmock` assertions** — not predicted by the brief, but predicted
by the branch's own ledger (`.superpowers/sdd/.../progress.md`:
*"`ApplyUpdateSpec`/`ApplyCreateSpec` iterate `Descriptor.Fields` (a
MAP) → SET/INSERT column order is nondeterministic per call... it broke
gemini's sqlmock arg-order expectations (Task 7 empirically hit it;
loosened those tests to `AnyArg` with explanatory comments — harmless
either way)"*), and this task did hit it exactly as anticipated: `go test
./schema/...` in `models` failed 9 tests across `escrow_seller_representation_close_hook_test.go`,
`ownership_primary_contact_hook_test.go`, and `wrike_task_test.go`, each
asserting a fixed argument order for a multi-column `INSERT`/`UPDATE` that
the generic entbuilder path now emits in Go's randomized map-iteration
order (confirmed empirically: re-running the same test showed a different
column order — and a different failure — on 3 consecutive runs). **First
fix attempt** (blanket `sqlmock.AnyArg()` on every position inside a
shuffled block) made the suite pass but, per a review round on this task,
lost more real regression coverage than the accompanying comments claimed:
`wrike_task_test.go`'s 8 tests ended up with no assertion at all of the
boolean their own docstrings say they verify (`photography_ordered`), and
the `ownership_*`/`escrow_seller_*` tests' comments overstated what the
downstream `properties_view` lookup actually checks (FK propagation only,
not `role`/`contact_role`/`primary_contact`). **Fixed properly**: replaced
the blanket `AnyArg()` with a small `sqlmock.Argument` matcher
(`timeOrTrue`/`valueAmong`, one per file) that accepts either a
`time.Time` or one of a fixed set of expected literals, applied to every
position in a shuffled block — this still tolerates the block's internal
reordering while asserting the *values* actually written: a regression
that flips `photography_ordered` to `false`, or writes the wrong
`contact_role`/`primary_contact`/`role`/edge-FK literal, now fails the
mock again, which blanket `AnyArg()` could not catch. Deterministic parts
(query-text regexes, single-arg `WHERE id = ?` lookups, and multi-id
`IN (...)` lists built from an ordered slice rather than a map) were left
as literals throughout, unchanged by either fix attempt. Re-ran the
affected tests 4+ consecutive times post-fix with no failures. The ledger
also records a **planned but not-yet-applied fork-side fix** (sort
field/edge names before iteration in both appliers, for deterministic SQL
text / prepared-statement plan-cache health under `lib/pq`) — verified
absent from this fork's `runtime/entbuilder/{create,update,sqlspec}.go` at
this task's `HEAD` (no `sort.` calls over `desc.Fields`/`desc.Edges`
anywhere); flagged below as a concern rather than implemented here, since
it's fork-side work outside this task's gemini-migration scope. Once that
fork-side fix lands, these matcher-based tests could be tightened further
to assert exact column-to-value pairing, though the matcher approach
already restores real value-level coverage without needing it.

### Semantic deviations

Carried forward from the fork's own task ledger
(`.superpowers/sdd/2026-08-28-stage3-reflection-dedupe/`), re-verified
against gemini where applicable:

- **Delete-builder cross-entity type collapse** (Task 5) — same upsert
  precedent as Stage 1: `UserDelete = entbuilder.Delete[User, int]` and any
  other entity sharing `int`'s ID type are the same generic instantiation.
  No gemini call site depends on `UserDelete` and e.g. `PetDelete` being
  distinct types.
- **SQL delete extension-template hooks unreachable** (Task 5) —
  `dialect/sql/delete/{additional,spec,fields/additional}/*` hooks were
  removed because a type alias can't gain extension methods; gemini's own
  `delete_modifier_compat.tmpl` (this task's deletion, above) is direct
  confirmation gemini never used that hook for anything live (its body was
  already `{{if false}}`-disabled before this task touched it).
- **`dialect/sql/defedge/spec/*` extension point unreachable** (Task 3,
  distinct from Task 5's SQL-delete hooks above) — the edge-spec-building
  path Task 3's generic `ApplyCreateSpec`/`ApplyUpdateSpec` replaced no
  longer calls through this extension-template hook; verified gemini has no
  extension or hand-written template that hooks it, so nothing gemini-side
  depended on it. Ledger-mandated doc note (`progress.md`'s Task 3 line:
  "defedge extension-point loss (doc note for results doc)").
- **`Modify()` unconditional on delete builders** (Task 5) — every entity's
  generic `Delete[T, I]` exposes `Modify()` regardless of the `sql/modifier`
  feature flag; harmless widening, no gemini call site is affected either
  way.
- **Hook-accessor sugar deferred (YAGNI)** — not built this stage; nothing
  in gemini's ~90-LOC-per-entity hook call sites needed it (re-examined per
  the plan's self-review, no whale left to shrink).
- **entql feature dropped in gemini** — Task 6's dead-code ruling, executed
  in full this task (see Migration above): the two helpers deleted outright
  rather than rebuilt onto the shared `internal.SchemaGraph`, since that
  graph is itself entql-gated and nothing calls the helpers.
- **Scanner interface shape** (Task 4) — `Descriptor.ScanFields` ranges all
  fields (not just mutation-settable ones, since edge-owned exposed fields
  must still scan); `FKColumns` is `[]FieldSpec` routed through generated
  setter methods; `AssignRow`'s unknown-callback path handles
  `selectValues`/FK fallback. No gemini schema field hits the documented
  edge cases (external `ValueScanner`, non-`Number`-kind custom-`GoType`
  `Add`, string-typed edge-field column) that would expose a capability
  loss from this shape.
- **edgeschema codegen non-idempotency** (Task 5 follow-up note,
  pre-existing) — reproduced independent of this task on a clean pre-Task-5
  tree; two consecutive `go generate` runs can shift privacy `Policy`/
  `Hooks` wiring before settling. Not gemini-specific (gemini doesn't use
  edgeschema) and not touched by this migration.
- **Deprecated `entbuilder` delete shims retained** (Task 5) —
  `DeleteDescriptor`/`DeleteIDDescriptor`/`BuildDeleteSpec`/`DeleteState`/
  `RunDelete` were restored as deprecated compat shims for `examples/`
  (frozen by the stage-1 restore rule) and any stale pre-Task-5 generated
  code; gemini's own regenerated code doesn't call them (confirmed: gemini
  builds and tests clean with the shims present but unused).
- **Nondeterministic field/edge map-iteration order** (new this task, see
  Migration above) — semantically harmless (every field/edge writes an
  independent spec entry) but produces unstable SQL text across calls and
  broke 3 gemini test files' positional `sqlmock` assertions; fixed
  gemini-side, fork-side deterministic-ordering fix still pending (see
  Concerns).

### Test results

- `go build`/`go vet ./...` clean in all three gemini modules (`models`,
  `api` — including every `api/integration/*` package touched by the
  targeted runs below plus a full `go vet ./...` sweep, `workers` — root +
  `jobs`), matching 2b's fully-clean bar. The one advisory `go vet`
  composites warning in generated code (`models/gen/gql_node.go:21`,
  `&NotFoundError{"node"}` unkeyed fields) is the same pre-existing,
  cosmetic, contrib-template-owned warning 2b already documented — not
  introduced by this stage.
- `go test ./...` in `models/` — all packages pass, including
  `models/schema` (the package with the sqlmock-brittleness fix above; ran
  4 additional times after the fix with no failures, confirming the fix
  holds across the map-iteration nondeterminism it's meant to tolerate).
- `go test -p 2 -parallel 4 ./...` in `workers/` — all pass (`workers` root
  + `workers/jobs`), same parallelism cap 2b needed for the shared
  IntegreSQL/Postgres testcontainer pool.
- `task test-integration -- -run OnConflict -count=1` — **47 tests, 47
  passing** (same count as every prior stage).
- `task test-integration -- ./integration/ent_resolvers/... -count=1` —
  **286 tests, 285 passing, 1 pre-existing skip, 0 failures** (same count
  as every prior stage).
- `task test-integration -- ./integration/transaction/... -count=1` —
  **117 tests, 117 passing, 0 failures** (write-heavy, matches 2b's
  post-follow-up count).
- `task test-integration -- ./integration/contact/... -count=1` — **109
  tests, 109 passing, 0 failures** (write-heavy, matches 2b's count).
- `task test-integration -- ./integration/chatter/... -count=1` — **66
  tests, 66 passing, 0 failures** — the brief's "scanner proof" pick
  (eager-loading-heavy), exercising `AssignRow`'s reflection path across
  every loaded edge without incident.
- `task test-integration -- ./integration/box/... -count=1` — **89 tests,
  89 passing, 0 failures** — the entql-drop-is-inert proof (found via
  `grep -rln "DealTeam" api/integration --include='*_test.go'`; `box`
  includes `box_folder_rls_integration_test.go` and
  `box_folder_rls_edge_cases_integration_test.go`, which exercise the
  Postgres-RLS-based deal-team scoping that replaced the entql-era
  `DealTeamScopedMixin` a year before this task). A clean pass here is
  exactly the empirical confirmation Task 6's ruling asked for.

### Concerns / follow-ups for the user

- **Fork-side deterministic-ordering fix: LANDED** (final-review fix wave,
  commit `2063754ce`): `ApplyUpdateSpec`/`ApplyCreateSpec` now sort field
  and edge names before iteration, and `EdgeIDs`/`RemovedEdgeIDs` sort node
  IDs within an edge — SQL statement text is deterministic again, pinned by
  50-iteration determinism tests. The same wave hardened `FormatEntity`
  against a non-pointer-Nillable panic, made the generated `SchemaOf` panic
  loudly on an unknown SchemaConfig key instead of yielding
  `"<invalid Value>"` (schemaconfig-gated — gemini has that feature off, so
  its pending regen delta from this wave is nil), and applied four minor
  robustness/doc fixes. Since the ordering fix lives in the shared runtime,
  gemini picks it up immediately through its `replace` directive with no
  regen; its three sqlmock test files could optionally restore literal
  positional args now that order is stable (the value-set matchers they use
  remain correct either way).
- **Build-benchmark load-average anomaly, resolved.** The first clean-build
  pass ran at 1-min load 4.57 (above the "<2" bar) immediately after the
  generation benchmark; a second pass after the load genuinely settled
  landed within 0.16s of the first, at *higher* peak RSS despite the
  quieter box — strong evidence the elevated load reading was an echo of
  the build's own near-1000%-CPU utilization decaying through the 1-minute
  load-average window, not third-party contention. Both numbers are
  reported in Results for transparency; the second (quieter) pass is used
  for the delta.
- **Generation-wall measurement still carries the local-replace-from-source
  artifact** 2a/2b documented (`go run entc.go` rebuilds the fork from
  source under a `replace` directive every invocation, instead of pulling a
  cached module) — this depresses the absolute gen-wall number for every
  stage measured this way; the stage-over-stage *direction* (this stage
  improved) should still be trustworthy since the artifact applies equally
  to the before/after numbers.
- Remaining: gemini-side changes this task made (2 entql-ruling edits in
  `models/entc.go`/`models/schema/utils.go`, 1 template deletion, 2
  rewriter-miss hand-fixes on the `ContactPhoneNumber` edge, 3 test-file
  sqlmock-coverage fixes — restored order-independent value assertions via a
  custom `sqlmock.Argument` matcher rather than blanket `AnyArg()`, after a
  review round found the initial loosening lost more real coverage than
  documented — plus the full regen) are intentionally left uncommitted in
  `gemini/.worktrees/codegen-reduction` for user review and commit, per this
  stage's own instructions — 660 files changed (659 modified, 1 deleted:
  `models/templates/delete_modifier_compat.tmpl`), 6,994 insertions(+),
  15,209 deletions(-) per `git diff --stat`. Nothing
  was committed in gemini or contrib by this stage — only this results
  document, in the fork worktree.

## Stage 4: Generic entgql (where/pagination/mutation-input/collection/edges)

Replaced five families of per-entity `entgql`-generated method bodies with
generic runtime packages driven by reflection or a descriptor table, while
keeping the per-entity *types* (`XWhereInput`, `XOrder`, `XPaginateArgs`,
mutation input structs) generated, since gqlgen binds GraphQL input/output
shapes to concrete Go types:

- **Lever A — `gqlwhere`**: per-entity `WhereInput.P()`/`Filter()` bodies
  become a reflection walker (`gqlwhere.Registry[P]`) driven off the
  `F`/`E` handle structs stage 2 introduced, plus a `.Warm(prototype)` call
  in each generated registry initializer that eagerly resolves and caches
  the struct-field/op binding at process start instead of on first request.
- **Lever B — `gqlpage`**: per-entity `Paginate`/`Connection`/`Pager`/
  `ApplyOrder`/`ApplyCursors`/`OrderExpr` bodies become generic
  `gqlpage.Ops[Q,T,ID]`/`Connection[T,ID]`/`Pager[Q,T,ID]`; per-entity
  `XOrder`/`XOrderField` become type aliases to a generic `Order[T,ID]`/
  `OrderField[T,ID]` instantiation, built from a small per-entity
  `OrderField` registration table (`gqlpage.Column`/`Computed`/`Register`).
  `MaxPageSize` (the `_maxPageSize`/clamp pair the old codegen enforced) is
  threaded through explicitly as an `Ops` field and a `PaginateLimit`
  parameter rather than assumed.
- **Lever C — `gqlinput`**: per-entity `Mutate()` bodies become a
  struct-tag-driven walker (`gqlinput.Mutate(input, Mutator)`, tag
  `mutate:"<op>:<name>"`) over `*entbuilder.Mutation[T,I]`. Deliberately
  reproduces an upstream quirk found while migrating: the generated
  `fa` (append) case guards on one struct field but assigns the value of
  the field declared immediately before it, matched by pairing on
  declaration order rather than "fixing" the discrepancy.
- **Lever D — `gqlcollect`**: `collectField` switches become a
  descriptor table (`gqlcollect.Spec`/`Edge`/`Field`) with type-erased
  `Paginate`/`PaginateArgs` functions; the per-entity `xPaginateArgs`
  struct stays generated (it unmarshals into per-entity `Order`/
  `WhereInput` types gqlgen must bind to). The 43 relay-connection arms
  turned out non-uniform across entities (see Deviations) and stay
  generated, reached through a one-line `gqlcollect.Custom` escape hatch
  in the descriptor table rather than being forced into the generic shape.
- **Lever E — `gqledge` + node resolvers**: per-entity edge-resolver
  (`Resolve*`) and Relay node-resolver (`noderOf`/`nodersOf`) bodies become
  thin per-entity closures over `gqledge.One`/`Many`/`Conn` and shared
  `gen/gql_node.go` helpers.

Measured the same way as stages 1-3: against `gemini`, this fork's largest
real-world consumer, in `gemini/.worktrees/codegen-reduction`. Contrib is
where the code landed (branch `feat/stage4-entgql` off `72a92819`, 13
commits `72a92819..6db9937f` plus a 14th post-review fix-wave commit
`56233d55`); nothing is committed in gemini by this stage, same as stages
2-3.

### Benchmark commands

Same three entrypoints as stage 3:

```bash
# LOC
cd models && find gen -name '*.go' -print0 | xargs -0 cat | wc -l

# generation time + peak RSS
MREIS_CODEGEN_ALLOW_WATCHER=1 task generate-go   # from api/

# clean build time + peak RSS
go clean -cache && /usr/bin/time -v go build ./...   # in models/
```

### Per-lever LOC deltas

| Lever | What it replaced | Delta |
|---|---|---|
| A — `gqlwhere` | per-entity `WhereInput.P()` bodies via a reflection walker | -67,826 |
| B — `gqlpage` | pagination: generic `OrderField`/`Connection`/`Pager`/`Paginate`; per-entity types became aliases to generic instantiations | -71,612 |
| C — `gqlinput` | `Mutate()` bodies via a struct-tag-driven walker | -24,048 |
| D — `gqlcollect` | `collectField` switches via a descriptor table | -21,749 |
| E — `gqledge` + node resolvers | per-entity edge and node resolver bodies | -4,201 |
| Final fix wave | deleted `gqledges` `init()` (see Deviations) | -5 |

**Total: 1,049,567 -> 860,126 = -189,441 (-18.05%).** Cumulative across all
four stages: **2,035,883 -> 860,126 = -1,175,757 (-57.75%)**.

### Results

Baseline is stage 3's settled numbers (LOC 1,049,567; gen 106.3s/3.0GB;
build 67.0s/~3.45GB), same gemini worktree lineage. Measured with the
three-pass protocol — median of three passes, box gated below 1-minute
load average 1.0 before each pass — because the first single-pass reading
landed within a few percent of baseline on both timed metrics, which is
exactly the regime where run-to-run noise can hide or manufacture a
result; see Measurement history below for why the single-pass numbers were
not trusted as-is.

| Metric | Before | After (median) | Spread | Verdict |
|---|---|---|---|---|
| Generated LOC | 1,049,567 | 860,126 | deterministic | -18.05% |
| Generation wall | 106.3s | 105.62s | 1.1% | -0.6%, no measurable change |
| Generation peak RSS | 3.0GB | 3.1GB | 3.2% (resolution-limited) | +3.3%, within rounding |
| Clean build wall | 67.0s | 69.35s | 1.3% | **+3.5%, real and reproducible** |
| Clean build peak RSS | ~3.45GB | 3.269GB | 5.7% | -5.2%, **inside noise, not a win** |

**This contradicts the spec's predicted -60-80% build time and memory**,
and it underperforms stage 3 (gen -15.6%, build -8.1%). Stage 4 bought a
large LOC/maintainability win but no compile-time win, and a small, real,
reproducible clean-build slowdown.

**Cause, per the final whole-branch review's analysis**: the import graph
actually got *narrower* this stage, not wider — so the regression is not
import-graph bloat. It's generic-instantiation count. `gqlcollect.Unique`/
`Named` and `gqledge.One`/`Many` are generic per **edge**, not per entity —
roughly 306 entities x ~10 edges produces several thousand distinct
instantiations, and Go elaborates each generic body once per instantiation
per importing package. Trading ~40 lines of straight-line per-entity code
for one generic call site is a net compiler loss even when it's a large
reader win.

**Unmeasured cost, also named by the final review**: incremental builds
are now more coupled. A one-line edit to any of the five runtime packages
(`gqlwhere`, `gqlpage`, `gqlinput`, `gqlcollect`, `gqledge`) invalidates
all ~306 entity subpackages that import it, where a template edit
previously invalidated only whatever it regenerated. Nobody measured this
directly — it wasn't in scope for the benchmark task — but for daily
developer experience it may matter more than the clean-build number above.

Measurement history: a first single-pass reading (gen -0.2%/+3.3% RSS,
build +3.1%/-5.5% RSS) looked like noise given the magnitudes, but a
few-percent delta is exactly the case that most needs repetition, not the
case exempt from it — n=1 cannot distinguish "no change" from noise on a
result this small, and this was the stage's headline performance claim.
The load gate was also tightened from <2.0 to <1.0 for the re-measurement.
The three-pass re-measurement changed two conclusions: the +3.5% clean
build wall-time regression is confirmed real (delta exceeds the pass-to-
pass spread), and the apparent -5.5% clean-build RSS "improvement" from
the first pass is not real (the spread exceeds the delta) — without the
three-pass protocol this document would have reported a memory win that
does not exist. Generation peak RSS is separately capped at 0.1GB rounding
resolution by the profiling script (`profile.sh` deletes its raw
`time -v` tmpfile in a trap), so its 3.2% spread is a lower bound, not the
true spread.

### Test coverage: two measured findings

These surfaced during the stage rather than being anticipated, and are the
most transferable lessons of the four stages so far — worth their own
subsection because they change how much weight the "test suite is green"
claims elsewhere in this document should carry:

- **Only 2 of 286 `ent_resolvers` integration tests reach a relay-connection
  arm** (lever D). Measured, not estimated: made `gqlcollect.Custom`'s arm
  body panic and re-ran the suite — 283 pass, 2 fail, 1 skip.
  `TestNestedOfficeOfficeUserWhereFilterMatchesTopLevel` and
  `TestNestedProposalMarketingCollateralsWhereFilterMatchesTopLevel` are
  the two, and neither exercises an M2M arm (15 of the 43 arms are M2M).
- **Gemini's `Query.node`/`nodes` resolvers never reach `byID`/`byIDs` at
  runtime** (lever E), because no `WithNodeType` is configured anywhere in
  gemini. `api/resolvers/ent_query.resolvers.go` calls `Noder`/`Noders`
  with zero options, so every call falls through to the default
  `"cannot resolve noder (%v) without its type"` error, and
  `node_interface_integration_test.go` asserts on exactly that string. The
  286-test suite therefore validates the stable error path, never the real
  `noderOf`/`nodersOf` execution this lever rewrote.

Consequence: for both levers, mechanical generated-old-vs-generated-new
parity (byte-identical diffs across every relay arm and every node
resolver, captured before/after regen) was the primary evidence that the
rewrite was correct — not the integration test suite. A green suite on
this codebase means considerably less than it appears to; it is a weak
secondary signal that repeatedly overstated its reach this stage.

### Deviations from the spec and plan

- **The spec estimated -130-150k LOC for stage 4.** The measured
  addressable surface, once the five levers were actually scoped and
  built, was larger: the stage delivered -189,441, about 26-46% above the
  spec's estimate.
- **Lever D's own estimate was revised mid-stage.** The plan's ~-30k
  projection for `gqlcollect` was revised down to ~-24.8k after Task 8
  measured the relay-arm non-uniformity (below), and the lever delivered
  -21,749 — a further ~12% short of even the revised estimate. The
  shortfall is per-entity `xPaginateArgs` structs (~8,700 lines across 157
  entities) that unmarshal into per-entity `Order`/`WhereInput` types and
  therefore cannot be type-erased, plus ~3,050 lines of per-entity
  `Spec`/`init()` scaffolding (the `Spec` var had to move into `init()`
  because a cross-entity `Spec -> collectField -> Spec` reference is a
  variable-initialization cycle Go rejects for 40+ entities).
- **Lever E estimated ~-10k, delivered -4,201.** Judged inherent by
  review, not a shortfall in execution: each edge needs a distinct query
  closure (`Only`/`All`/an entity-specific `Query` call) that cannot be
  hoisted into the shared `gqledge` package without that package importing
  generated code — the same constraint that forces `MaskNotFound` to be
  passed as a parameter rather than looked up generically. 3-6 lines per
  edge is a floor in Go, not a shortfall; the ~-10k estimate assumed a
  one-liner the language does not permit here.
- **Relay-connection arms are not uniform across entities and stay
  generated.** `AppendLoadTotal` forks between an M2M-join query shape and
  an FK-group-by shape (15 of the 43 relay arms are M2M), the
  `TotalCount[i]` slot is positional per arm, and `LimitPerRow`'s target
  column flips to an indexed PK constant under the M2M shape. These three
  differences make the arms genuinely non-uniform, not just verbose, so
  they enter the lever D descriptor table through a one-line
  `gqlcollect.Custom` escape hatch rather than being forced into a shared
  shape. The arithmetic backs the call: full erasure would have needed
  roughly 8 typed closures per arm to save ~2,600 lines total — more
  generated complexity introduced than removed.
- **`gqlinput` (lever C) is driven by struct tags, not Go-name inference.**
  The walker reads a `mutate:"<op>:<name>"` tag off each field rather than
  deriving the descriptor's snake_case field/edge name from the Go field
  name. The round trip is lossy on real gemini fields —
  `ZoomInfoCompanyID` -> `zoom_info_company_id`, `SfObject` -> `sf_object`
  — and a wrong guess would write to the wrong column silently, with no
  compile-time or test-time signal unless that exact field happened to be
  covered. Tags cost zero generated lines (they attach to struct fields
  the generator already emits), a deliberate trade of a slightly noisier
  generated struct for eliminating a whole class of silent data
  corruption. Documented at the top of
  `entgql/gqlinput/gqlinput.go`. This is also why `gqlinput` has no
  `gqlwhere`-style eager `Warm` hook (Ruling R15): `gqlwhere` binds a
  WhereInput struct from one template against `F`/`E` handles from a
  *different* template, so the two sides can drift and only a runtime
  check catches it; `gqlinput`'s tag and its field are emitted on the same
  template line and structurally cannot drift, so a `Warm` guard there
  would have cost ~260 generated lines to protect against a bug class that
  cannot occur.
- **`gqlpage`'s `Ops[Q, T, ID]` (lever B) is a struct of closures, not an
  interface constraint on `*XQuery`.** The pager needs
  `query.Ctx.Fields`/`query.Ctx.AppendFieldOnce`, and `Ctx` is a struct
  *field* on the generated query type, not a method — no Go interface can
  reach a field. The alternatives were adding a method to the ent fork
  itself (out of scope: this stage was contrib-only) or threading the
  accessors as closures, hence `Ops.Fields`/`Ops.AppendField`/
  `Ops.ClearFields`. Verified against `Ops` in `entgql/gqlpage/page.go`
  and its emission in `entgql/template/pagination_subpkg.tmpl`. This is
  the reason every entity still carries a ~14-line `Ops` literal — a
  visible, easy-to-mistake-for-avoidable chunk of lever B's residual LOC
  that a plain interface constraint could not have removed.

### Known limitations

- **Mixed-ID and `UnmarshalGQL` template branches are exercised by
  nothing.** No entity in gemini, and no live contrib fixture, has a
  mixed-ID type (the kind `todopulid`-style fixtures exist to exercise
  upstream) or needs the `UnmarshalGQL` branch; both were relocated
  verbatim into the new closures and are logically preserved but
  empirically untested by any build this stage ran. This is not
  hypothetical: the final whole-branch review found a real defect in
  exactly this area — mixed-ID cursors had silently lost `marshalID()` (a
  panic on the first paginated query for any mixed-ID entity) — which was
  fixed in the post-review wave, but the surrounding branches remain
  untested by anything that actually builds today.
- **`whereinputs` eager init costs +20ms startup and +11.9MB heap** (lever
  A's `.Warm()` call, now running for all ~306 entities in one
  `package whereinputs`, so every binary in every module pays the full
  cost to use one input type). Measured via two probe binaries differing
  only in whether they import `gen/whereinputs` — mean of 25 runs, but
  **variance was not recorded**, so treat the timing figure as
  approximate; the magnitude is independently supported by the mechanism
  regardless (`Warm` plans only the prototype's own struct, not its
  nested neighbours, which is why the actual cost came in far under the
  review's 100-300ms/10-25MB estimate).
- **Generation peak RSS is capped at 0.1GB rounding resolution** by the
  profiling script (`profile.sh` parses and deletes the raw `time -v`
  tmpfile), which is why the 3.2% generation-RSS spread reported above is
  a lower bound rather than a true measurement of run-to-run variance.

Note on a figure that appeared mid-stage and was corrected: an early
lever-D estimate cited "3,048" pure-scalar field arms; that number was
found to double-count 179 FK-column blocks. The reconciled figures are
**2,869 pure scalar arms and 3,183 total field-table entries** (2,869
scalar + 157 id + 157 `__typename`).

### Test results

- Contrib: zero test failures across all 10 packages at `6db9937f`
  (`entgo.io/contrib/entgql`'s one pre-existing baseline failure —
  `TestNodeEntityTemplateExecution` asserting stale `todo.ID(id)` text a
  prior stage's own commit had already replaced — was fixed as part of
  lever E). The final-review fix wave's re-review found no new
  Critical/Important breakage at `56233d55`.
- Gemini: models `go test ./...` 27/27 packages pass; `workers` 2/2;
  `task test-integration -- -run OnConflict -count=1` 47/47;
  `ent_resolvers` 285 pass + 1 pre-existing skip (286 total, same count as
  every prior stage); `transaction` 117/117; `contact` 109/109; `chatter`
  66/66; `box` 89/89. No regressions, no bisection needed. What these
  suites do and do not establish is qualified above (Test coverage).

### Final whole-branch review and fix wave

An OPUS review over the full contrib diff (`72a92819..6db9937f`, 13
commits, 7,218 lines, 5 passes) found no Critical findings, 3 Important, 7
Minor, and triaged 18 of the ~20 deferred minors from individual task
reviews as fine to leave; 2 were escalated to Important:

- **I-1**: mixed-ID cursors had lost `marshalID()` (see Known limitations
  above) — fixed, and the fix let the implementer delete `OrderField.Cursor`
  outright; they also found and fixed an additional parity bug it had been
  masking (the default ID order was emitting a non-nil cursor `Value`,
  harmless on gemini but genuinely wrong on a mixed-ID graph).
- **I-2**: `gqledge`'s `MaskNotFound` was a single mutable process global —
  the only one across all five new packages — so a binary linking two
  generated ent schemas would have the second schema's `init()` silently
  overwrite the first's masking configuration. Fixed by passing the mask
  as a parameter instead; the global, its registrar, and the now-moot
  "unregistered" test were all deleted, making the fix smaller than the
  workaround it replaced.
- **I-3**: the `whereinputs` eager-init cost (Known limitations above) was
  unmeasured; measured and closed in the same wave.

The fix wave landed as a single commit (`56233d55`), dropping LOC from
860,131 to 860,126 (the deleted `gqledges` `init()`), with discriminating
tests for every fix (each fails/panics against the pre-fix code). A scoped
re-review — the one re-review this process allows — found all findings
addressed with no new Critical/Important breakage.

### Concerns / follow-ups for the user

- **Clean-build wall-time regression (+3.5%) is real, confirmed over three
  passes, and unresolved.** It is a structural consequence of moving
  per-entity straight-line code into per-edge generic instantiations
  (thousands of them, elaborated once per importing package by the Go
  compiler), not a bug to fix within this stage's scope.
- **Incremental-build coupling was named but not measured.** Whoever picks
  this up next should benchmark a one-line change to one of the five
  runtime packages against a one-line template edit under the old codegen,
  since the final review judged this may matter more day-to-day than the
  clean-build number.
- **Contrib's own `internal/todo*` test fixtures predate stage 1** and
  don't exercise `entfield`/`wherehelpers`, so contrib's own generated-code
  test suite never validated these templates directly — gemini's regen was
  the real gate throughout the stage. A mixed-ID fixture (`todopulid`-style)
  could not have been regenerated against this branch before the I-1 fix
  landed; it can now, but nothing in this stage's evidence chain actually
  did so.
- **The mixed-ID/`UnmarshalGQL` branches remain empirically untested** (see
  Known limitations) — worth a synthetic contrib fixture before the next
  consumer relies on either path.
- Remaining: as with every prior stage, nothing is committed in gemini by
  this task — the regen that produced the 860,126 LOC figure and the test
  results above lives uncommitted in
  `gemini/.worktrees/codegen-reduction`, for user review and commit.
  Nothing was committed in contrib beyond the 14 commits
  (`72a92819..56233d55`) already listed above — only this results
  document, in the fork worktree.
