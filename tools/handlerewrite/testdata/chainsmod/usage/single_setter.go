package usage

import "example.com/chainsmod/gen"

func singleSetter(client *gen.Client) {
	client.Escrow.Create().SetName("x")
}
