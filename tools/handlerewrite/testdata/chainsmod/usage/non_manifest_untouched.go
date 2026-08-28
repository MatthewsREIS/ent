package usage

import "example.com/chainsmod/other"

// widgetUntouched calls a builder type shaped just like a tracked one
// (WidgetCreate.SetName) but declared in a package outside -pkgprefix — the
// blast-radius guard must leave it untouched.
func widgetUntouched(c *other.WidgetCreate) {
	c.SetName("x")
}
