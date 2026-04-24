package entbuilder

import (
	"testing"

	"entgo.io/ent/dialect/sql"
)

// fakePred is a stand-in for a generated predicate.Xxx type.
// Generated predicates are declared as `type Xxx func(*sql.Selector)`.
type fakePred func(*sql.Selector)

func TestFieldEQ_WrapsSQLFieldEQ(t *testing.T) {
	p := FieldEQ[fakePred]("name", "alice")
	if p == nil {
		t.Fatal("nil predicate")
	}
	// Apply to a fresh selector and confirm it produces the same SQL
	// fragment as sql.FieldEQ directly.
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldEQ("name", "alice")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("FieldEQ output differs from sql.FieldEQ:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldEQ_InstantiatedForMultiplePredicateTypes(t *testing.T) {
	// Ensures the generic is instantiable for distinct named predicate types
	// (compile-only assertion; no runtime behavior).
	type predA func(*sql.Selector)
	type predB func(*sql.Selector)
	_ = FieldEQ[predA]("a", 1)
	_ = FieldEQ[predB]("b", 2)
}
