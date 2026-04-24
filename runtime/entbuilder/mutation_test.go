package entbuilder

import (
	"testing"
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
