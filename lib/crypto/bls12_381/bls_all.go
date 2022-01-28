package bls

const G1Size = 48
const G2Size = 96
const FrSize = 32

func (p *G1Point) String() string {
	return G1Str(p)
}

func (p *G2Point) String() string {
	return G2Str(p)
}
