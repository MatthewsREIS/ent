# Ent codegen isolation via `entcodegen` build tag

**Status:** Design — awaiting implementation plan.
**Owner:** Matthews REIS ent fork.
**Date:** 2026-05-22.

## 1. Problem

`entc.Generate` requires the schema package and its full transitive import graph to typecheck. Two failure modes happen routinely:

1. **Hook code that references generated code.** A schema's `Hooks()` method (or a file the schema package imports) references `ent/gen/...` symbols that don't exist yet (renamed edge, new field, deleted accessor). The schema package fails to typecheck, codegen fails to load.
2. **Snapshot conflicts and stale restores.** When load fails, `entc/entc.go:443` (`mayRecover`) falls back to restoring `gen.Config.Schema` from a JSON blob in `internal/schema.go`. This is silent — a stale snapshot regenerates stale code without any indication. Merge conflicts in the snapshot file have to be resolved manually.

Documented incident from prior epic memory: schema imported `river`, which referenced a not-yet-generated accessor, codegen silently restored a stale snapshot, generated output dropped the new edge. The bug took hours to diagnose because the apparent codegen "success" was actually a stale-restore.

## 2. Goal

Make `entc.Generate` succeed whenever the *schema definitions* are valid Go, even if hook bodies, interceptors, or other runtime-helper code in or adjacent to the schema package references symbols that don't currently exist. Eliminate the silent stale-snapshot fallback.

Out of scope: any tooling change that requires schema authors to learn a new DSL or move away from `ent.Schema`. The schema authoring surface stays the same.

## 3. Non-goals

- AST-only schema loading (considered and rejected; see §10).
- Backward compatibility with upstream ent. This fork already diverges; the change is breaking.
- A new feature flag for the build-tag mechanism. The tag is always on.

## 4. Approach

Add a Go build tag `entcodegen` that entc unconditionally sets when loading the schema package. Authors gate any file that depends on generated code (hook bodies, runtime helpers, transitive packages) with `//go:build !entcodegen`. Those files are excluded by the Go toolchain during codegen and included in normal builds.

Coupled with the build-tag mechanism: shrink `ent.Interface` to remove `Hooks() / Interceptors() / Policy()`. These methods are not part of the static schema definition; they belong in client-side registration code (`client.Use(...)`, `client.Intercept(...)`). With the methods gone from the interface, the load model cannot accidentally re-introduce bootstrap problems via these surfaces.

Delete the snapshot mechanism entirely. The build-tag escape hatch obviates the need for a fallback, and the silent-stale failure mode is structurally eliminated.

The combined change is the smallest set that achieves the goal coherently.

## 5. Architecture

### 5.1 Load flow (before)

```
entc.Generate → LoadGraph → packages.Load(schema pkg, full typecheck)
                                        ↓ fails?
                                  mayRecover → restore from internal/schema.go JSON → graph.Gen()
```

### 5.2 Load flow (after)

```
entc.Generate → LoadGraph → packages.Load(schema pkg, BuildFlags += "-tags=entcodegen")
                                        ↓ fails?
                                  wrapLoadError → return error naming failing import + build-tag hint
```

No fallback. No silent recovery.

### 5.3 Schema-package file classes

After the change, files in or adjacent to the schema package fall into three classes:

- **Class A — pure schema (no build tag).** Fields/Edges/Indexes/Mixin/Annotations definitions. Imports limited to stdlib, `entgo.io/ent/...`, third-party libs, and other schema-package mixins. Must not import `ent/gen/...`, `ent/hook`, `ent/intercept`, or any package that does.
- **Class B — runtime helpers (`//go:build !entcodegen`).** Hook bodies, interceptor bodies, predicate helpers, runtime context lookups. Compiled in normal builds; excluded during codegen.
- **Class C — codegen-mode stubs (`//go:build entcodegen`).** Stub implementations of identifiers referenced from Class A annotation closures. Provides a no-op definition during codegen so the schema package typechecks.

### 5.4 `ent.Interface` after the cut

```go
type Interface interface {
    Type()
    Fields()      []Field
    Edges()       []Edge
    Indexes()     []Index
    Config()      Config       // deprecated upstream, still legal
    Mixin()       []Mixin
    Annotations() []schema.Annotation
}
```

`Hooks/Interceptors/Policy` are removed. Their corresponding default impls on `ent.Schema` are removed. The runtime `ent.Hook` / `ent.Interceptor` types remain (used by `client.Use(...)` / `client.Intercept(...)`). The `ent.Policy` type is deleted along with the `privacy/` runtime package and `FeaturePrivacy`.

## 6. Concrete change surface

### 6.1 ent fork (`matthewsreis/ent`)

**`entc/load/load.go`** — inject `-tags=entcodegen` into `BuildFlags` at lines 96 (`gorun` for `.entc/main.go`) and 118 (`packages.Load` config). User-supplied `BuildFlags` are preserved alongside.

**`entc/load/schema.go`** — remove `Hooks/Interceptors/Policy` fields from `Schema` struct (lines 28-31); remove `loadHooks/loadInterceptors/loadPolicy` calls in `Load` (lines 219-225); delete those methods (lines 339-376); delete `safeHooks/safeInterceptors/safePolicy` (lines 473-505); remove corresponding blocks in `loadMixin` (lines 283-313).

**`ent.go`** — remove `Hooks() []Hook`, `Interceptors() []Interceptor`, `Policy() Policy` from `Interface` (lines 60-74); remove default impls `func (Schema) Hooks/Interceptors/Policy` (lines 245-251); delete the `Policy` type.

**`entc/gen/type.go`** — delete `NumHooks/HookPositions/NumInterceptors/InterceptorPositions/NumPolicy/PolicyPositions` (lines 979-1011).

**`entc/gen/feature.go`** — delete `FeatureSnapshot` (lines 64-79) and `FeaturePrivacy`; remove both from `AllFeatures` (line 169 and equivalent). Keep `FeatureIntercept` — it gates runtime helpers consumers still use via `client.Intercept`.

**`entc/gen/template/internal.tmpl`** — delete `internal/schema` block (lines 9-19).

**`entc/gen/template/meta.tmpl`** — remove `$.NumHooks`, `$.NumPolicy`, `$.NumInterceptors` from the gating expression at lines 53-70. Re-form around remaining triggers (defaults, validators, value scanners).

**`entc/gen/template/runtime.tmpl`** — delete hook/interceptor/policy init scaffolding at lines 47, 112-145. The `$numHooks := add ...` expression is simplified or removed.

**`entc/gen/template/builder/create.tmpl`**, **`builder/update.tmpl`**, **`dialect/sql/update.tmpl`** — remove `$runtimeRequired := or $.NumHooks $.NumPolicy` halves. The `_ = createE.defaults()` etc. become unconditional or gated on remaining flags.

**`entc/gen/template/builder/client_type.tmpl`** — remove `Hooks/Interceptors` accessor methods at lines 193-204.

**`entc/gen/template/privacy/`** — delete the entire subdirectory.

**`entc/gen/template/hook.tmpl`**, **`intercept.tmpl`** — keep. Generate per-type wrapper types consumers use inside `client.Use/Intercept`.

**`entc/entc.go`** — delete `SnapshotDir` option (lines 168-178); drop snapshot-copy block (lines 423-438); replace `mayRecover` call in `generate` (lines 408-413) with a new `wrapLoadError` helper that returns informative errors; delete `mayRecover` function (lines 443-469).

**`entc/internal/snapshot.go`** — delete.
**`entc/internal/snapshot_test.go`** — delete.

**`privacy/`** — delete the directory (root-level runtime privacy package).

**`entc/gen/globalid.go:36`** — delete the `if FeatureSnapshot enabled` branch for `IncrementStartsFilePath` merge-conflict resolution.

**Integration directories that have generated snapshot files:**
- `entc/integration/edgeschema/ent/internal/schema.go` — delete.
- `entc/integration/edgeschema/ent/entc.go:30` — drop `gen.FeatureSnapshot`.
- `entc/integration/hooks/ent/internal/schema.go` — delete.
- `entc/integration/privacy/` — delete entire directory.
- Any other `entc/integration/*/ent/internal/schema.go` produced by the snapshot template — delete.
- Any `entc/integration/*/ent/entc.go` that names `"schema/snapshot"` or `"privacy"` — remove those feature names.

**`entc/gen/graph_test.go:595`** — delete `graph.Features = []Feature{FeatureSnapshot}` test case.

### 6.2 Consumer (`matthewsreis/gemini` — `service-api-go/api-graphql`)

**Schema files (`src/ent/schema/`):**
- 48 files defining `Hooks()` — bodies moved to `src/ent/hookreg/<type>.go`; method removed from schema; sibling `*_hooks.go` files (where present as dedicated hook-only files) deleted after content moved; otherwise the offending file gets `//go:build !entcodegen` and is reduced to runtime-only bodies.
- 7 files defining `Interceptors()` — same pattern.
- 4 files with `entsearch.FilterFunc(closure)` in `Annotations()` (`listing.go`, `escrow_edges.go`, `contact_list.go`, `proposal.go`) — closure extracted to named function in `*_filter.go` (Class B); stub added in `*_filter_stub.go` (Class C); annotation references the named function.
- `src/ent/schema/utils.go` — gets `//go:build !entcodegen` (it imports `ent/gen` and `entgo.io/ent/entql` for predicate helpers used by annotation closures).

**New package `src/ent/hookreg/`:**
- `register.go` defines `Install(client *gen.Client)` calling per-type registration functions.
- One file per migrated schema (`hookreg/attachment.go`, `hookreg/contract.go`, …) containing the moved hook/interceptor bodies and `installX(c *gen.Client)` registration.
- Absorbs existing `src/ent/dealhooks/listing_agreement.go` contents.

**Codegen config (`service-api-go/api-graphql/entc.go`):**
- Drop `"privacy"` and `"schema/snapshot"` from `entc.FeatureNames(...)` (lines 126, 128).
- Delete `entc.SnapshotDir("src/ent/snapshot")` (line 135).

**Snapshot artifact:**
- `src/ent/snapshot/` directory — delete after the cutover.

**New tooling:**
- `cmd/entcodegen-migrate/` — one-shot AST-driven migration tool that performs schema → hookreg refactoring. Deletable after the cutover.
- `cmd/entcodegen-lint/` — long-lived lint that fails if schema files violate the Class A/B/C contract (specifically: any `func (T) Hooks()/Interceptors()/Policy()` method definitions, or untagged files that fail to parse under `-tags=entcodegen`).

**Documentation:**
- `docs/codegen-isolation.md` — convention guide, file-class explanation, migration history, troubleshooting.

### 6.3 Extensions audit

None of the consumer's currently-registered extensions emit code into the schema package that imports generated code. Specifically:

- `entgql`, `entsoftdelete`, `entfieldsecurity`, `entscopes`, `entorderindex`, `entmultipicklist`, `entsearch`, `entsf`, `entfake`, `enthubspot`: emit into `ent/gen/...` or external paths, not into the schema package.
- `enthistory`: emits `*_history.gen.go` into the schema dir, but those files import only `entscopes` annotations (Class A safe).

No extension-side build-tag injection is required. Future in-house extensions that emit code into the schema dir referencing generated code must prepend `//go:build !entcodegen` in their templates (documented as a contributor expectation).

### 6.4 Resolvers audit

`src/resolvers/` contains 258 files, 247 of which import `ent/gen`. None are imported by the schema package. Resolvers are downstream of codegen, not in the load-time import graph. No tagging required.

## 7. Error UX

After snapshot removal, load failures must surface clearly. The new `wrapLoadError` in `entc/entc.go`:

1. Extracts the failing package path and unresolved symbol(s) from `packages.Error`.
2. If the unresolved symbol's import path matches a generated-code heuristic (path contains `/ent/gen` or symbol name matches `hook|intercept|<lowercase-type>` patterns), appends a hint suggesting `//go:build !entcodegen` on the offending file.
3. Wraps with a URL to `docs/codegen-isolation.md`.

Sample post-cut error:

```
entc/load: schema package failed to typecheck under -tags=entcodegen:
  service-api-go/api-graphql/src/ent/schema/attachment_hooks.go:14:2:
    undefined: hook.AttachmentFunc

  This file appears to reference generated code. If it contains hook,
  interceptor, or runtime helpers, add this on the first line:

      //go:build !entcodegen

  See docs/codegen-isolation.md for details.
```

The heuristic is best-effort. False positives are benign: adding the tag when the real problem is a typo means `go build ./...` (which doesn't set the tag) will surface the typo on the next normal build.

## 8. Phased delivery

### Phase 0 — PoC validation (no commits to long-lived branches)

Throwaway branch in consumer repo, no ent fork changes. Validates the technical foundation before investing in the full refactor.

**Stage 1 — Validate tag injection (~30 min).** Pick `attachment_hooks.go`; add `//go:build !entcodegen`; extend `entc.BuildFlags("-tags=entcodegen", ...)`; run codegen; verify success and that only the Attachment per-type hook scaffolding is missing from generated output.

**Stage 2 — Validate isolation (~30 min).** Add a deliberately-broken Class B file in the schema dir. Confirm codegen succeeds and normal `go build ./...` fails.

**Stage 3 — Validate annotation-closure pattern (~1-2h).** Refactor one of the 4 closure cases into Class A + B + C trio. Confirm both modes work.

**Gate:** review PoC results. If Stage 1 fails, redesign before proceeding.

### Phase 1 — ent fork PRs

1. **ent PR #1 — Tag injection only.** Additive, no breaking changes.
2. **ent PR #2 — Drop snapshot feature.** Removes the fallback path; adds `wrapLoadError`.
3. **ent PR #3 — Drop `Hooks()/Interceptors()/Policy()` from `ent.Interface`.** Breaking change.
4. **ent PR #4 — Drop `FeaturePrivacy` and `privacy/` runtime package.** Breaking change.

### Phase 2 — Consumer migration PRs

Land against pre-PR-3 ent (schemas may still have `Hooks()`; ent ignores extra methods).

1. **Consumer PR #1 — `hookreg/` scaffolding.** Empty `Install(client)` called from app entrypoint.
2. **Consumer PR #2 — Migrate 48 hook files.** Migration script execution.
3. **Consumer PR #3 — Migrate 7 interceptor files.** Same pattern.
4. **Consumer PR #4 — Refactor 4 annotation closures.**
5. **Consumer PR #5 — Cutover.** Bump ent dep, drop `"privacy"`/`"schema/snapshot"` feature names, delete `SnapshotDir`, regenerate, delete `src/ent/snapshot/`.
6. **Consumer PR #6 — Lint, docs, river-worker carve-out.**

Order of ent PR #3 vs consumer PRs #2-#4 is flexible: schemas without `Hooks()` methods are silently legal against pre-PR-3 ent (`ent.Schema` provides default); schemas with `Hooks()` methods are silently legal against post-PR-3 ent (the extra method is ignored). The danger zone is hypothetical; no synchronization required.

## 9. Testing strategy

Behavior changes must be covered by integration tests — existing where possible, new where there are gaps.

### 9.1 Existing tests to update

- `entc/integration/hooks/hooks_test.go` — `TestSchemaHooks` rewritten as `TestRegisteredHooks` using `client.Use`. Assertions preserved (number-too-short, name-set-by-hook, password-edit-rejection). Companion: `TestInterceptor_Sanity` and friends audited; schema-attached interceptor patterns migrated.
- `entc/integration/hooks/ent/schema/*.go` — hook bodies move to a `hookreg/` subpackage in the integration dir. The integration becomes the canonical example for migration docs.

### 9.2 Existing tests to delete

- `entc/internal/snapshot_test.go` — entire file (snapshot path removed).
- `entc/integration/privacy/` — entire directory (privacy feature removed).
- `entc/gen/graph_test.go:595` `FeatureSnapshot` test case.

### 9.3 New tests

1. **`entc/integration/codegen_isolation/isolation_test.go`** — Class B file with deliberately-broken symbol gated by `!entcodegen`; codegen succeeds; normal build fails.
2. **`entc/integration/codegen_isolation/error_path_test.go`** — Class B file without the tag; codegen fails; asserted error message contains the failing import path and the build-tag hint.
3. **`entc/load/load_test.go` (extension)** — `-tags=entcodegen` ends up in `BuildFlags` whether or not user supplies own; user-supplied flags preserved.
4. **`entc/load/schema_test.go` (extension)** — shrunk `ent.Interface` accepts a schema with only `Type/Fields/Edges/Indexes/Config/Mixin/Annotations`.
5. **`entc/gen/graph_test.go` (additions)** — for any graph, templates do not emit references to `NumHooks/NumPolicy/NumInterceptors` identifiers. Regression guard.

### 9.4 Consumer test obligations

- App test suite passes at every Consumer PR boundary.
- New `hookreg/` test confirms `Install(client)` wires the expected hook count (sentinel against silent registration loss).
- New river-worker test confirms a worker-constructed client has zero hooks registered.

## 10. Alternatives considered

### 10.1 AST-only schema loader

Replace runtime-reflection load with static AST parsing of schema files. Rejected because:

- The schema files use dynamic patterns (helper functions, loops, computed values for annotations and defaults) that aren't statically resolvable without partial Go interpretation.
- Significant fork divergence from upstream ent (~thousand-LOC new component).
- Doesn't compose with extension-provided annotation types whose values are runtime objects.

### 10.2 Custom comment marker (e.g., `//ent:ignore`)

Custom marker preprocessed by entc. Rejected because:

- Only fixes the schema-package-internal case (Class A). Cannot propagate through the Go toolchain's import graph, so it can't help when the schema imports a broken package transitively.
- `//go:build !entcodegen` is a standard Go primitive that already propagates correctly.

### 10.3 Keep snapshot as fallback

Retain `internal/schema.go` snapshot path for users who don't adopt the build-tag convention. Rejected because:

- The silent-stale-restore failure mode persists exactly as before for anyone not on the new convention. The convention is on by default, so the fallback path runs whenever the convention is broken — i.e., precisely the situation where a stale restore is dangerous.
- Maintaining two paths complicates ent without proportional benefit. The fork already controls both ent and consumer.

### 10.4 Drop only `Policy()`, keep `Hooks()/Interceptors()`

Conservative scoping. Rejected because:

- Leaves two ways to register hooks (schema-attached vs `client.Use`); the architectural irregularity that motivated the discussion remains.
- The 48 hook files in the consumer still depend on generated code; the bootstrap fragility for hooks persists; the build-tag escape hatch becomes the de-facto convention for those files anyway, so we pay the migration cost without the simplification benefit.

## 11. Risks

| Risk | Mitigation |
|---|---|
| Tag injection doesn't propagate to `.entc/main.go` runtime | PoC Stage 1 validates this before any commit. |
| Migration script misses a corner-case schema file | Lint guard (Consumer PR #6) catches residual `Hooks()/Interceptors()/Policy()` methods at PR time. |
| Removing `NumHooks` template gating breaks downstream codegen in unforeseen ways | Staged after PR #1/#2 to isolate regression sources. Full integration suite runs in CI. |
| Annotation-closure stub returns wrong shape and breaks annotation reflection | PoC Stage 3 validates the pattern on one file before scaling. |
| River-worker client without hooks misses a hook needed for correctness | Audit hooks for "data integrity" vs "side-effect" before Consumer PR #6; data-integrity hooks installed in worker client explicitly. |
| Cutover PR lands while another developer has in-flight branch with new `Hooks()` method | Lint guard catches at PR time; freeze window communicated for the cutover. |
| ent fork drifts from upstream further | Acknowledged tradeoff per existing fork strategy. |

## 12. Success criteria

- All ent integration tests pass on the post-PR-4 fork.
- All consumer app tests pass after the cutover PR.
- `cmd/entcodegen-lint` passes in CI.
- `docs/codegen-isolation.md` exists and is linked from the load-error message.
- Build-time measurement on bench host: `go build` wall on `ent/gen` post-cutover compared against pre-cutover baseline; result documented in epic memory regardless of sign.
- Verification scenario: a deliberately-broken hook file with `//go:build !entcodegen` allows codegen to succeed while normal `go build ./...` fails on the broken symbol. The "make generate work when the rest of the world doesn't compile" promise verified by example.

## 13. Open items

None. All design questions answered during brainstorming session 2026-05-22.
