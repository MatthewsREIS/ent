<!-- Generated: 2026-02-08 -->
<!-- Parent: ../AGENTS.md -->

# AGENTS.md — privacy

This document provides guidance for AI agents working in the **privacy** package.

**Package**: `entgo.io/ent/privacy`
**Purpose**: Access control and authorization framework for ent schemas
**Go Version**: 1.24+

---

## Overview

The `privacy` package provides a pluggable authorization framework for ent applications. It enables developers to define and enforce access control policies at the query and mutation level, allowing:

- Row-level security (RLS)
- Role-based access control (RBAC)
- Attribute-based access control (ABAC)
- Custom authorization logic per operation

**Key File**:
- `privacy.go`: Core interfaces, policies, and rule evaluation (251 lines)

---

## Core Concepts

### Policy Decisions

Three sentinel errors define policy outcomes:

```go
var (
	Allow = errors.New("ent/privacy: allow rule")
	Deny  = errors.New("ent/privacy: deny rule")
	Skip  = errors.New("ent/privacy: skip rule")
)
```

- **Allow**: Permit the operation and stop evaluation
- **Deny**: Reject the operation (default behavior unless Allow or Skip)
- **Skip**: Continue to the next rule in the policy chain

Helper functions with formatting:

```go
Allowf(format string, a ...any) error
Denyf(format string, a ...any) error
Skipf(format string, a ...any) error
```

### Rule Interfaces

**QueryRule** (line 65):
- Method: `EvalQuery(ctx context.Context, q ent.Query) error`
- Evaluates read operations; can optionally modify the query

**MutationRule** (line 74):
- Method: `EvalMutation(ctx context.Context, m ent.Mutation) error`
- Evaluates write operations (Create, Update, Delete)

**QueryMutationRule** (line 82):
- Combines QueryRule and MutationRule interfaces
- Used for rules that apply to both reads and writes

### Policy Types

**QueryPolicy** (line 70):
- Type alias: `[]QueryRule`
- Method: `EvalQuery(ctx context.Context, q ent.Query) error`
- Evaluates rules in sequence; stops on first non-Skip result

**MutationPolicy** (line 79):
- Type alias: `[]MutationRule`
- Method: `EvalMutation(ctx context.Context, m ent.Mutation) error`
- Evaluates rules in sequence; stops on first non-Skip result

**Policy** (lines 116-129):
- Struct combining separate Query and Mutation policies
- Methods: `EvalQuery()` and `EvalMutation()`

### Rule Adapters

**MutationRuleFunc** (line 90):
- Type alias: `func(context.Context, ent.Mutation) error`
- Adapter allowing plain functions to act as MutationRules
- Method: `EvalMutation(ctx, m)` delegates to the function

**OnMutationOperation** (line 98):
- Helper to wrap a rule, evaluating it only for specific mutation operations
- Useful for per-operation policies (e.g., deny deletes, allow creates)

**DenyMutationOperationRule** (line 108):
- Creates a rule denying a specific mutation operation

---

## Common Tasks

### Writing a Custom Query Rule

```go
type MyQueryRule struct{}

func (r *MyQueryRule) EvalQuery(ctx context.Context, q ent.Query) error {
	// Check if user is authorized
	userID, ok := ctx.Value("user_id").(int)
	if !ok {
		return privacy.Denyf("user_id not found in context")
	}

	// Optionally modify the query (e.g., filter to user's own data)
	// Requires assertion to specific query type

	return privacy.Skip  // Let next rule decide
}
```

### Writing a Custom Mutation Rule

```go
type MyMutationRule struct{}

func (r *MyMutationRule) EvalMutation(ctx context.Context, m ent.Mutation) error {
	userID, ok := ctx.Value("user_id").(int)
	if !ok {
		return privacy.Deny
	}

	// Allow specific operations
	if m.Op().Is(ent.OpCreate) {
		return privacy.Allow
	}

	return privacy.Skip
}
```

### Creating a Policy from Schema Methods

Users define `Policy()` methods on their schemas:

```go
func (User) Policy() ent.Policy {
	return &privacy.Policy{
		Query: privacy.QueryPolicy{
			// Query rules
		},
		Mutation: privacy.MutationPolicy{
			// Mutation rules
		},
	}
}
```

The generated ent client automatically evaluates these policies.

### Composing Multiple Rules

```go
policy := privacy.Policy{
	Mutation: privacy.MutationPolicy{
		privacy.DenyMutationOperationRule(ent.OpDelete),
		MyCustomMutationRule{},
	},
}
```

Rules are evaluated in order; first non-Skip result determines the outcome.

---

## Code Organization

### policy.go (Lines 17-30: Decisions)

Defines the three policy decision constants and formatting helpers.

### Rule & Policy Interfaces (Lines 62-86)

- **QueryRule**: Query evaluation interface
- **QueryPolicy**: Slice of QueryRules with evaluation logic
- **MutationRule**: Mutation evaluation interface
- **MutationPolicy**: Slice of MutationRules with evaluation logic
- **QueryMutationRule**: Combined interface

### Adapters & Helpers (Lines 88-113)

- **MutationRuleFunc**: Function adapter for mutations
- **OnMutationOperation**: Wraps rules to filter by operation type
- **DenyMutationOperationRule**: Pre-built rule for denying operations

### Policy Struct (Lines 115-129)

Combines query and mutation policies with evaluation methods.

### Policy Composition (Lines 131-182)

- **NewPolicies**: Creates a composite policy from schema-provided policies
- **Policies type**: Slice of `ent.Policy` with composition logic

### Policy Evaluation (Lines 184-206)

- **QueryPolicy.EvalQuery**: Evaluates query rules sequentially
- **MutationPolicy.EvalMutation**: Evaluates mutation rules sequentially

### Context Integration (Lines 208-226)

- **DecisionContext**: Embeds a policy decision in a context
- **DecisionFromContext**: Retrieves a policy decision from a context
- Allows pre-determined decisions to short-circuit rule evaluation

### Internal Types (Lines 228-250)

- **fixedDecision**: Simple rule returning a constant decision
- **contextDecision**: Rule based on context evaluation

---

## Key Patterns

### Pattern 1: Simple Always-Allow Rule

```go
rule := privacy.AlwaysAllowRule()
```

### Pattern 2: Operation-Specific Rules

```go
deleteRule := privacy.DenyMutationOperationRule(ent.OpDelete)
createRule := privacy.OnMutationOperation(
	privacy.AlwaysAllowRule(),
	ent.OpCreate,
)
```

### Pattern 3: Context-Based Authorization

```go
isAdmin := privacy.ContextQueryMutationRule(func(ctx context.Context) error {
	role, ok := ctx.Value("role").(string)
	if !ok || role != "admin" {
		return privacy.Deny
	}
	return privacy.Skip
})
```

### Pattern 4: Multi-Rule Policies

```go
policy := privacy.MutationPolicy{
	privacy.AlwaysAllowRule(),
	privacy.DenyMutationOperationRule(ent.OpDelete),
	myCustomRule,
}
```

---

## Evaluation Logic

**QueryPolicy.EvalQuery** and **MutationPolicy.EvalMutation** follow this logic:

1. Iterate through rules in order
2. Evaluate each rule
3. On error return (non-nil):
   - If `errors.Is(err, Skip)`: continue to next rule
   - Otherwise: return error immediately
4. If all rules skip (return nil or Skip), allow the operation (return nil)

**Policies** (composite) uses similar logic but with policy evaluation instead.

---

## Testing Strategy

**Unit tests** (assumed in privacy_test.go):
- Test each rule type independently
- Test policy composition (multiple rules)
- Test context integration
- Verify decision propagation (Allow → nil, Deny → error)

**Integration tests** (in examples):
- Test with generated ent clients
- Verify policies are evaluated on query/mutation
- Test context passing through client operations

**Run tests**:
```bash
go test ./privacy/...
```

---

## Common Pitfalls

### Pitfall 1: Confusing Allow and Nil

**Wrong**: Returning `Allow` error; treating it like a normal error
**Right**: `Allow` is a sentinel error; `errors.Is(err, Allow)` converts to nil outcome

### Pitfall 2: Forgetting Skip in Custom Rules

**Wrong**: Returning nil when uncertain, blocking next rule evaluation
**Right**: Return `Skip` to defer to next rule; return nil only to allow

### Pitfall 3: Not Checking Context Properly

**Wrong**: Assuming context keys exist without checking
**Right**: Check `ok` in type assertion: `userID, ok := ctx.Value("user_id").(int)`

### Pitfall 4: Order-Dependent Rules

**Wrong**: Rules that depend on evaluation order without documentation
**Right**: Clearly document rule order expectations; use single rules for order-critical logic

### Pitfall 5: Missing Error Messages

**Wrong**: Returning `Deny` without context
**Right**: Use `Denyf()` with descriptive message: `privacy.Denyf("user %d not authorized", userID)`

---

## Verification Checklist

Before marking work complete:

- [ ] Code compiles: `go build ./...`
- [ ] Tests pass: `go test ./privacy/...`
- [ ] All rule types implement correct interfaces (QueryRule, MutationRule, or both)
- [ ] Decisions use sentinel errors (Allow, Deny, Skip)
- [ ] Error messages are descriptive when using `*f` helpers
- [ ] Context handling includes `ok` checks for type assertions
- [ ] Rules document their behavior and assumptions
- [ ] Evaluation order is clear and documented

---

## Useful Commands

```bash
# Run tests
go test ./privacy/...

# Run tests with coverage
go test -cover ./privacy/...

# Check for lint issues
golangci-lint run ./privacy/...

# Run with race detector
go test -race ./privacy/...
```

---

## Key Files Reference

| File | Purpose | When to Touch |
|------|---------|---------------|
| `privacy.go` | Core interfaces, policies, and evaluation | Adding new rule types or policy composition logic |

---

## Questions? Check These Resources

1. **Rule interface examples**: See helper functions in `privacy.go` (lines 47-113)
2. **Policy evaluation**: See `Policies.eval()` (lines 168-182)
3. **Context integration**: See `DecisionContext()` and `DecisionFromContext()` (lines 208-226)
4. **Example usage**: See examples in repo (e.g., `examples/privacyadmin/`)

---

**Last Updated**: 2026-02-08
