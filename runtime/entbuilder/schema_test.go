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

// fakeCardNode is a test stand-in for an entity struct.
type fakeCardNode struct {
	ID        int
	Number    string
	Name      string
	CreatedAt time.Time
	InHook    string
	ExpiredAt *time.Time
}

func TestSchema_FindField(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name:  "cards",
		Table: "cards",
		IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
		Fields: []Field{
			{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
			{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
		},
	}
	f, ok := s.FindField("Number")
	if !ok {
		t.Fatal("expected to find Number")
	}
	if f.Column != "number" {
		t.Fatalf("wrong field: %+v", f)
	}
	if _, ok := s.FindField("Missing"); ok {
		t.Fatal("expected FindField to return ok=false for unknown field")
	}
}

func TestSchema_FindEdge(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "cards",
		Edges: []Edge{
			{Name: "Owner", Unique: true, Inverse: true, Field: "owner_id"},
		},
	}
	e, ok := s.FindEdge("Owner")
	if !ok || e.Field != "owner_id" {
		t.Fatalf("FindEdge failed: ok=%v edge=%+v", ok, e)
	}
	if _, ok := s.FindEdge("Missing"); ok {
		t.Fatal("expected FindEdge to return ok=false for unknown edge")
	}
}

func TestValidateSchema_Matches(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name:    "Card",
		Table:   "cards",
		IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
		Fields: []Field{
			{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
			{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
			{Name: "CreatedAt", Column: "created_at", Type: reflect.TypeOf(time.Time{})},
			{Name: "InHook", Column: "in_hook", Type: reflect.TypeOf("")},
			{Name: "ExpiredAt", Column: "expired_at", Type: reflect.TypeOf(time.Time{}), Nullable: true},
		},
	}
	if err := ValidateSchema[fakeCardNode](s); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestValidateSchema_MissingField(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{
			{Name: "NotOnStruct", Column: "x", Type: reflect.TypeOf("")},
		},
	}
	err := ValidateSchema[fakeCardNode](s)
	if err == nil {
		t.Fatal("expected error when descriptor names a field absent on the struct")
	}
	if !containsAll(err.Error(), "NotOnStruct", "fakeCardNode") {
		t.Fatalf("error message should name the bad field and type: %v", err)
	}
}

func TestValidateSchema_WrongType(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{
			// Number is string on the struct; descriptor claims int
			{Name: "Number", Column: "number", Type: reflect.TypeOf(int(0))},
		},
	}
	err := ValidateSchema[fakeCardNode](s)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestValidateSchema_NullableMatchesPointer(t *testing.T) {
	// ExpiredAt on fakeCardNode is *time.Time. The descriptor should
	// declare Type=time.Time and Nullable=true; the validator peels
	// the pointer on the struct side.
	s := &Schema[fakeCardNode]{
		Name:    "Card",
		IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
		Fields: []Field{
			{Name: "ExpiredAt", Column: "expired_at", Type: reflect.TypeOf(time.Time{}), Nullable: true},
			{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
			{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
			{Name: "CreatedAt", Column: "created_at", Type: reflect.TypeOf(time.Time{})},
			{Name: "InHook", Column: "in_hook", Type: reflect.TypeOf("")},
		},
	}
	if err := ValidateSchema[fakeCardNode](s); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// test helper
func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
