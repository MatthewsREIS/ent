package usage

import (
	"context"

	"example.com/chainsmod/gen"
)

func chainInterruptedByWhere(client *gen.Client, ctx context.Context) {
	client.Escrow.Update().SetName("x").Where().SetBio("y").Save(ctx)
}
