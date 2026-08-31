package usage

import (
	"example.com/chainsmod/gen"
	esc "example.com/chainsmod/gen/escrow"
)

var _ esc.Marker

func aliasedImportReused(client *gen.Client) {
	client.Escrow.Create().SetName("x")
}
