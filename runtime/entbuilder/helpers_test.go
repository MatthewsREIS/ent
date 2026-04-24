package entbuilder

import (
	"testing"

	"entgo.io/ent/schema/field"
)

// fakeMut is a stand-in for a generated *XxxMutation.
// Tests use it to exercise SimpleField without pulling in real generated code.
type fakeMut struct {
	intVal *int
	strVal *string
}

func (m fakeMut) Age() (int, bool) {
	if m.intVal == nil {
		return 0, false
	}
	return *m.intVal, true
}

func (m fakeMut) Name() (string, bool) {
	if m.strVal == nil {
		return "", false
	}
	return *m.strVal, true
}

// fakeNode is a stand-in for a generated *Xxx entity.
type fakeNode struct {
	Age  int
	Name string
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
