package hookref

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// User's Hooks() method references userHook, which is defined only in a
// //go:build !entcodegen file. The loader injects the entcodegen build tag, so
// that file is excluded and a plain load fails with "undefined: userHook".
// The loader must stage a hook-stripped copy and retry, mirroring what
// entc.Generate's bootstrap staging does for codegen.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		userHook(),
	}
}
