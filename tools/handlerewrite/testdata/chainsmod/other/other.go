// Package other lives outside the "example.com/chainsmod/gen" import path
// prefix used by the -chains fixture tests, so a WidgetCreate call site
// exercises the blast-radius guard (a builder type not under -pkgprefix is
// untouched) even though its method names would otherwise decompose.
package other

type WidgetCreate struct{}

func (c *WidgetCreate) SetName(v string) *WidgetCreate { return c }
