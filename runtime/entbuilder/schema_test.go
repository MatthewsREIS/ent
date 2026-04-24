package entbuilder

import (
	"reflect"
	"testing"
	"time"
)

func TestField_Basic(t *testing.T) {
	f := Field{
		Name:     "name",
		Column:   "name",
		Type:     reflect.TypeOf(""),
		Nullable: false,
	}
	if f.Name != "name" {
		t.Fatalf("Name mismatch: %q", f.Name)
	}
	if f.Type != reflect.TypeOf("") {
		t.Fatalf("Type mismatch: %v", f.Type)
	}
	if f.Nullable {
		t.Fatal("expected Nullable=false")
	}
}

func TestField_NullableTime(t *testing.T) {
	f := Field{
		Name:     "expired_at",
		Column:   "expired_at",
		Type:     reflect.TypeOf(time.Time{}),
		Nullable: true,
	}
	if f.Type != reflect.TypeOf(time.Time{}) {
		t.Fatalf("Type mismatch: %v", f.Type)
	}
	if !f.Nullable {
		t.Fatal("expected Nullable=true")
	}
}

func TestEdge_UniqueInverse(t *testing.T) {
	e := Edge{
		Name:     "owner",
		Unique:   true,
		Inverse:  true,
		Field:    "owner_id",
		TargetID: reflect.TypeOf(int(0)),
	}
	if !e.Unique || !e.Inverse {
		t.Fatalf("expected Unique && Inverse; got %+v", e)
	}
	if e.Field != "owner_id" {
		t.Fatalf("Field mismatch: %q", e.Field)
	}
}
