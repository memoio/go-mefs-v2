package types

type ChalEpoch struct {
	Epoch uint64
	Prev  []byte
}

func (c *ChalEpoch) NewSeed() {

}
