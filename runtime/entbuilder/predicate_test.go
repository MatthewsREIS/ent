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

func TestFieldNEQ_WrapsSQLFieldNEQ(t *testing.T) {
	p := FieldNEQ[fakePred]("status", "closed")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldNEQ("status", "closed")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldGT_WrapsSQLFieldGT(t *testing.T) {
	p := FieldGT[fakePred]("age", 18)
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldGT("age", 18)(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldGTE_WrapsSQLFieldGTE(t *testing.T) {
	p := FieldGTE[fakePred]("age", 21)
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldGTE("age", 21)(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldLT_WrapsSQLFieldLT(t *testing.T) {
	p := FieldLT[fakePred]("score", 100)
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldLT("score", 100)(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldLTE_WrapsSQLFieldLTE(t *testing.T) {
	p := FieldLTE[fakePred]("score", 99)
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldLTE("score", 99)(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldIn_WrapsSQLFieldIn(t *testing.T) {
	p := FieldIn[fakePred]("status", []any{"active", "pending"})
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldIn[any]("status", "active", "pending")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldNotIn_WrapsSQLFieldNotIn(t *testing.T) {
	p := FieldNotIn[fakePred]("status", []any{"banned", "deleted"})
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldNotIn[any]("status", "banned", "deleted")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldIsNull_WrapsSQLFieldIsNull(t *testing.T) {
	p := FieldIsNull[fakePred]("deleted_at")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldIsNull("deleted_at")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldNotNull_WrapsSQLFieldNotNull(t *testing.T) {
	p := FieldNotNull[fakePred]("verified_at")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldNotNull("verified_at")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldContains_WrapsSQLFieldContains(t *testing.T) {
	p := FieldContains[fakePred]("bio", "engineer")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldContains("bio", "engineer")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldHasPrefix_WrapsSQLFieldHasPrefix(t *testing.T) {
	p := FieldHasPrefix[fakePred]("email", "admin")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldHasPrefix("email", "admin")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldHasSuffix_WrapsSQLFieldHasSuffix(t *testing.T) {
	p := FieldHasSuffix[fakePred]("email", "@example.com")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldHasSuffix("email", "@example.com")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldEqualFold_WrapsSQLFieldEqualFold(t *testing.T) {
	p := FieldEqualFold[fakePred]("username", "Alice")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldEqualFold("username", "Alice")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldContainsFold_WrapsSQLFieldContainsFold(t *testing.T) {
	p := FieldContainsFold[fakePred]("bio", "Engineer")
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldContainsFold("bio", "Engineer")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestAndPreds_WrapsSQLAndPredicates(t *testing.T) {
	p := AndPreds(
		FieldEQ[fakePred]("name", "alice"),
		FieldGT[fakePred]("age", 18),
	)
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.AndPredicates(sql.FieldEQ("name", "alice"), sql.FieldGT("age", 18))(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestOrPreds_WrapsSQLOrPredicates(t *testing.T) {
	p := OrPreds(
		FieldEQ[fakePred]("role", "admin"),
		FieldEQ[fakePred]("role", "moderator"),
	)
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.OrPredicates(sql.FieldEQ("role", "admin"), sql.FieldEQ("role", "moderator"))(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestNotPred_WrapsSQLNotPredicates(t *testing.T) {
	p := NotPred(FieldEQ[fakePred]("name", "alice"))
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.NotPredicates(sql.FieldEQ("name", "alice"))(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}
