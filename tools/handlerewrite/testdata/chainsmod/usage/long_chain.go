package usage

import "example.com/chainsmod/gen"

func longChain(client *gen.Client) {
	client.Escrow.Create().SetName("x").SetBio("y").SetScore(5)
}
