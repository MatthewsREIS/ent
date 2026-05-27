//go:build !entcodegen

package hookref

import "entgo.io/ent"

// userHook stands in for a hook body that references not-yet-generated code.
// It lives in a build-excluded file so it is invisible under the entcodegen
// tag the loader injects.
func userHook() ent.Hook {
	return nil
}
