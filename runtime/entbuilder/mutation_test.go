package entbuilder

import (
	"reflect"
	"testing"
	"time"
)

// Reuses fakeCardNode from schema_test.go.

func TestNewMutation_InitializesEmpty(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "Card"}
	m := NewMutation[fakeCardNode](s, OpCreate)
	if m.Op() != OpCreate {
		t.Fatalf("Op mismatch: %v", m.Op())
	}
	if len(m.Fields()) != 0 {
		t.Fatalf("expected empty Fields(): %v", m.Fields())
	}
	if len(m.AddedFields()) != 0 {
		t.Fatalf("expected empty AddedFields(): %v", m.AddedFields())
	}
	if len(m.ClearedFields()) != 0 {
		t.Fatalf("expected empty ClearedFields(): %v", m.ClearedFields())
	}
}

var cardTestSchema = &Schema[fakeCardNode]{
	Name:  "Card",
	Table: "cards",
	IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
	Fields: []Field{
		{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
		{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
		{Name: "ExpiredAt", Column: "expired_at", Type: reflect.TypeOf(time.Time{}), Nullable: true},
	},
}

func TestSetField_StoresAndReads(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if err := m.SetField("Number", "1234"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	v, ok := m.Field("Number")
	if !ok || v.(string) != "1234" {
		t.Fatalf("Field: ok=%v v=%v", ok, v)
	}
	names := m.Fields()
	if len(names) != 1 || names[0] != "Number" {
		t.Fatalf("Fields: %v", names)
	}
}

func TestField_Unset(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if _, ok := m.Field("Number"); ok {
		t.Fatal("expected ok=false for unset field")
	}
}

func TestSetField_UnknownField_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	err := m.SetField("NotAField", "v")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestClearField_RecordsAndClearsSetValue(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	_ = m.SetField("Name", "alice")
	if err := m.ClearField("Name"); err != nil {
		t.Fatalf("ClearField: %v", err)
	}
	if _, ok := m.Field("Name"); ok {
		t.Fatal("expected Field(Name) ok=false after ClearField")
	}
	if !m.FieldCleared("Name") {
		t.Fatal("expected FieldCleared(Name)=true")
	}
	cleared := m.ClearedFields()
	if len(cleared) != 1 || cleared[0] != "Name" {
		t.Fatalf("ClearedFields: %v", cleared)
	}
}

func TestClearField_NonNullableField_ReturnsError(t *testing.T) {
	// Number is not Optional/Nullable — cannot be cleared.
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	err := m.ClearField("Number")
	if err == nil {
		t.Fatal("expected error clearing a non-nullable field")
	}
}

func TestFieldCleared_UnknownField_ReturnsFalse(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if m.FieldCleared("NotAField") {
		t.Fatal("expected false for unknown field")
	}
}
