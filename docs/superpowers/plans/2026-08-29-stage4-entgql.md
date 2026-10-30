# Stage 4: entgql Reflection + Generics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the per-entity entgql bodies — `WhereInput.P()`, the
`OrderField` var blocks and pager machinery, `MutationInput.Mutate()`, the
`collectField` switches, the edge resolvers and the node `Noder`/`Noders`
funcs — with reflection walkers and generics living in the contrib fork,
leaving only structs, one-line handles and small config literals. Expected
−200-215k generated LOC in gemini (1,049,567 → ~840-850k).

**Architecture:** Four new packages under `entgql/` in the contrib fork:
`gqlwhere` (reflection walker mapping non-nil WhereInput op-fields to
`F`/`E` handle predicates), `gqlpage` (generic `Edge`/`Connection`/`Order`/
`OrderField`/`Pager` + `Paginate`, driven by a small per-entity `Ops`
literal), `gqlinput` (struct-tag-driven `Mutate` walker over the generic
`entbuilder.Mutation` method set), and `gqlcollect` (descriptor-driven
GraphQL field collection). Generated per-entity pagination types become
**aliases to generic instantiations**, so their method sets come from
contrib and no per-entity method bodies survive. None of this needs ent
internals: every ent API it touches (`F.X.EQ`, `E.X.HasWith`,
`m.SetField`, `q.Where`) is reached through reflection, a locally-declared
structural interface, or a generated closure — so **contrib's `entgo.io/ent`
pin is not bumped in this stage**.

**Tech Stack:** Go generics + `reflect`; `entgo.io/contrib/entgql`
(`Cursor`, `PageInfo`, `CursorsPredicate`, `MultiCursorsPredicate`,
`OrderDirection`); `entgo.io/ent/dialect/sql`; `github.com/99designs/gqlgen/graphql`;
text/template codegen in `entgql/template/*.tmpl`.

**Spec:** `docs/superpowers/specs/2026-08-27-codegen-reduction-design.md`
(Stage 4 section, lines 131-141 — binding authority). The spec estimates
−130-150k; the survey below measures the real ceiling at −200-215k because
it also counts `gqlcollections/`, `gqledges/` and the root `gql_node_*`
files, which the spec folded into "node_implementors".

Spec item already done before this plan (do not redo): gemini's
`where_input_entity.tmpl` override has already been removed (lever B-3a);
the `HasXWith` empty-predicate fix now lives upstream in
`where_input_subpkg.tmpl`. Preserve that behavior when rewriting it.

## Survey (measured 2026-08-29, gemini `models/gen`, total 1,049,567 LOC)

| Target | LOC | Files | Removable |
|---|---|---|---|
| `<entity>/gql_pagination.go` | 93,340 | 157 | ~85k (lever B) |
| `whereinputs/*.go` | 114,965 | 158 | ~63k (lever A) |
| `gqlcollections/*.go` | 49,755 | 158 | ~30k (lever D) |
| `mutationinputs/*.go` | 38,160 | 130 | ~21k (lever C) |
| root `gql_*.go` | 25,325 | 911 | ~5k (lever E) |
| `gqledges/*.go` | 13,581 | 150 | ~6k (lever E) |
| `<entity>/gql_node_implementors.go` | 1,256 | 157 | 0 (leave) |

## Global Constraints

- Fork branch: `feat/stage4-entgql`, stacked on `feat/stage3-reflection-dedupe`
  (base `ac7066c6a`). PR base = that branch. **The fork gets only this plan
  and the results-doc commit** — no Go changes are expected in the ent fork.
  If a task discovers one is genuinely required, stop and report it rather
  than silently expanding the fork diff.
- Contrib branch: `feat/stage4-entgql`, stacked on `feat/stage2a-field-handles`
  (base `72a92819`), in `/home/smoothbrain/dev/matthews/contrib`. All Go and
  template changes land there. Contrib is normally read-only for this
  project; this stage is the exception.
- **Do not bump contrib's `entgo.io/ent` replace and do not regenerate
  contrib's `entgql/internal/todo*` fixtures.** Those fixtures were generated
  before stage 1 and no longer match the templates; regenerating them is a
  separate, user-owned follow-up. Their staleness is why contrib's generated
  tests cannot gate this stage.
- Gate per contrib task, in order:
  1. `go build ./entgql/...` and `go test ./entgql/...` in contrib. The
     `Test*TemplateContent` / `Test*TemplateExecution` tests in
     `entgql/template_test.go` assert on rendered template text; when a
     template change makes one fail, update the assertion to the new
     expected text — never delete the test.
  2. Regenerate gemini and build it (see below). This is the real proof.
- Gemini worktree: `/home/smoothbrain/dev/matthews/gemini/.worktrees/codegen-reduction`.
  **Never commit there.** Its `go.mod` already replaces `entgo.io/ent` with the
  fork worktree and `entgo.io/contrib` with `/home/smoothbrain/dev/matthews/contrib`,
  so contrib edits are picked up with no version dance.
  - Regen: `MREIS_CODEGEN_ALLOW_WATCHER=1 task generate-go` from `api/`.
  - Build: `go build ./...` from `models/`.
  - LOC: `find gen -name '*.go' -print0 | xargs -0 cat | wc -l` from `models/`.
- NEVER use bare `git stash` — the stash stack is shared across worktrees. Use
  a throwaway `git worktree add` for any before/after comparison.
- Gemini test suites (Task 14): workers need `-p 2 -parallel 4`
  (`pq: could not write block` is IntegreSQL pool contention, infra not code);
  api integration via `task test-integration -- ...` from `api/`. Expected
  counts: OnConflict 47, ent_resolvers 285+1skip, transaction 117,
  contact 109, chatter 66, box 89.
- Long commands run in the FOREGROUND with a large timeout. If something must
  be backgrounded, poll its output file with Read. Never end a turn waiting
  for a background notification.
- gopls diagnostics are chronically stale mid-edit — trust `go build` / `go test` only.
- Benchmarks LAST (Task 15), on a settled box (load avg < 2). Baseline:
  LOC 1,049,567; gen 106.3s / 3.0GB RSS; clean build 67.0s / ~3.45GB.
- Commit trailers on every commit:
  `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` plus the
  `Claude-Session:` link for the current session.
- Byte parity of generated output is NOT expected. Parity is semantic:
  identical GraphQL behavior and identical emitted SQL. Predicate and ORDER BY
  ordering must be preserved exactly — see the ordering note in Task 1.

---

### Task 1: `gqlwhere` reflection walker

**Files:**
- Create: `entgql/gqlwhere/gqlwhere.go` (contrib)
- Create: `entgql/gqlwhere/gqlwhere_test.go` (contrib)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces (Task 2 generates calls to these):

```go
package gqlwhere

// ErrEmpty is the shared parent of every generated ErrEmpty<X>WhereInput.
// The walker tests nested inputs with errors.Is(err, ErrEmpty); a caller's
// Filter still tests its own leaf sentinel, so a nested entity's empty error
// does not satisfy the parent's leaf — matching today's behavior.
var ErrEmpty = errors.New("gqlwhere: empty predicate")

// NewEmptyError returns a leaf sentinel wrapping ErrEmpty.
func NewEmptyError(msg string) error

// Registry holds the reflection tables for one WhereInput type.
type Registry[P ~func(*sql.Selector)] struct{ /* unexported */ }

// NewRegistry builds the tables once, at package-init time.
//   fields, edges: the entity's F and E handle structs (passed by value)
//   not, and, or:  the entity package's Not/And/Or combinators
//   empty:         this entity's leaf sentinel
func NewRegistry[P ~func(*sql.Selector)](
    fields, edges any,
    not func(P) P, and func(...P) P, or func(...P) P,
    empty error,
) *Registry[P]

// P walks input (a *XWhereInput) and returns the combined predicate.
// Returns (nil, r.empty) when nothing was set.
func (r *Registry[P]) P(input any) (P, error)

// FilterP is P with the empty case flattened: returns (nil, nil) when empty.
func (r *Registry[P]) FilterP(input any) (P, error)
```

- [ ] **Step 1: Read the body being replaced.** Read
  `entgql/template/where_input_subpkg.tmpl` end to end and
  `/home/smoothbrain/dev/matthews/gemini/.worktrees/codegen-reduction/models/gen/whereinputs/company.go`
  lines 761-1960. Write down, in order, every statement shape the walker must
  reproduce: the Not / Or (n==1 vs n>1) / And (n==1 vs n>1) prologue with its
  `fmt.Errorf("%w: field 'or'", err)` wrapping and its
  `continue`-on-empty behavior; `predicates = append(predicates, i.Predicates...)`;
  then per field `AppendPtr` / `AppendSlice` / `AppendBool`; then per edge the
  `Has<E>` bool-inverts-with-Not block and the `Has<E>With` block including
  the `hasEmptyPredicate` → `sql.False()` short-circuit; then the
  `len(predicates)` 0/1/default switch.

- [ ] **Step 2: Fix the ordering contract.** The generated struct declares
  fields in exactly the order the old `P()` consumed them (`range
  $comparableFields` × `range safeOps`, then `range $filteredEdges`), so
  walking `reflect.VisibleFields` in declaration order reproduces the
  predicate sequence — and therefore the emitted SQL text — byte for byte.
  The four names `Predicates`, `Not`, `Or`, `And` are handled by the fixed
  prologue and must be skipped during the field walk. Unexported fields
  (gemini's entsearch injects `searchQuery` and `meilisearchFilters`) must
  also be skipped. Record this contract as a comment at the top of
  `gqlwhere.go`.

- [ ] **Step 3: Write the failing tests.** Create `gqlwhere_test.go` with a
  local fake entity — a `P` type `func(*sql.Selector)`, an `F` struct of two
  `entfield`-shaped handles built by hand (a struct with `EQ(string) P`,
  `In(...string) P`, `IsNil() P` methods and a variadic/niladic mix), an `E`
  struct with `Has() P` / `HasWith(...P) P`, and a WhereInput struct mirroring
  the generated shape. Cover: single EQ; variadic In with len 0 (skipped) and
  len 2; niladic bool false (skipped) and true; a nil-able field whose own
  type is a slice (the `RType.IsPtr` case — struct field type equals the arg
  type, no deref); Not/Or/And nesting including the n==1 shortcut; nested
  empty producing `ErrEmpty`; `HasXWith` with an empty nested input producing
  a never-match predicate; empty input returning the leaf sentinel; and an
  order test asserting the predicates arrive in struct-declaration order.

- [ ] **Step 4: Run the tests and confirm they fail.**
  Run: `go test ./entgql/gqlwhere/... -v` in contrib. Expected: build failure
  (package does not exist yet).

- [ ] **Step 5: Implement `gqlwhere.go`.** Key mechanics:
  - `NewRegistry` reflects over `fields` and `edges`. For each handle field
    (e.g. `F.SalesforceID`), enumerate the handle's method set and keep every
    method whose single result type is assignable to `P`. Register the key
    `<HandleFieldName><MethodName>`, plus the bare `<HandleFieldName>` aliased
    to the `EQ` method (the template names the EQ filter without a suffix).
    For each edge field, register `Has<EdgeFieldName>` and
    `Has<EdgeFieldName>With`.
  - Dispatch is driven by the *method* signature, never by guessing from the
    struct field's type: `t.IsVariadic()` → treat the struct value as the
    slice of arguments, skip when `Len() == 0`; `NumIn() == 0` → niladic,
    call when the bool is true; otherwise single-arg — pass the struct value
    directly when its type equals the parameter type, dereference when it is
    a pointer to the parameter type, and skip when nil. Any other shape is a
    codegen/registry mismatch: panic at `NewRegistry` time (init), never at
    request time.
  - Cache `reflect.Type` → resolved plan (ordered slice of
    `{structFieldIndex, methodValue, kind}`) on first `P()` call for that
    input type, guarded by a `sync.Map`, so per-request work is an indexed
    walk plus `reflect.Value.Call`.
  - `HasXWith`: for each element call its `P()` method reflectively; on an
    error satisfying `errors.Is(err, ErrEmpty)` set `hasEmptyPredicate` and
    break; on any other error wrap as
    `fmt.Errorf("%w: field '%s'", err, structFieldName)`. When
    `hasEmptyPredicate`, append `P(func(s *sql.Selector) { s.Where(sql.False()) })`;
    otherwise call `HasWith` variadically with the collected predicates.

- [ ] **Step 6: Run the tests and confirm they pass.**
  Run: `go test ./entgql/gqlwhere/... -v` in contrib. Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/gqlwhere
git commit -m "feat(entgql): reflection walker for WhereInput predicates"
```

---

### Task 2: Rewrite `where_input_subpkg.tmpl` onto `gqlwhere`

**Files:**
- Modify: `entgql/template/where_input_subpkg.tmpl` (contrib)
- Modify: `entgql/template_test.go` (contrib) — only the assertions this changes

**Interfaces:**
- Consumes: `gqlwhere.NewRegistry`, `gqlwhere.NewEmptyError`, `Registry.P`,
  `Registry.FilterP` from Task 1.
- Produces: unchanged public surface —
  `type XWhereInput struct{...}`, `(*XWhereInput) AddPredicates`,
  `(*XWhereInput) Filter(*XQuery) (*XQuery, error)`, `(*XWhereInput) P()`,
  `ErrEmptyXWhereInput`.

- [ ] **Step 1: Keep the struct block exactly as it is.** Only the three
  method bodies and the sentinel change. The struct's field order is the
  ordering contract from Task 1 Step 2 — do not reorder, do not merge the EQ
  special-case, do not drop the `json` tags.

- [ ] **Step 2: Replace the sentinel and the bodies** with:

```go
// ErrEmpty{{ $input }} is returned in case the {{ $input }} is empty.
var {{ $err }} = gqlwhere.NewEmptyError("{{ base $.Config.Package }}: empty predicate {{ $input }}")

var registry{{ $input }} = gqlwhere.NewRegistry[predicate.{{ $n.Name }}](
	{{ $n.Package }}.F, {{ $n.Package }}.E,
	{{ $n.Package }}.Not, {{ $n.Package }}.And, {{ $n.Package }}.Or,
	{{ $err }},
)

// AddPredicates adds custom predicates to the where input to be used during the filtering phase.
func (i *{{ $input }}) AddPredicates(predicates ...predicate.{{ $n.Name }}) {
	i.Predicates = append(i.Predicates, predicates...)
}

// Filter applies the {{ $input }} filter on the {{ $n.QueryName }} builder.
func (i *{{ $input }}) Filter(q *{{ $n.Package }}.{{ $n.QueryName }}) (*{{ $n.Package }}.{{ $n.QueryName }}, error) {
	if i == nil {
		return q, nil
	}
	p, err := registry{{ $input }}.FilterP(i)
	if err != nil || p == nil {
		return q, err
	}
	return q.Where(p), nil
}

// P returns a predicate for filtering {{ plural $n.Name | lower }}.
// An error is returned if the input is empty or invalid.
func (i *{{ $input }}) P() (predicate.{{ $n.Name }}, error) {
	return registry{{ $input }}.P(i)
}
```

  Drop the now-unused imports (`errors`, `fmt`, `entsql`, `wherehelpers`, and
  the per-edge target-package imports that only existed for the `HasWith`
  calls); add `entgo.io/contrib/entgql/gqlwhere`. The `predicate` and
  `{{ $n.Package }}` imports stay.

- [ ] **Step 3: Run contrib's tests.**
  Run: `go build ./entgql/... && go test ./entgql/...` in contrib.
  Update any `Test*TemplateContent` assertion that names the removed
  `wherehelpers` lines to expect the new body. Expected: PASS.

- [ ] **Step 4: Regenerate gemini and build.**
  Run, from `/home/smoothbrain/dev/matthews/gemini/.worktrees/codegen-reduction/api`:
  `MREIS_CODEGEN_ALLOW_WATCHER=1 task generate-go`; then from `models/`:
  `go build ./...`. Expected: clean build.

- [ ] **Step 5: Prove SQL parity on one entity.** Before/after diff of a
  rendered predicate: in `models/`, write a throwaway `go test` (in the
  scratchpad, not committed) that builds a `CompanyWhereInput` with an EQ, an
  In, an IsNil, a nested Or, and a `HasXWith`, calls `.P()`, applies it to a
  `sql.Selector` and prints the resulting query + args. Compare against the
  same output captured from a throwaway `git worktree add` of the
  pre-Task-2 gemini gen tree. The strings must match exactly.

- [ ] **Step 6: Record the LOC delta.**
  Run `find gen -name '*.go' -print0 | xargs -0 cat | wc -l` in `models/` and
  note the new total in the task ledger.

- [ ] **Step 7: Commit (contrib).**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/template/where_input_subpkg.tmpl entgql/template_test.go
git commit -m "feat(entgql): route WhereInput P()/Filter through gqlwhere walker"
```

---

### Task 3: `gqlpage` order fields — generic types, registry, GQL marshaling

**Files:**
- Create: `entgql/gqlpage/order.go` (contrib)
- Create: `entgql/gqlpage/order_test.go` (contrib)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces (Tasks 4 and 5 use these):

```go
package gqlpage

// Valuer is the model-side accessor for non-column order terms
// (edge counts, computed fields). Generated entity structs already have it.
type Valuer interface{ Value(name string) (ent.Value, error) }

type OrderField[T any, ID any] struct {
	// Value extracts the ordering value from the given entity.
	Value func(*T) (ent.Value, error)
	// unexported: gql (enum name), column, expression, toTerm, structField
}

func (f OrderField[T, ID]) Column() string
func (f OrderField[T, ID]) Expression() string
func (f OrderField[T, ID]) Term(opts ...sql.OrderTermOption) func(*sql.Selector)
func (f OrderField[T, ID]) Cursor(v *T) entgql.Cursor[ID]
func (f OrderField[T, ID]) String() string
func (f OrderField[T, ID]) MarshalGQL(w io.Writer)
func (f *OrderField[T, ID]) UnmarshalGQL(v any) error

type Order[T any, ID any] struct {
	Direction      entgql.OrderDirection `json:"direction"`
	Field          *OrderField[T, ID]    `json:"field"`
	NullsDirection entgql.NullsDirection `json:"nullsDirection"`
}

// Column builds an order field backed by a real column.
//   gql:         the GraphQL enum value (e.g. "NAME")
//   column:      the SQL column constant
//   structField: the Go struct field to read for Value/Cursor (e.g. "Name")
//   term:        the entity handle's Order method value (F.Name.Order)
//   opts:        Expr(...) for an ORDER BY expression override
func Column[T any, ID any](gql, column, structField string,
    term func(...sql.OrderTermOption) func(*sql.Selector),
    opts ...ColumnOption) *OrderField[T, ID]

// Expr overrides the ORDER BY / cursor expression and supplies the
// term builder that writes it (replacing the 15-line inline literal).
func Expr(expression string) ColumnOption

// Computed builds an order field whose value comes from T's Value(name)
// method rather than a struct field — edge counts and computed terms.
func Computed[T any, ID any](gql, column, valueField string,
    term func(...sql.OrderTermOption) func(*sql.Selector)) *OrderField[T, ID]

// Register records an entity's order fields for UnmarshalGQL lookup and
// for %!s(MISSING)-style error text. Call once per entity from an init or a var.
func Register[T any, ID any](fields ...*OrderField[T, ID])
```

- [ ] **Step 1: Read the block being replaced.** Read
  `entgql/template/pagination_subpkg.tmpl` lines 65-280 (the `$orderFields`
  var block, `String`, `MarshalGQL`, `UnmarshalGQL`, `$defaultOrder`) and
  `models/gen/company/gql_pagination.go` lines 65-756. Note the three arm
  shapes: plain column, column with `expression` (the inline
  `OrderExprFunc` literal), and non-`IsFieldTerm` terms that call
  `_m.Value(...)`. Note that `UnmarshalGQL` errors read
  `%s is not a valid <X>OrderField`.

- [ ] **Step 2: Write the failing tests.** `order_test.go` with a fake entity
  struct carrying a `Name string`, an `ID uuid.UUID` and a
  `Value(string) (ent.Value, error)` method. Cover: `Column` reads the struct
  field for both `Value` and `Cursor`; `Expr` sets `Expression()` and makes
  `Term` emit the raw expression with `DESC` / `NULLS FIRST` / `NULLS LAST`
  exactly as the old inline literal did; `Computed` routes through `Valuer`;
  `String`/`MarshalGQL` emit the GQL enum name; `UnmarshalGQL` resolves a
  registered name, rejects a non-string with `fmt.Errorf("enum %T must be a string", v)`
  and rejects an unknown name with the `%s is not a valid` message; and that
  the struct-field index is resolved once and cached.

- [ ] **Step 3: Run the tests and confirm they fail.**
  Run: `go test ./entgql/gqlpage/... -v` in contrib.

- [ ] **Step 4: Implement `order.go`.** `Column` resolves `structField` to a
  `reflect.StructField` index lazily on first use (`sync.Once` per field
  value) and panics with the type and field name if absent — a codegen bug
  must fail loudly at first use, not silently return a zero cursor. `Expr`
  supplies a `toTerm` built from `sql.NewOrderTermOptions` + `OrderExprFunc`,
  reproducing the generated literal exactly (expression, then `" DESC"` when
  `o.Desc`, then `" NULLS FIRST"` / `" NULLS LAST"`). `Register` stores into a
  package map keyed by `reflect.TypeFor[T]()`.

- [ ] **Step 5: Run the tests and confirm they pass.**
  Run: `go test ./entgql/gqlpage/... -v` in contrib. Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/gqlpage
git commit -m "feat(entgql): generic OrderField with reflection-backed value/cursor"
```

---

### Task 4: `gqlpage` connection, pager and `Paginate`

**Files:**
- Create: `entgql/gqlpage/page.go` (contrib)
- Create: `entgql/gqlpage/page_test.go` (contrib)

**Interfaces:**
- Consumes: `OrderField`, `Order`, `Valuer` from Task 3.
- Produces (Task 5 generates these):

```go
package gqlpage

type Edge[T any, ID any] struct {
	Node   *T                `json:"node"`
	Cursor entgql.Cursor[ID] `json:"cursor"`
}

type Connection[T any, ID any] struct {
	Edges      []*Edge[T, ID]      `json:"edges"`
	PageInfo   entgql.PageInfo[ID] `json:"pageInfo"`
	TotalCount int                 `json:"totalCount"`
}

func (c *Connection[T, ID]) Build(nodes []*T, pager *Pager[Q, T, ID], after *entgql.Cursor[ID], first *int, before *entgql.Cursor[ID], last *int)

// Ops is the per-entity glue: everything the pager needs from *Q that is not
// expressible as a shared method set. Emitted once per entity (~14 lines).
type Ops[Q any, T any, ID any] struct {
	Where       func(*Q, func(*sql.Selector)) *Q
	Order       func(*Q, func(*sql.Selector)) *Q
	Limit       func(*Q, int) *Q
	Clone       func(*Q) *Q
	All         func(*Q, context.Context) ([]*T, error)
	Count       func(*Q, context.Context) (int, error)
	Fields      func(*Q) []string
	AppendField func(*Q, string)
	ClearFields func(*Q)
	ID          func(*T) ID
	Default     *Order[T, ID]
	MultiOrder  bool
	// EdgeTermColumns are the columns of non-column order terms (the old
	// $byEdges switch). ApplyOrder/OrderExpr must not AppendFieldOnce these.
	EdgeTermColumns []string
	// Collect is the registered collectField hook, or nil.
	Collect func() func(*Q, context.Context, bool, *graphql.OperationContext, graphql.CollectedField, []string, ...string) error
}

type Pager[Q any, T any, ID any] struct{ /* unexported: ops, order, filter, reverse */ }

type Option[Q any, T any, ID any] func(*Pager[Q, T, ID]) error

func WithOrder[Q any, T any, ID any](order []*Order[T, ID]) Option[Q, T, ID]
func WithFilter[Q any, T any, ID any](filter func(*Q) (*Q, error)) Option[Q, T, ID]

func NewPager[Q any, T any, ID any](ops *Ops[Q, T, ID], opts []Option[Q, T, ID], reverse bool) (*Pager[Q, T, ID], error)

func (p *Pager[Q, T, ID]) ApplyFilter(q *Q) (*Q, error)
func (p *Pager[Q, T, ID]) ApplyCursors(q *Q, after, before *entgql.Cursor[ID]) (*Q, error)
func (p *Pager[Q, T, ID]) ApplyOrder(q *Q) *Q
func (p *Pager[Q, T, ID]) OrderExpr(q *Q) sql.Querier
func (p *Pager[Q, T, ID]) ToCursor(v *T) entgql.Cursor[ID]

func Paginate[Q any, T any, ID any](q *Q, ctx context.Context,
    after *entgql.Cursor[ID], first *int, before *entgql.Cursor[ID], last *int,
    ops *Ops[Q, T, ID], opts ...Option[Q, T, ID]) (*Connection[T, ID], error)

func ToEdge[T any, ID any](v *T, order *Order[T, ID], def *Order[T, ID]) *Edge[T, ID]

// Shared helpers, previously copied into all 157 entity packages.
func ValidateFirstLast(first, last *int) *gqlerror.Error
func CollectedField(ctx context.Context, path ...string) *graphql.CollectedField
func HasCollectedField(ctx context.Context, path ...string) bool
func PaginateLimit(first, last *int) int
```

- [ ] **Step 1: Read the bodies being replaced.** Read
  `entgql/template/pagination_subpkg.tmpl` lines 280-786 and
  `entgql/template/pagination_shared.tmpl`. Enumerate every behavior:
  `validateFirstLast`'s two `errcode` gqlerrors; `paginateLimit`'s
  first/last arithmetic; `Connection.Build`'s reverse handling, head/tail
  trimming and `PageInfo` computation; `newPager`'s multi-order validation;
  `ApplyCursors`'s single vs multi-order split (`entgql.CursorsPredicate`,
  `CursorsPredicateExpr`, `MultiCursorsPredicate`) with the reverse
  direction flips; `ApplyOrder`'s default-order append and the `$byEdges`
  `AppendFieldOnce` suppression; `OrderExpr`'s `sql.ExprFunc` builder
  including the `NullsLast` default and the trailing default-column term;
  and `Paginate`'s totalCount / ignoredEdges / limit / collectField sequence.
  The single-order path is the `len(order) == 1` case of the multi path
  everywhere except `ApplyCursors` and `ApplyOrder` — keep `MultiOrder` as
  an explicit flag rather than inferring it from length, because a
  multi-order entity with one order term must still take the multi path.

- [ ] **Step 2: Write the failing tests.** `page_test.go` with a fake `Q`
  that records the calls made against it (a slice of applied
  `func(*sql.Selector)` plus limit/clone/count/all counters) and a fake `T`.
  Cover: `ValidateFirstLast` rejecting both-set and negative values with the
  exact old messages; `PaginateLimit` for first, last, neither;
  `Connection.Build` for forward, backward, over-fetch trimming and the empty
  case; `ApplyOrder` appending the default order when the requested field is
  not the default, and skipping `AppendField` for a column listed in
  `EdgeTermColumns`; `ApplyOrder` not calling `AppendField` when `Fields` is
  empty; `OrderExpr` producing the expected SQL fragment for a plain column,
  for an `Expr` field, and for multi-order; `ApplyCursors` selecting the
  expression predicate when the order field has one; and `Paginate`'s
  early-return paths (ignored edges, `first == 0`).

- [ ] **Step 3: Run the tests and confirm they fail.**
  Run: `go test ./entgql/gqlpage/... -run 'Page|Pager|Paginate|Connection' -v` in contrib.

- [ ] **Step 4: Implement `page.go`,** transcribing each behavior enumerated
  in Step 1. Where the old template branched on `$multiOrder` at codegen
  time, branch on `ops.MultiOrder` at runtime.

- [ ] **Step 5: Run the tests and confirm they pass.**
  Run: `go test ./entgql/gqlpage/... -v` in contrib. Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/gqlpage
git commit -m "feat(entgql): generic connection, pager and Paginate"
```

---

### Task 5: Rewrite the pagination templates onto `gqlpage`

**Files:**
- Modify: `entgql/template/pagination_subpkg.tmpl` (contrib)
- Modify: `entgql/template/pagination_entity.tmpl` (contrib)
- Modify: `entgql/template/pagination_shared.tmpl` (contrib)
- Modify: `entgql/template_test.go` (contrib)

**Interfaces:**
- Consumes: everything from Tasks 3 and 4.
- Produces: per-entity public surface, now as aliases —

```go
type (
	CompanyEdge           = gqlpage.Edge[Company, uuid.UUID]
	CompanyConnection     = gqlpage.Connection[Company, uuid.UUID]
	CompanyOrder          = gqlpage.Order[Company, uuid.UUID]
	CompanyOrderField     = gqlpage.OrderField[Company, uuid.UUID]
	CompanyPaginateOption = gqlpage.Option[CompanyQuery, Company, uuid.UUID]
)
```
  plus `CompanyQueryPaginate`, `NewCompanyPager`, `WithCompanyOrder`,
  `WithCompanyFilter`, `CompanyToEdge`, `DefaultCompanyOrder`,
  `RegisterCompanyQueryCollectFieldFn` and the `CompanyOrderField<X>` vars —
  all with unchanged names and signatures.

- [ ] **Step 1: Emit the alias block and the `Ops` literal.** Replace the
  five type declarations with the alias block above, and emit one `Ops`
  literal per entity:

```go
var companyOps = &gqlpage.Ops[CompanyQuery, Company, uuid.UUID]{
	Where:       func(q *CompanyQuery, p func(*sql.Selector)) *CompanyQuery { return q.Where(p) },
	Order:       func(q *CompanyQuery, o func(*sql.Selector)) *CompanyQuery { return q.Order(o) },
	Limit:       func(q *CompanyQuery, n int) *CompanyQuery { return q.Limit(n) },
	Clone:       func(q *CompanyQuery) *CompanyQuery { return q.Clone() },
	All:         func(q *CompanyQuery, ctx context.Context) ([]*Company, error) { return q.All(ctx) },
	Count:       func(q *CompanyQuery, ctx context.Context) (int, error) { return q.Count(ctx) },
	Fields:      func(q *CompanyQuery) []string { return q.Ctx.Fields },
	AppendField: func(q *CompanyQuery, f string) { q.Ctx.AppendFieldOnce(f) },
	ClearFields: func(q *CompanyQuery) { q.Ctx.Fields = nil },
	ID:          func(_m *Company) uuid.UUID { return _m.ID },
	Default:     DefaultCompanyOrder,
	MultiOrder:  false,
	Collect:     func() func(*CompanyQuery, context.Context, bool, *graphql.OperationContext, graphql.CollectedField, []string, ...string) error { return collectFieldFnCompanyQuery },
}
```

  The `Ctx` accessors are closures rather than an interface constraint
  because `Ctx` is a struct field on the generated query, not a method.
  Where the entity's ID is marshaled (`$idType.Mixed` + `gqlMarshaler`), `ID`
  returns `_m.marshalID()` as today.

- [ ] **Step 2: Collapse the order-field var block to one line per field.**

```go
var CompanyOrderFieldName = gqlpage.Column[Company, uuid.UUID]("NAME", FieldName, "Name", F.Name.Order, gqlpage.Expr(`left("name", 256)`))
var CompanyOrderFieldPropertiesCount = gqlpage.Computed[Company, uuid.UUID]("PROPERTIES_COUNT", "properties_count", "properties_count", <term>)
```
  `<term>` for a non-field term is the existing generated closure that builds
  the edge-count order — keep the template's current expression for it, only
  the surrounding struct literal goes away. Follow the block with a single
  `gqlpage.Register[Company, uuid.UUID](CompanyOrderFieldCreatedAt, ...)`
  call in an `init()`.

- [ ] **Step 3: Delete the per-package method and helper bodies.** Remove
  `String`, `MarshalGQL`, `UnmarshalGQL`, `Connection.Build`, the `Pager`
  type and all its methods, `newXPager`, and the four copied helpers
  (`validateFirstLast`, `collectedField`, `hasCollectedField`,
  `paginateLimit`). Replace the remaining free functions with thin
  forwarders:

```go
func WithCompanyOrder(order []*CompanyOrder) CompanyPaginateOption {
	return gqlpage.WithOrder[CompanyQuery, Company, uuid.UUID](order)
}

func WithCompanyFilter(filter func(*CompanyQuery) (*CompanyQuery, error)) CompanyPaginateOption {
	return gqlpage.WithFilter[CompanyQuery, Company, uuid.UUID](filter)
}

func NewCompanyPager(opts []CompanyPaginateOption, reverse bool) (*gqlpage.Pager[CompanyQuery, Company, uuid.UUID], error) {
	return gqlpage.NewPager(companyOps, opts, reverse)
}

func CompanyQueryPaginate(_q *CompanyQuery, ctx context.Context, after *Cursor, first *int, before *Cursor, last *int, opts ...CompanyPaginateOption) (*CompanyConnection, error) {
	return gqlpage.Paginate(_q, ctx, after, first, before, last, companyOps, opts...)
}

func CompanyToEdge(_m *Company, order *CompanyOrder) *CompanyEdge {
	return gqlpage.ToEdge(_m, order, DefaultCompanyOrder)
}
```

  `WithCompanyOrder` keeps its non-multi-order signature (`*CompanyOrder`,
  not a slice) where `$multiOrder` is false — wrap into a one-element slice
  inside the forwarder.

- [ ] **Step 4: Update `pagination_entity.tmpl`.** The root-level forwarder
  file keeps its type aliases and var forwarders unchanged; verify it still
  compiles against the new subpackage names and drop any forwarder whose
  target no longer exists.

- [ ] **Step 5: Update `pagination_shared.tmpl`.** The shared root-level
  cursor/pageinfo aliases stay. Delete anything that duplicates a
  `gqlpage` helper.

- [ ] **Step 6: Run contrib's tests.**
  Run: `go build ./entgql/... && go test ./entgql/...` in contrib. Update
  `TestPaginationSharedTemplate*` and `TestPaginationEntityTemplate*`
  assertions to the new expected text. Expected: PASS.

- [ ] **Step 7: Regenerate gemini and build.** Same commands as Task 2 Step 4.
  Expected: clean build. The alias route means gqlgen's model bindings and
  every gemini resolver referencing `gen.CompanyConnection` keep resolving —
  if any gemini file declares a *method* on one of these five types, the
  build will fail here; report it rather than reverting to named structs.

- [ ] **Step 8: Prove ORDER BY parity.** Same before/after harness as Task 2
  Step 5: for one plain-column order, one `Expr` order and one edge-count
  order, print `pager.OrderExpr(q)`'s rendered SQL and the `ApplyOrder`
  selector output, and diff against the pre-Task-5 tree.

- [ ] **Step 9: Record the LOC delta and commit (contrib).**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/template entgql/template_test.go
git commit -m "feat(entgql): pagination types alias generic gqlpage instantiations"
```

---

### Task 6: `gqlinput` mutation-input walker

**Files:**
- Create: `entgql/gqlinput/gqlinput.go` (contrib)
- Create: `entgql/gqlinput/gqlinput_test.go` (contrib)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces (Task 7 generates calls to `Mutate`):

```go
package gqlinput

// Mutator is the subset of entbuilder.Mutation[T, I]'s method set that
// mutation inputs need. Declared locally so contrib does not have to import
// (or pin) the ent fork's runtime package.
type Mutator interface {
	SetField(name string, value any) error
	AppendField(name string, value any) error
	ClearField(name string) error
	SetEdgeID(edge string, id any) error
	AddEdgeIDs(edge string, ids ...any) error
	RemoveEdgeIDs(edge string, ids ...any) error
	ClearEdge(edge string) error
}

// Mutate applies every set field of input onto m, in struct declaration
// order, driven by each field's `mutate:"<op>:<name>"` tag.
//
// Ops: f (SetField), fc (ClearField), fa (AppendField),
//      e (SetEdgeID), ea (AddEdgeIDs), er (RemoveEdgeIDs), ec (ClearEdge).
//
// Errors from the Mutator can only fire on descriptor mismatch — a codegen
// bug, not a runtime condition — so they are discarded, matching the
// behavior of the generated bodies this replaces.
func Mutate(input any, m Mutator)
```

- [ ] **Step 1: Read the body being replaced.** Read
  `entgql/template/mutation_input_sibling.tmpl` and
  `models/gen/mutationinputs/company.go`. Note the emission order — per
  field: `ClearX`, then `X`, then `AppendX`; per edge: `ClearX`, then the
  unique/non-unique and create/update variants. Note that a non-pointer,
  non-nillable field is set unconditionally (no nil guard), that
  `AppendField` guards on `i.<MutationAppend> != nil` but passes
  `i.<StructField>`, and that `AddEdgeIDs`/`RemoveEdgeIDs` guard on `len > 0`.

- [ ] **Step 2: Choose tags over name inference, and record why.** The walker
  is driven by struct tags, not by converting Go names to snake_case: the
  round trip is lossy (`ZoomInfoCompanyID` → `zoom_info_company_id`,
  `SfObject` → `sf_object`) and a wrong guess writes to the wrong column.
  Tags cost zero generated lines because they attach to struct fields that
  already exist. Put this reasoning in a comment at the top of `gqlinput.go`.

- [ ] **Step 3: Write the failing tests.** Cover each of the seven ops
  against a recording `Mutator`; the unconditional-set case for a
  non-pointer field; skipping a nil pointer, a nil slice and an empty slice;
  a `false` bool for `fc`/`ec` being skipped and `true` firing; declaration
  order being preserved across a mixed field/edge struct; boxing of a typed
  ID slice into `...any`; and an untagged exported field being ignored.

- [ ] **Step 4: Run the tests and confirm they fail.**
  Run: `go test ./entgql/gqlinput/... -v` in contrib.

- [ ] **Step 5: Implement `gqlinput.go`,** caching the parsed per-type plan in
  a `sync.Map` keyed by `reflect.Type`, exactly as `gqlwhere` does.

- [ ] **Step 6: Run the tests and confirm they pass.**
  Run: `go test ./entgql/gqlinput/... -v` in contrib. Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/gqlinput
git commit -m "feat(entgql): tag-driven Mutate walker for mutation inputs"
```

---

### Task 7: Rewrite `mutation_input_sibling.tmpl` onto `gqlinput`

**Files:**
- Modify: `entgql/template/mutation_input_sibling.tmpl` (contrib)
- Modify: `entgql/template_test.go` (contrib)

**Interfaces:**
- Consumes: `gqlinput.Mutate` from Task 6.
- Produces: unchanged surface — `CreateXInput`, `UpdateXInput`,
  `(i XInput) Mutate(*XMutation)`, `SetXCreateInput` / `SetXUpdateInput` /
  `SetXUpdateOneInput`.

- [ ] **Step 1: Add the `mutate` tags to the struct block.** Every emitted
  field gains a tag naming its op and descriptor name, e.g.:

```go
ClearName       bool                 `mutate:"fc:name"`
Name            *string              `mutate:"f:name"`
AppendPrimaryFocus pq.StringArray    `mutate:"fa:primary_focus"`
ClearParentCompany bool              `mutate:"ec:parent_company"`
ParentCompanyID *uuid.UUID           `mutate:"e:parent_company"`
AddContactIDs   []uuid.UUID          `mutate:"ea:contacts"`
RemoveContactIDs []uuid.UUID         `mutate:"er:contacts"`
```

  Field and edge declaration order is unchanged, so the walker reproduces the
  old emission order.

- [ ] **Step 2: Replace the `Mutate` body.**

```go
// Mutate applies the {{ $input }} on the {{ $n.MutationName }} builder.
// Field/edge routing is driven by the `mutate` struct tags above; see
// entgo.io/contrib/entgql/gqlinput.
func (i {{ $input }}) Mutate(m *{{ $pkg }}.{{ $n.MutationName }}) {
	gqlinput.Mutate(i, m)
}
```

  Swap the `entbuilder` import for `entgo.io/contrib/entgql/gqlinput`. The
  `entbuilder.ToAny` calls go away — `gqlinput` boxes ID slices itself.

- [ ] **Step 3: Run contrib's tests and update assertions.**
  Run: `go build ./entgql/... && go test ./entgql/...` in contrib. Expected: PASS.

- [ ] **Step 4: Regenerate gemini and build.** Same commands as Task 2 Step 4.

- [ ] **Step 5: Prove parity on one input.** Write a throwaway test in
  `models/` that constructs a fully-populated `UpdateCompanyInput` (a set
  field, a cleared field, an appended field, a set edge, a cleared edge, an
  added and a removed edge-ID slice), runs `Mutate` against a real
  `CompanyMutation`, and dumps the mutation's fields/edges. Diff against the
  same dump from a throwaway worktree of the pre-Task-7 tree.

- [ ] **Step 6: Record the LOC delta and commit (contrib).**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/template/mutation_input_sibling.tmpl entgql/template_test.go
git commit -m "feat(entgql): route MutationInput.Mutate through gqlinput walker"
```

---

### Task 8: `gqlcollect` field-collection runtime

**Files:**
- Create: `entgql/gqlcollect/gqlcollect.go` (contrib)
- Create: `entgql/gqlcollect/gqlcollect_test.go` (contrib)

**Interfaces:**
- Consumes: `gqlpage.Option` from Task 4 (paginated edges build pager options).
- Produces (Task 9 generates these):

```go
package gqlcollect

// Edge describes one collectable GraphQL edge field of an entity. Every
// typed operation is a generated closure, so the runtime stays type-erased.
type Edge struct {
	// GQL is the GraphQL field name this arm matches.
	GQL string
	// NewQuery builds a fresh target-entity query from the parent's config.
	NewQuery func(parent any) any
	// Collect recurses into the target entity's collectField.
	Collect func(q any, ctx context.Context, oneNode bool, opCtx *graphql.OperationContext, field graphql.CollectedField, path []string, satisfies ...string) error
	// Attach wires the built sub-query onto the parent query. alias is "" for
	// unnamed (unique-edge) attachment.
	Attach func(parent any, alias string, sub any)
	// Implementors is the target entity's Implementors slice.
	Implementors []string
	// FKColumn, when non-empty, is added to the parent's selected fields.
	FKColumn string
	// OneNode forces oneNode=true on the recursive call (unique edges).
	OneNode bool
	// Paginated marks a relay-connection edge; PaginateArgs and Paginate are
	// non-nil exactly when this is true.
	Paginated bool
	PaginateArgs func(rv map[string]any) *Args
	Paginate     func(parent any, sub any, args *Args, ctx context.Context, field graphql.CollectedField, path []string) error
}

// Field describes one collectable scalar field: the GraphQL name and the
// column to add to the parent's selection.
type Field struct {
	GQL    string
	Column string
}

// Spec is one entity's collection table, built once per entity.
type Spec struct {
	IDColumn   string
	Columns    []string
	Edges      []Edge
	Fields     []Field
	// Select applies the accumulated column selection to the parent query.
	Select func(parent any, columns []string)
}

// Collect is the generic replacement for the per-entity collectField switch.
func Collect(spec *Spec, parent any, ctx context.Context, oneNode bool,
	opCtx *graphql.OperationContext, collected graphql.CollectedField,
	path []string, satisfies ...string) error

// MayAddCondition is the shared helper previously copied per entity.
func MayAddCondition(satisfies []string, implementors []string) []string
```

- [ ] **Step 1: Read the body being replaced.** Read
  `entgql/template/gql_collection_subpkg.tmpl`,
  `entgql/template/gql_collection_subpkg_runtime.tmpl`,
  `entgql/template/collection_shared.tmpl`, and
  `models/gen/gqlcollections/company.go` end to end. Enumerate every arm
  shape: unique edge with an FK column; unique edge without one; non-unique
  edge via `WithNamedX`; relay-connection edge with its paginate-args block,
  `hasCollectedField(totalCount)` handling, limit/offset modifier and
  `WithNamed...` attachment; the scalar-field arms; the `unknownSeen`
  fallthrough; and the trailing `if !unknownSeen { _q.Select(selectedFields...) }`.
  Confirm whether the relay arms differ between entities in anything other
  than types and names — if an arm's *control flow* varies, it stays
  generated rather than moving into the runtime; record which.

- [ ] **Step 2: Write the failing tests.** With a fake parent query type and
  two fake edges (one unique with an FK column, one named non-unique), assert:
  the matching arm fires for the collected field name and its alias is passed
  to `Attach`; `FKColumn` is added exactly once even when the field is
  collected twice under different aliases; a scalar `Field` adds its column;
  an unknown field sets `unknownSeen` and suppresses the final `Select`; the
  ID column is always first in the selection; and `MayAddCondition` matches
  the old helper's output for overlapping and disjoint implementor sets.

- [ ] **Step 3: Run the tests and confirm they fail.**
  Run: `go test ./entgql/gqlcollect/... -v` in contrib.

- [ ] **Step 4: Implement `gqlcollect.go`.** Match arms via a
  `map[string]int` from GQL field name to `Edges`/`Fields` index, built once
  in `Spec` on first use; preserve the old `fieldSeen` / `selectedFields`
  semantics exactly.

- [ ] **Step 5: Run the tests and confirm they pass.**
  Run: `go test ./entgql/gqlcollect/... -v` in contrib. Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/gqlcollect
git commit -m "feat(entgql): descriptor-driven GraphQL field collection"
```

---

### Task 9: Rewrite `gql_collection_subpkg.tmpl` onto `gqlcollect`

**Files:**
- Modify: `entgql/template/gql_collection_subpkg.tmpl` (contrib)
- Modify: `entgql/template/gql_collection_subpkg_runtime.tmpl` (contrib)
- Modify: `entgql/template_test.go` (contrib)

**Interfaces:**
- Consumes: `gqlcollect.Spec`, `Edge`, `Field`, `Collect`, `MayAddCondition`.
- Produces: unchanged surface — `XQueryCollectFields`,
  `CollectFieldXQuery`, `collectFieldXQuery`, the `init()` registration,
  `xPaginateArgs` and `newXPaginateArgs`.

- [ ] **Step 1: Emit one `Spec` per entity** whose `Edges` entries are the
  compact literals `gqlcollect.Edge{...}` with the four generated closures,
  and whose `Fields` entries are the scalar arms. Keep `newXPaginateArgs`
  generated as-is — it unmarshals into per-entity `Order`/`WhereInput` types
  and is only ~50 lines.

- [ ] **Step 2: Replace `collectFieldXQuery`'s body** with a single
  `return gqlcollect.Collect(companySpec, _q, ctx, oneNode, opCtx, collected, path, satisfies...)`.
  Keep `XQueryCollectFields`, `CollectFieldXQuery` and the `init()`
  registration exactly as they are.

- [ ] **Step 3: Move `mayAddCondition` out of the per-package runtime file**
  into `gqlcollect.MayAddCondition`, leaving
  `gql_collection_subpkg_runtime.tmpl` with only what genuinely must be
  per-package.

- [ ] **Step 4: Run contrib's tests and update assertions.**
  Run: `go build ./entgql/... && go test ./entgql/...` in contrib. Update
  `TestCollectionSharedTemplate*`, `TestCollectionEntityTemplate*` and
  `TestCollectionSubpkgTemplate*` expectations. Expected: PASS.

- [ ] **Step 5: Regenerate gemini and build.** Same commands as Task 2 Step 4.

- [ ] **Step 6: Prove collection parity.** Run gemini's `ent_resolvers` suite
  (`task test-integration -- ent_resolvers` from `api/`) — it is the
  suite that exercises nested field collection. Expected: 285 pass + 1 skip.
  This is the one lever whose parity cannot be shown by a unit diff.

- [ ] **Step 7: Record the LOC delta and commit (contrib).**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/template entgql/template_test.go
git commit -m "feat(entgql): route collectField through gqlcollect spec"
```

---

### Task 10: Generic edge resolvers

**Files:**
- Create: `entgql/gqledge/gqledge.go` (contrib)
- Create: `entgql/gqledge/gqledge_test.go` (contrib)
- Modify: `entgql/template/gql_edge_subpkg.tmpl` (contrib)
- Modify: `entgql/template/gql_edge_subpkg_runtime.tmpl` (contrib)
- Modify: `entgql/template_test.go` (contrib)

**Interfaces:**
- Consumes: `gqlpage.Connection`, `gqlpage.Option`, `gqlpage.Paginate`,
  `gqlpage.Ops` from Tasks 4 and 5.
- Produces:

```go
package gqledge

// One resolves a unique edge: try the loaded Edges value, fall back to a
// query when not loaded, and optionally mask a not-found error.
func One[T any](loaded func() (*T, error), query func() (*T, error), notLoaded func(error) bool, mask bool) (*T, error)

// Many resolves a non-unique, non-connection edge: prefer the aliased
// Named<Edge> value inside a GraphQL context, else the plain Edges value,
// falling back to a query when not loaded.
func Many[T any](ctx context.Context, named func(string) ([]*T, error), loaded func() ([]*T, error), query func() ([]*T, error), notLoaded func(error) bool) ([]*T, error)

// Conn resolves a relay-connection edge from preloaded nodes when they are
// available, else by paginating the edge query.
func Conn[Q any, T any, ID any](ctx context.Context, ops *gqlpage.Ops[Q, T, ID],
	opts []gqlpage.Option[Q, T, ID], alias string,
	totalCount int, hasTotalCount bool,
	named func(string) ([]*T, error), query func() *Q,
	after *entgql.Cursor[ID], first *int, before *entgql.Cursor[ID], last *int,
) (*gqlpage.Connection[T, ID], error)
```

- [ ] **Step 1: Read `gql_edge_subpkg.tmpl` end to end** and confirm the three
  arm shapes (relay connection, non-unique, unique) map onto `Conn`, `Many`
  and `One` with no behavior left behind — in particular the
  `MaskNotFound` only on optional unique edges, and `Many`'s
  `fc.Field.Alias != ""` guard.

- [ ] **Step 2: Write the failing tests** for all three, covering loaded /
  not-loaded / not-found / masked paths and, for `Many`, both the aliased and
  the non-GraphQL-context paths.

- [ ] **Step 3: Run the tests and confirm they fail.**
  Run: `go test ./entgql/gqledge/... -v` in contrib.

- [ ] **Step 4: Implement `gqledge.go`.** `notLoaded` and the mask decision
  are passed in because `IsNotLoaded` lives in the generated `gqledges`
  package (it reads `internal.NotLoadedError`), which contrib cannot import.

- [ ] **Step 5: Rewrite the template arms** to one call each, e.g.:

```go
// ResolveCompanyOwner resolves the owner edge of a Company.
func ResolveCompanyOwner(_m *company.Company, ctx context.Context) (*user.User, error) {
	return gqledge.One(_m.Edges.OwnerOrErr, func() (*user.User, error) {
		return edges.QueryCompanyOwner(companyClientFromCtx(ctx), _m).Only(ctx)
	}, IsNotLoaded, true)
}
```

- [ ] **Step 6: Run contrib's tests and update assertions.**
  Run: `go build ./entgql/... && go test ./entgql/...` in contrib.
  `TestEdgeSubpkgTemplate*` will need new expectations. Expected: PASS.

- [ ] **Step 7: Regenerate gemini and build.** Same commands as Task 2 Step 4.

- [ ] **Step 8: Record the LOC delta and commit (contrib).**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/gqledge entgql/template entgql/template_test.go
git commit -m "feat(entgql): generic edge resolvers"
```

---

### Task 11: Generic node resolvers

**Files:**
- Modify: `entgql/template/node_entity.tmpl` (contrib)
- Modify: `entgql/template/node_shared.tmpl` (contrib)
- Modify: `entgql/template_test.go` (contrib)

**Interfaces:**
- Consumes: nothing new; the shared helper lives in the generated root
  package because it needs `*Client` and `Noder`, both root-local types.
- Produces: unchanged `xNoder` / `xNoders` signatures and the same
  `registerNodeResolver` registration.

- [ ] **Step 1: Add two generic helpers to `node_shared.tmpl`** (they are
  emitted once into root `gen`, not per entity):

```go
func noderOf[T Noder, Q any](ctx context.Context, id ID, q *Q,
	where func(*Q, ID) *Q,
	collect func(*Q, context.Context, bool, *graphql.OperationContext, graphql.CollectedField, []string, ...string) error,
	only func(*Q, context.Context) (T, error),
	implementors []string) (Noder, error)

func nodersOf[T Noder, Q any](ctx context.Context, ids []ID, idmap map[ID][]*Noder, q *Q,
	whereIn func(*Q, []ID) *Q,
	collectAll func(*Q, context.Context, ...string) (*Q, error),
	all func(*Q, context.Context) ([]T, error),
	key func(T) ID,
	implementors []string) error
```

- [ ] **Step 2: Reduce each entity's arm in `node_entity.tmpl`** to a call
  passing the five closures. The `marshalID` and `UnmarshalGQL` id
  conversions stay in the generated arm — they are per-entity by nature.

- [ ] **Step 3: Run contrib's tests and update assertions.**
  Run: `go build ./entgql/... && go test ./entgql/...` in contrib. Expected: PASS.

- [ ] **Step 4: Regenerate gemini and build.** Same commands as Task 2 Step 4.

- [ ] **Step 5: Verify the node interface still resolves.** Run gemini's
  `ent_resolvers` suite; it covers `Noder`/`Noders`. Expected: 285 pass + 1 skip.

- [ ] **Step 6: Record the LOC delta and commit (contrib).**

```bash
cd /home/smoothbrain/dev/matthews/contrib
git add entgql/template entgql/template_test.go
git commit -m "feat(entgql): generic node resolvers"
```

---

### Task 12: Gemini extension templates and call-site fallout

**Files:**
- Modify (gemini worktree, **never commit**):
  `models/extensions/entsearch/templates/search_pagination.tmpl`,
  `search_whereinput.tmpl`, `search_filters_whereinput.tmpl`, and whatever
  else the build names.

**Interfaces:**
- Consumes: everything Tasks 2-11 produced.
- Produces: a green `go build ./...` across the whole gemini module.

- [ ] **Step 1: Full regen and full build.**
  Run: `MREIS_CODEGEN_ALLOW_WATCHER=1 task generate-go` from `api/`, then
  `go build ./...` from the gemini worktree root (not just `models/`).

- [ ] **Step 2: Fix the extension templates.** `search_pagination.tmpl`
  duplicates parts of the pager; `search_whereinput.tmpl` and
  `search_filters_whereinput.tmpl` add methods to `XWhereInput`. Adapt each
  to the new surface. These are app-side extension templates, explicitly out
  of the reduction's scope — adapt them minimally, do not refactor them.

- [ ] **Step 3: Fix any remaining call sites.** If the surface really is
  unchanged (it should be — every rename in Tasks 2-11 was avoided
  deliberately), there will be few. If a mechanical, repeated rewrite is
  needed across many files, extend `tools/handlerewrite` in the fork rather
  than hand-editing — and note that this is the one case where a fork code
  change is expected in this stage.

- [ ] **Step 4: Confirm the build is clean.**
  Run: `go build ./...` from the gemini worktree root and from `models/`.
  Run: `go vet ./models/gen/...`. Expected: clean, except the known
  `NotFoundError` composite-literal advisory in `gql_node.go` (a user-owned
  contrib follow-up, do not fix here).

- [ ] **Step 5: Record the final LOC.**
  Run: `find gen -name '*.go' -print0 | xargs -0 cat | wc -l` in `models/`.

---

### Task 13: Full gemini test suites

**Files:** none modified (unless a failure demands a fix).

- [ ] **Step 1: Run the models unit tests.**
  Run: `go test ./... -p 2 -parallel 4` from `models/`.

- [ ] **Step 2: Run the workers suite.**
  Run: `go test ./... -p 2 -parallel 4` from `workers/`. A
  `pq: could not write block` failure is IntegreSQL pool contention, not a
  code defect — re-run rather than chasing it.

- [ ] **Step 3: Run the api integration suites** from `api/`:
  `task test-integration -- OnConflict` (expect 47),
  `-- ent_resolvers` (expect 285 + 1 skip),
  `-- transaction` (expect 117),
  `-- contact` (expect 109),
  `-- chatter` (expect 66),
  `-- box` (expect 89).

- [ ] **Step 4: Triage.** Any count below expectation is a stage-4 regression
  until proven otherwise. Bisect by lever: the five contrib commits are
  independent enough to revert one at a time in a throwaway worktree.

---

### Task 14: Benchmarks

**Files:** none.

- [ ] **Step 1: Wait for a settled box.** Check `uptime`; do not start until
  the 1-minute load average is below 2.

- [ ] **Step 2: Measure LOC.**
  Run: `find gen -name '*.go' -print0 | xargs -0 cat | wc -l` in `models/`.
  Baseline: 1,049,567.

- [ ] **Step 3: Measure generation.**
  Run: `MREIS_CODEGEN_ALLOW_WATCHER=1 task generate-go` from `api/` (it wraps
  `/usr/bin/time -v`). Record wall time and peak RSS. Baseline: 106.3s / 3.0GB.
  The gen wall carries a local-replace from-source artifact — documented in
  the results doc, and it applies equally before and after.

- [ ] **Step 4: Measure the clean build.**
  Run: `go clean -cache && /usr/bin/time -v go build ./...` in `models/`.
  Record wall time and peak RSS. Baseline: 67.0s / ~3.45GB.

- [ ] **Step 5: If any number looks confounded, run the three-pass protocol**
  used in the stage 2b results section and report the quiet-pass medians.

---

### Task 15: Results doc and PR

**Files:**
- Modify: `CODEGEN_REDUCTION_RESULTS.md` (fork)

- [ ] **Step 1: Append the stage 4 section**, following the shape of the
  stage 1-3 sections: what shipped per lever, per-lever LOC deltas, the
  benchmark table with before/after, every deviation from the spec (the
  measured ceiling being higher than the spec's −130-150k estimate; tags
  chosen over name inference in `gqlinput`; the `Ops` closure struct chosen
  over an interface constraint because `Ctx` is a field; contrib's stale
  fixtures not regenerated), and the cumulative 2,035,883 → final figure with
  the total percentage.

- [ ] **Step 2: Commit the results doc in the fork.** (The plan itself is
  already the branch's first commit.)

```bash
cd /var/home/smoothbrain/dev/matthews/ent/.claude/worktrees/say-less
git add CODEGEN_REDUCTION_RESULTS.md
git commit -m "docs: record stage 4 entgql results"
```

- [ ] **Step 3: Report, do not push.** Summarize the branch state in both
  repos and hand back for the user's review. Opening the stacked draft PR is
  the user's call.

---

## Self-Review

**Spec coverage.** Spec lines 131-141 name four items. WhereInput structs
stay and `Filter`/`P()` become a reflection walker — Tasks 1-2. Per-entity
`Paginate` bodies become generic with a small OrderField table — Tasks 3-5.
Mutation inputs get the same treatment — Tasks 6-7. `node_implementors` —
the `Implementors` slice is already a one-liner in
`gql_node_implementors_subpkg.tmpl` and needs nothing; the node *resolvers*
around it are Task 11. The spec's "Gemini's `where_input_entity.tmpl`
override rebased onto this" is a no-op: that override was already removed
(noted in the Spec section above). Levers D and E exceed the spec's stage-4
scope; the user approved them explicitly on 2026-08-29.

**Type consistency.** `gqlpage.Ops` is referenced by Tasks 4, 5 and 10 with
the same three type parameters `[Q, T, ID]` throughout. `gqlpage.Option`
likewise. `gqlwhere.Registry[P]` takes one parameter, used consistently in
Tasks 1-2. `gqlinput.Mutator` is declared in Task 6 and used unchanged in
Task 7. `gqlcollect.Spec` is declared in Task 8 and used in Task 9.

**Known open question, to be resolved in Task 8 Step 1:** whether every
relay-connection arm of `collectField` has identical control flow across
entities. If it does not, part of lever D stays generated and the −30k
estimate drops; record the finding in the results doc rather than forcing
the abstraction.
