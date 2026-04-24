package entbuilder

import (
	"context"
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

// For numeric deltas we need a schema whose struct has numeric fields.
type fakeCardWithBalance struct {
	Balance float64
	Version int
}

var cardBalanceSchema = &Schema[fakeCardWithBalance]{
	Name: "Card",
	Fields: []Field{
		{Name: "Balance", Column: "balance", Type: reflect.TypeOf(float64(0))},
		{Name: "Version", Column: "version", Type: reflect.TypeOf(int(0))},
	},
}

func TestAddField_AccumulatesNumericDelta(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	if err := m.AddField("Balance", 3.14); err != nil {
		t.Fatalf("AddField: %v", err)
	}
	v, ok := m.AddedField("Balance")
	if !ok {
		t.Fatal("expected AddedField(Balance) ok=true")
	}
	if v.(float64) != 3.14 {
		t.Fatalf("AddedField delta: got %v", v)
	}
	added := m.AddedFields()
	if len(added) != 1 || added[0] != "Balance" {
		t.Fatalf("AddedFields: %v", added)
	}
}

func TestAddField_UnknownField_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	if err := m.AddField("NotAField", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddedField_UnsetReturnsFalse(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	if _, ok := m.AddedField("Balance"); ok {
		t.Fatal("expected ok=false before any AddField")
	}
}

func TestResetField_ClearsAllState(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	_ = m.SetField("Name", "alice")
	_ = m.ClearField("Name")
	// Re-set for good measure — ClearField removed setFields entry.
	_ = m.SetField("Name", "bob")

	if err := m.ResetField("Name"); err != nil {
		t.Fatalf("ResetField: %v", err)
	}

	if _, ok := m.Field("Name"); ok {
		t.Fatal("expected Field(Name) unset after ResetField")
	}
	if m.FieldCleared("Name") {
		t.Fatal("expected FieldCleared(Name)=false after ResetField")
	}
	if len(m.Fields()) != 0 {
		t.Fatalf("Fields: %v", m.Fields())
	}
	if len(m.ClearedFields()) != 0 {
		t.Fatalf("ClearedFields: %v", m.ClearedFields())
	}
}

func TestResetField_NumericAlsoClearsAdded(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	_ = m.AddField("Balance", 3.14)
	_ = m.SetField("Balance", 100.0)
	if err := m.ResetField("Balance"); err != nil {
		t.Fatalf("ResetField: %v", err)
	}
	if _, ok := m.AddedField("Balance"); ok {
		t.Fatal("expected AddedField(Balance) ok=false after ResetField")
	}
	if _, ok := m.Field("Balance"); ok {
		t.Fatal("expected Field(Balance) ok=false after ResetField")
	}
}

func TestResetField_UnknownField_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if err := m.ResetField("NotAField"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetID_UpdateOne(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdateOne)
	m.SetID(42)
	id, ok := m.ID()
	if !ok || id.(int) != 42 {
		t.Fatalf("ID: ok=%v id=%v", ok, id)
	}
}

func TestID_Unset(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpCreate)
	if _, ok := m.ID(); ok {
		t.Fatal("expected ok=false before SetID")
	}
}

var cardEdgeSchema = &Schema[fakeCardNode]{
	Name: "Card",
	Fields: []Field{
		{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
	},
	Edges: []Edge{
		{Name: "Owner", Unique: true, Inverse: true, Field: "owner_id", TargetID: reflect.TypeOf(int(0))},
	},
}

func TestSetEdgeID_StoresAndReadsUniqueEdge(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	if err := m.SetEdgeID("Owner", 7); err != nil {
		t.Fatalf("SetEdgeID: %v", err)
	}
	id, ok := m.EdgeID("Owner")
	if !ok || id.(int) != 7 {
		t.Fatalf("EdgeID: ok=%v id=%v", ok, id)
	}
}

func TestSetEdgeID_UnknownEdge_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	if err := m.SetEdgeID("NotAnEdge", 1); err == nil {
		t.Fatal("expected error for unknown edge")
	}
}

func TestEdgeID_Unset(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	if _, ok := m.EdgeID("Owner"); ok {
		t.Fatal("expected ok=false before SetEdgeID")
	}
}

func TestClearEdge_UniqueEdge(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	_ = m.SetEdgeID("Owner", 7)
	if err := m.ClearEdge("Owner"); err != nil {
		t.Fatalf("ClearEdge: %v", err)
	}
	if _, ok := m.EdgeID("Owner"); ok {
		t.Fatal("expected EdgeID ok=false after ClearEdge")
	}
	if !m.EdgeCleared("Owner") {
		t.Fatal("expected EdgeCleared=true")
	}
}

func TestClearEdge_UnknownEdge_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	if err := m.ClearEdge("NotAnEdge"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEdgeCleared_UnknownEdge_ReturnsFalse(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	if m.EdgeCleared("NotAnEdge") {
		t.Fatal("expected false for unknown edge")
	}
}

func TestAddedEdges_ReflectsSet(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	_ = m.SetEdgeID("Owner", 7)
	edges := m.AddedEdges()
	if len(edges) != 1 || edges[0] != "Owner" {
		t.Fatalf("AddedEdges: %v", edges)
	}
}

func TestClearedEdges_ReflectsCleared(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	_ = m.ClearEdge("Owner")
	edges := m.ClearedEdges()
	if len(edges) != 1 || edges[0] != "Owner" {
		t.Fatalf("ClearedEdges: %v", edges)
	}
}

func TestRemovedEdges_EmptyForSpike(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	if edges := m.RemovedEdges(); len(edges) != 0 {
		t.Fatalf("RemovedEdges should be empty in spike: %v", edges)
	}
}

func TestOldField_DelegatesToSchemaFetcher(t *testing.T) {
	called := false
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{{Name: "Name", Column: "name", Type: reflect.TypeOf("")}},
		OldFieldFetcher: func(ctx context.Context, id any, field string) (any, error) {
			called = true
			if field != "Name" {
				t.Errorf("unexpected field: %s", field)
			}
			if id.(int) != 42 {
				t.Errorf("unexpected id: %v", id)
			}
			return "alice", nil
		},
	}
	m := NewMutation[fakeCardNode](s, OpUpdateOne)
	m.SetID(42)
	v, err := m.OldField(context.Background(), "Name")
	if err != nil {
		t.Fatalf("OldField: %v", err)
	}
	if v.(string) != "alice" {
		t.Fatalf("OldField value: %v", v)
	}
	if !called {
		t.Fatal("expected fetcher to be invoked")
	}
}

func TestOldField_NoFetcher_ReturnsError(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{{Name: "Name", Column: "name", Type: reflect.TypeOf("")}},
	}
	m := NewMutation[fakeCardNode](s, OpUpdateOne)
	m.SetID(1)
	_, err := m.OldField(context.Background(), "Name")
	if err == nil {
		t.Fatal("expected error when schema has no OldFieldFetcher")
	}
}

func TestOldField_NoID_ReturnsError(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{{Name: "Name", Column: "name", Type: reflect.TypeOf("")}},
		OldFieldFetcher: func(ctx context.Context, id any, field string) (any, error) {
			return nil, nil
		},
	}
	m := NewMutation[fakeCardNode](s, OpUpdate) // bulk update — no ID
	_, err := m.OldField(context.Background(), "Name")
	if err == nil {
		t.Fatal("expected error when mutation has no ID set")
	}
}

func TestOldField_UnknownField_ReturnsError(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		OldFieldFetcher: func(ctx context.Context, id any, field string) (any, error) {
			return nil, nil
		},
	}
	m := NewMutation[fakeCardNode](s, OpUpdateOne)
	m.SetID(1)
	_, err := m.OldField(context.Background(), "NotAField")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}
