// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package spike is a throwaway descriptor-driven port of the Card entity,
// used to validate Phase 4A feasibility. See README.md.
package spike

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/entc/integration/hooks/ent/card"
	"entgo.io/ent/runtime/entbuilder"
)

// cardSchema is the descriptor for the Card entity, constructed from the
// same source-of-truth as the generated code: `entc/integration/hooks/ent/schema/card.go`.
//
// Every field here must correspond exactly to a struct field on *ent.Card; this
// is asserted by entbuilder.ValidateSchema on first use.
var cardSchema = &entbuilder.Schema[ent.Card]{
	Name:  "Card",
	Table: card.Table,
	IDField: entbuilder.Field{
		Name:   "ID",
		Column: card.FieldID,
		Type:   reflect.TypeOf(int(0)),
	},
	Fields: []entbuilder.Field{
		{
			Name:      "Number",
			Column:    card.FieldNumber,
			Type:      reflect.TypeOf(""),
			Immutable: true,
		},
		{
			Name:     "Name",
			Column:   card.FieldName,
			Type:     reflect.TypeOf(""),
			Nullable: true,
		},
		{
			Name:   "CreatedAt",
			Column: card.FieldCreatedAt,
			Type:   reflect.TypeOf(time.Time{}),
		},
		{
			Name:   "InHook",
			Column: card.FieldInHook,
			Type:   reflect.TypeOf(""),
		},
		{
			Name:     "ExpiredAt",
			Column:   card.FieldExpiredAt,
			Type:     reflect.TypeOf(time.Time{}),
			Nullable: true,
		},
	},
	Edges: []entbuilder.Edge{
		{
			Name:     "Owner",
			Unique:   true,
			Inverse:  true,
			Field:    "owner_id",
			TargetID: reflect.TypeOf(int(0)),
		},
	},
}

// wireCardOldFieldFetcher wires the OldFieldFetcher closure. The fetcher does
// a one-row select from the cards table by ID. This is called once at
// package init time (or client construction) — the closure captures the
// client for lookups.
func wireCardOldFieldFetcher(client *ent.Client) {
	cardSchema.OldFieldFetcher = func(ctx context.Context, id any, field string) (any, error) {
		intID, ok := id.(int)
		if !ok {
			return nil, fmt.Errorf("spike: card id expected int, got %T", id)
		}
		c, err := client.Card.Get(ctx, intID)
		if err != nil {
			return nil, err
		}
		switch field {
		case "Number":
			return c.Number, nil
		case "Name":
			return c.Name, nil
		case "CreatedAt":
			return c.CreatedAt, nil
		case "InHook":
			return c.InHook, nil
		case "ExpiredAt":
			return c.ExpiredAt, nil
		default:
			return nil, fmt.Errorf("spike: card has no field %q", field)
		}
	}
}
