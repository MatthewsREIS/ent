package entbuilder

import (
	"testing"

	"entgo.io/ent/schema/field"
)

// fakeMut is a stand-in for a generated *XxxMutation.
// Tests use it to exercise SimpleField without pulling in real generated code.
type fakeMut struct {
	intVal *int
}

func (m fakeMut) Age() (int, bool) {
	if m.intVal == nil {
		return 0, false
	}
	return *m.intVal, true
}

// fakeNode is a stand-in for a generated *Xxx entity.
type fakeNode struct {
	Age int
}

type fakeCfg struct{}

func TestSimpleField_Int_Value_Set(t *testing.T) {
	n := 42
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)

	fv, ok, err := desc.Value(fakeMut{intVal: &n})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true when value is set")
	}
	if fv.Spec != 42 || fv.Node != 42 {
		t.Fatalf("unexpected FieldValue: %+v", fv)
	}
}

func TestSimpleField_Int_Value_Unset(t *testing.T) {
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)

	fv, ok, err := desc.Value(fakeMut{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when value is unset")
	}
	if fv != (FieldValue{}) {
		t.Fatalf("expected zero FieldValue, got: %+v", fv)
	}
}

func TestSimpleField_Assign_Int(t *testing.T) {
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)

	var node fakeNode
	if err := desc.Assign(&node, FieldValue{Node: 99}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if node.Age != 99 {
		t.Fatalf("expected node.Age=99, got %d", node.Age)
	}
}

func TestSimpleField_Assign_WrongType_Panics(t *testing.T) {
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on type assertion mismatch")
		}
	}()
	var node fakeNode
	_ = desc.Assign(&node, FieldValue{Node: "not-an-int"})
}

type fakeMutNillable struct {
	strVal *string // set means value present; mutation reports (val, true)
}

func (m fakeMutNillable) Nickname() (string, bool) {
	if m.strVal == nil {
		return "", false
	}
	return *m.strVal, true
}

type fakeNodeNillable struct {
	Nickname *string
}

func TestNillableField_Value_Set(t *testing.T) {
	s := "alice"
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	fv, ok, err := desc.Value(fakeMutNillable{strVal: &s})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if fv.Spec != "alice" {
		t.Fatalf("unexpected Spec: %v", fv.Spec)
	}
	// Node must be a *string pointing at a copy, not the original.
	ptr, isPtr := fv.Node.(*string)
	if !isPtr {
		t.Fatalf("expected Node to be *string, got %T", fv.Node)
	}
	if ptr == &s {
		t.Fatal("Node must be a COPY, not alias of the caller's value")
	}
	if *ptr != "alice" {
		t.Fatalf("unexpected *Node: %v", *ptr)
	}
}

func TestNillableField_Value_Unset(t *testing.T) {
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	fv, ok, err := desc.Value(fakeMutNillable{})
	if err != nil || ok || fv != (FieldValue{}) {
		t.Fatalf("expected zero result, got ok=%v err=%v fv=%+v", ok, err, fv)
	}
}

func TestNillableField_Assign_Pointer(t *testing.T) {
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	s := "bob"
	var node fakeNodeNillable
	if err := desc.Assign(&node, FieldValue{Node: &s}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if node.Nickname == nil || *node.Nickname != "bob" {
		t.Fatalf("expected *node.Nickname=bob, got %v", node.Nickname)
	}
	// Pass-through is intentional: Assign does not copy the pointee.
	// Callsites pass freshly-allocated pointers from a row scan; copying again
	// would be redundant. Contrast with Value, which DOES copy (see
	// TestNillableField_Value_Set's ptr == &s check).
	if node.Nickname != &s {
		t.Fatalf("expected node.Nickname to alias caller pointer (pass-through contract); got %p vs %p", node.Nickname, &s)
	}
}

func TestNillableField_Assign_WrongType_NoOp(t *testing.T) {
	// Existing helper silently skips on wrong type (type assertion returns ok=false).
	// Test documents this behavior so a future refactor doesn't accidentally panic.
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	var node fakeNodeNillable
	if err := desc.Assign(&node, FieldValue{Node: "not-a-pointer"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if node.Nickname != nil {
		t.Fatal("expected node.Nickname unchanged when type assertion fails")
	}
}
