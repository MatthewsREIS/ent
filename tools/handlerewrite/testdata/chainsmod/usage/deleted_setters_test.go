package usage

import (
	"testing"

	"example.com/chainsmod/gen"
)

// TestVoucherDeletedSetters exercises two things at once: (a) go/packages
// loading _test.go files at all (packages.Config.Tests must be true — see
// ProcessPackages — or every setter-chain call site in a _test.go file,
// virtually the whole real migration, is invisible to -chains), and
// (b) chain-fold propagation across links whose receiver type can't
// resolve at all (see VoucherCreate's doc comment in gen/gen.go — none of
// its old setters exist, matching what real regenerated code looks like).
func TestVoucherDeletedSetters(t *testing.T) {
	var client *gen.Client
	client.Voucher.Create().SetTitle("x").SetDesc("y").SetPrice(5)
}
