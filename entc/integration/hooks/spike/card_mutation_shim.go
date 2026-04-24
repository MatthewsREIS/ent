// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package spike

import (
	"context"
	"time"

	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/runtime/entbuilder"
)

// CardMutationShim wraps the generic Mutation[ent.Card] and exposes the
// typed API that hooks_test.go expects. Method bodies delegate to
// entbuilder.Mutation[ent.Card].
type CardMutationShim struct {
	m *entbuilder.Mutation[ent.Card]
}

// NewCardMutationShim constructs an empty shim for the given op.
func NewCardMutationShim(op entbuilder.Op) *CardMutationShim {
	return &CardMutationShim{m: entbuilder.NewMutation[ent.Card](cardSchema, op)}
}

// ----- Typed field accessors -----

func (s *CardMutationShim) SetNumber(v string) { _ = s.m.SetField("Number", v) }
func (s *CardMutationShim) Number() (string, bool) {
	v, ok := s.m.Field("Number")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (s *CardMutationShim) OldNumber(ctx context.Context) (string, error) {
	v, err := s.m.OldField(ctx, "Number")
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (s *CardMutationShim) SetName(v string)  { _ = s.m.SetField("Name", v) }
func (s *CardMutationShim) ClearName()        { _ = s.m.ClearField("Name") }
func (s *CardMutationShim) ResetName()        { _ = s.m.ResetField("Name") }
func (s *CardMutationShim) NameCleared() bool { return s.m.FieldCleared("Name") }
func (s *CardMutationShim) Name() (string, bool) {
	v, ok := s.m.Field("Name")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (s *CardMutationShim) OldName(ctx context.Context) (string, error) {
	v, err := s.m.OldField(ctx, "Name")
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return v.(string), nil
}

func (s *CardMutationShim) SetCreatedAt(v time.Time) { _ = s.m.SetField("CreatedAt", v) }
func (s *CardMutationShim) ResetCreatedAt()          { _ = s.m.ResetField("CreatedAt") }
func (s *CardMutationShim) CreatedAt() (time.Time, bool) {
	v, ok := s.m.Field("CreatedAt")
	if !ok {
		return time.Time{}, false
	}
	return v.(time.Time), true
}
func (s *CardMutationShim) OldCreatedAt(ctx context.Context) (time.Time, error) {
	v, err := s.m.OldField(ctx, "CreatedAt")
	if err != nil {
		return time.Time{}, err
	}
	return v.(time.Time), nil
}

func (s *CardMutationShim) SetInHook(v string) { _ = s.m.SetField("InHook", v) }
func (s *CardMutationShim) InHook() (string, bool) {
	v, ok := s.m.Field("InHook")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (s *CardMutationShim) OldInHook(ctx context.Context) (string, error) {
	v, err := s.m.OldField(ctx, "InHook")
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (s *CardMutationShim) SetExpiredAt(v time.Time) { _ = s.m.SetField("ExpiredAt", v) }
func (s *CardMutationShim) ClearExpiredAt()          { _ = s.m.ClearField("ExpiredAt") }
func (s *CardMutationShim) ExpiredAtCleared() bool   { return s.m.FieldCleared("ExpiredAt") }
func (s *CardMutationShim) ExpiredAt() (time.Time, bool) {
	v, ok := s.m.Field("ExpiredAt")
	if !ok {
		return time.Time{}, false
	}
	return v.(time.Time), true
}
func (s *CardMutationShim) OldExpiredAt(ctx context.Context) (time.Time, error) {
	v, err := s.m.OldField(ctx, "ExpiredAt")
	if err != nil {
		return time.Time{}, err
	}
	return v.(time.Time), nil
}

// ----- Owner edge -----

func (s *CardMutationShim) SetOwnerID(id int)  { _ = s.m.SetEdgeID("Owner", id) }
func (s *CardMutationShim) ClearOwner()        { _ = s.m.ClearEdge("Owner") }
func (s *CardMutationShim) OwnerCleared() bool { return s.m.EdgeCleared("Owner") }
func (s *CardMutationShim) OwnerID() (int, bool) {
	v, ok := s.m.EdgeID("Owner")
	if !ok {
		return 0, false
	}
	return v.(int), true
}
func (s *CardMutationShim) OwnerIDs() []int {
	v, ok := s.m.EdgeID("Owner")
	if !ok {
		return nil
	}
	return []int{v.(int)}
}

// ----- Introspection -----

func (s *CardMutationShim) Op() entbuilder.Op { return s.m.Op() }
func (s *CardMutationShim) ID() (int, bool) {
	v, ok := s.m.ID()
	if !ok {
		return 0, false
	}
	return v.(int), true
}
func (s *CardMutationShim) SetID(id int) { s.m.SetID(id) }

func (s *CardMutationShim) Fields() []string        { return s.m.Fields() }
func (s *CardMutationShim) AddedFields() []string   { return s.m.AddedFields() }
func (s *CardMutationShim) ClearedFields() []string { return s.m.ClearedFields() }
func (s *CardMutationShim) AddedEdges() []string    { return s.m.AddedEdges() }
func (s *CardMutationShim) ClearedEdges() []string  { return s.m.ClearedEdges() }
func (s *CardMutationShim) RemovedEdges() []string  { return s.m.RemovedEdges() }

func (s *CardMutationShim) Field(name string) (any, bool)      { return s.m.Field(name) }
func (s *CardMutationShim) AddedField(name string) (any, bool) { return s.m.AddedField(name) }
func (s *CardMutationShim) FieldCleared(name string) bool      { return s.m.FieldCleared(name) }
func (s *CardMutationShim) EdgeCleared(name string) bool       { return s.m.EdgeCleared(name) }
func (s *CardMutationShim) OldField(ctx context.Context, name string) (any, error) {
	return s.m.OldField(ctx, name)
}
