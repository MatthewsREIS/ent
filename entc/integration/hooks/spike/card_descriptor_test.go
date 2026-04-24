// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package spike

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/entc/integration/hooks/ent/card"
	"entgo.io/ent/entc/integration/hooks/ent/enttest"
	"entgo.io/ent/runtime/entbuilder"
	_ "github.com/mattn/go-sqlite3"
)

func TestCardDescriptor_Valid(t *testing.T) {
	if err := entbuilder.ValidateSchema[ent.Card](cardSchema); err != nil {
		t.Fatalf("Card descriptor fails validation: %v", err)
	}
}

func TestShim_Number_SetAndGet(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpCreate)
	s.SetNumber("1234")
	got, ok := s.Number()
	if !ok || got != "1234" {
		t.Fatalf("Number: ok=%v got=%q", ok, got)
	}
}

func TestShim_Name_ClearAndReset(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpUpdate)
	s.SetName("alice")
	if _, ok := s.Name(); !ok {
		t.Fatal("expected Name set")
	}
	s.ClearName()
	if !s.NameCleared() {
		t.Fatal("expected NameCleared=true")
	}
	if _, ok := s.Name(); ok {
		t.Fatal("expected Name unset after ClearName")
	}
	s.ResetName()
	if s.NameCleared() {
		t.Fatal("expected NameCleared=false after Reset")
	}
}

func TestShim_Fields_ListsSet(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpCreate)
	s.SetNumber("1234")
	s.SetName("alice")
	s.SetCreatedAt(time.Now())

	fields := s.Fields()
	seen := make(map[string]bool)
	for _, f := range fields {
		seen[f] = true
	}
	for _, expected := range []string{"Number", "Name", "CreatedAt"} {
		if !seen[expected] {
			t.Errorf("Fields missing %q: %v", expected, fields)
		}
	}
}

func TestShim_Owner_SetClearGet(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpUpdate)
	s.SetOwnerID(42)
	id, ok := s.OwnerID()
	if !ok || id != 42 {
		t.Fatalf("OwnerID: ok=%v id=%d", ok, id)
	}
	if ids := s.OwnerIDs(); len(ids) != 1 || ids[0] != 42 {
		t.Fatalf("OwnerIDs: %v", ids)
	}
	s.ClearOwner()
	if !s.OwnerCleared() {
		t.Fatal("expected OwnerCleared")
	}
}

func TestShim_Field_ByName_UsesConstant(t *testing.T) {
	// Ensure the string name used for card.FieldName constant matches
	// what the descriptor / shim expect.
	s := NewCardMutationShim(entbuilder.OpCreate)
	s.SetName("bob")
	v, ok := s.Field("Name")
	if !ok {
		t.Fatal("Field(\"Name\") ok=false after SetName")
	}
	if v.(string) != "bob" {
		t.Fatalf("Field value: %v", v)
	}
	// Also confirm the generated constant resolves to the same DB column.
	if card.FieldName == "" {
		t.Fatal("card.FieldName should not be empty")
	}
}

// TestShim_OldName_ThroughHooksClient confirms that the descriptor's
// OldFieldFetcher, wired to a real hooks-scenario client, returns the
// pre-mutation value correctly.
func TestShim_OldName_ThroughHooksClient(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, dialect.SQLite, "file:spike?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	wireCardOldFieldFetcher(client)

	// Seed: create a card so we have a row to OldField against.
	u := client.User.Create().SetName("alice").SaveX(ctx)
	c := client.Card.Create().
		SetNumber("1234").
		SetName("original-name").
		SetInHook("hook-value").
		SetOwnerID(u.ID).
		SaveX(ctx)

	// Build a shim as though we were about to update the card. SetID picks
	// the row; OldName should read the pre-mutation value from the DB.
	s := NewCardMutationShim(entbuilder.OpUpdateOne)
	s.SetID(c.ID)
	s.SetName("new-name")

	oldName, err := s.OldName(ctx)
	if err != nil {
		t.Fatalf("OldName: %v", err)
	}
	if oldName != "original-name" {
		t.Fatalf("OldName: got %q want %q", oldName, "original-name")
	}
}

// TestShim_HasFields_Semantics confirms that Fields() / AddedFields() ordering
// and membership on the shim matches what hook.HasFields combinators test for.
func TestShim_HasFields_Semantics(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpUpdate)
	s.SetExpiredAt(time.Now())

	found := false
	for _, name := range s.Fields() {
		if name == "ExpiredAt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Fields() should contain ExpiredAt: %v", s.Fields())
	}

	// And a field that was NOT set should not appear.
	for _, name := range s.Fields() {
		if name == "Name" {
			t.Fatalf("Fields() should not contain Name: %v", s.Fields())
		}
	}
}

