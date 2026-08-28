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
