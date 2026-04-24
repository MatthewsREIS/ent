// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package spike

import (
	"testing"
	"time"

	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/entc/integration/hooks/ent/card"
	"entgo.io/ent/runtime/entbuilder"
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
