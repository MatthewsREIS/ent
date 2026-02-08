<!-- Generated: 2026-02-08 -->
<!-- Parent: ../AGENTS.md -->

# AGENTS.md — entql

This document provides guidance for AI agents working in the **entql** package.

**Package**: `entgo.io/ent/entql`
**Purpose**: Type-safe predicate language and expression evaluation for dynamic queries
**Go Version**: 1.24+

---

## Overview

The `entql` package provides an experimental, expression-based API for building type-safe query predicates dynamically. It enables developers to construct queries programmatically without relying on generated query builders, useful for:

- Dynamic filtering based on runtime conditions
- Type-safe predicate composition
- Generic query construction systems
- Integration with external query systems

**Key Files**:
- `entql.go`: Core expression types, operators, and predicate builder functions
- `types.go`: Generated type-specific predicate interfaces (BoolP, IntP, StringP, etc.)
- `entql_test.go`, `types_test.go`: Unit tests

---

## Core Concepts

### Operators (Op type)

Located in `entql.go`, the `Op` type represents 11 predicate operators:

```
OpAnd, OpOr, OpNot                  // Logical operators
OpEQ, OpNEQ, OpGT, OpGTE, OpLT, OpLTE  // Comparison operators
OpIn, OpNotIn                       // Membership operators
```

Each operator has a string representation (via `Op.String()`).

### Expression Types

All expressions implement the `Expr` interface. Core types:

- **BinaryExpr**: Two operands (e.g., `x > y`, `field == value`)
- **UnaryExpr**: One operand (e.g., `!x`)
- **NaryExpr**: N operands (e.g., `x && y && z`)
- **CallExpr**: Function call (e.g., `contains(field, "substr")`)
- **Field**: References a schema field by name
- **Edge**: References a relationship in the graph
- **Value**: Wraps arbitrary Go values for comparison

### Predicates (P interface)

The `P` interface extends `Expr` and adds a `Negate()` method. Most builder functions return predicates.

### Type-Specific Predicates

Generated in `types.go` (via `internal/gen.go`), type-specific predicate interfaces provide compile-time type safety:

- `BoolP`, `BytesP`, `TimeP`, `UintP`, `IntP`, `Float32P`, `Float64P`, `StringP`, `ValueP`, `OtherP`

Each type-specific interface includes:

- Standard operators: `EQ`, `NEQ`, `LT`, `LTE`, `GT`, `GTE`
- Composition: `And`, `Or`, `Not`
- Nil checks: `Nil`, `NotNil`

Example usage:

```go
predicate := entql.StringEQ("name").Field("name")  // type-safe
```

---

## Common Tasks

### Adding a New Operator

1. **Define Op constant** in `entql.go` (`const` block, lines 19-31)
2. **Add string representation** in `ops` array (lines 33-45)
3. **Create builder function** (e.g., `func EQ(x, y Expr) P`) if public-facing
4. **Add tests** in `entql_test.go`

### Adding a New Type-Specific Predicate Interface

1. **Edit `internal/gen.go`** to include the new type
2. **Run `go generate ./...`** in the `entql` directory to regenerate `types.go`
3. **Test** by calling the generated functions (e.g., `IntEQ(42)`)

### Writing Tests

Tests use table-driven format. Example from `entql_test.go`:

```go
func TestFieldEQ(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    any
		expected string
	}{
		{
			name:     "string field",
			field:    "name",
			value:    "Alice",
			expected: "name == \"Alice\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := entql.FieldEQ(tt.field, tt.value)
			if p.String() != tt.expected {
				t.Errorf("got %q, want %q", p.String(), tt.expected)
			}
		})
	}
}
```

---

## Code Organization

### entql.go

**Lines 1-80**: Core operators and functions
- `Op` type and constants
- `Func` type for builtin functions (FuncEqualFold, FuncContains, etc.)
- `Expr` and `P` interfaces

**Lines 81-465**: Expression type definitions and builder functions
- Struct types: `UnaryExpr`, `BinaryExpr`, `NaryExpr`, `CallExpr`, `Field`, `Edge`, `Value`
- Builder functions: `Not()`, `And()`, `Or()`, `F()`, `EQ()`, `FieldEQ()`, etc.
- String methods for all expression types
- Helper: `p2expr()` for converting `[]P` to `[]Expr`

### types.go

**Lines 1-50**: Interfaces and generation directive
- `Fielder` interface (base for all type-specific predicates)
- Individual type-specific `*P` interfaces (e.g., `BoolP`, `StringP`)
- `//go:generate` directive to regenerate this file

**Lines 51-1981**: Type-specific implementations
- For each type: predicate functions (`TypeNil()`, `TypeEQ()`, etc.) and composition operators

---

## Key Patterns

### Pattern 1: Building Predicates Dynamically

```go
// Construct a predicate at runtime based on user input
fieldName := "age"
operator := ">"
value := 18

var pred entql.P
switch operator {
case ">":
	pred = entql.FieldGT(fieldName, value)
case "==":
	pred = entql.FieldEQ(fieldName, value)
}

// Use pred in query
```

### Pattern 2: Composing Predicates

```go
age := entql.FieldGT("age", 18)
active := entql.FieldEQ("status", "active")
combined := entql.And(age, active)
```

### Pattern 3: Type-Safe Predicates

```go
// Generated type-specific function
pred := entql.StringEQ("name").Field("name")  // StringP implements Fielder
```

---

## Testing Strategy

**Unit tests** (`entql_test.go`, `types_test.go`):
- Test each builder function (FieldEQ, FieldGT, And, Or, etc.)
- Verify string output matches expected predicate format
- Test edge cases: nil values, empty lists, negation

**Run tests**:
```bash
go test ./entql/...
```

---

## Common Pitfalls

### Pitfall 1: Manually Editing types.go

**Wrong**: Edit `types.go` directly
**Right**: Modify `internal/gen.go` and run `go generate ./...`

### Pitfall 2: Not Implementing String() Method

**Wrong**: New expression type without `String()` method
**Right**: Add `String()` method to new type (required by `Expr` interface)

### Pitfall 3: Forgetting to Negate() Predicates

**Wrong**: Using `Not()` when `Negate()` should be used on `P` types
**Right**: Call `pred.Negate()` on predicates implementing the `P` interface

### Pitfall 4: Type Safety Misuse

**Wrong**: Mixing types (e.g., `entql.StringEQ(42)`)
**Right**: Use correct type-specific function (`entql.IntEQ(42)`)

---

## Verification Checklist

Before marking work complete:

- [ ] Code compiles: `go build ./...`
- [ ] Tests pass: `go test ./entql/...`
- [ ] `types.go` regenerated if needed: `go generate ./...`
- [ ] All new expression types have `String()` implementations
- [ ] All new operators have string representations in `ops` array
- [ ] All public functions are documented with comments
- [ ] Expression string output is unambiguous and parseable

---

## Useful Commands

```bash
# Run tests
go test ./entql/...

# Run tests with coverage
go test -cover ./entql/...

# Regenerate types.go
cd entql && go generate ./...

# Check for lint issues
golangci-lint run ./entql/...
```

---

## Key Files Reference

| File | Purpose | When to Touch |
|------|---------|---------------|
| `entql.go` | Core operators and expressions | Adding operators, functions, or expression types |
| `types.go` | Generated type-specific predicates | Never directly; edit `internal/gen.go` instead |
| `internal/gen.go` | Generator for `types.go` | Adding new types or modifying code generation |
| `entql_test.go` | Tests for core operators | Adding tests for new operators or functions |
| `types_test.go` | Tests for type-specific predicates | Adding tests for new generated predicates |

---

## Questions? Check These Resources

1. **Expression construction**: See `entql.go` function examples (lines 161-365)
2. **Type-safe predicates**: See generated type functions in `types.go`
3. **Test patterns**: See `*_test.go` files
4. **Code generation**: See `internal/gen.go`

---

**Last Updated**: 2026-02-08
