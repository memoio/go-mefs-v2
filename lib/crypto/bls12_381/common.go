package bls

func AddFr(a, b []byte) []byte {
	var aFr, bFr Fr
	FrFromBytes(&aFr, a)
	FrFromBytes(&bFr, b)
	FrAddMod(&aFr, &aFr, &bFr)
	return FrToBytes(&aFr)
}

func SubFr(a, b []byte) []byte {
	var aFr, bFr Fr
	FrFromBytes(&aFr, a)
	FrFromBytes(&bFr, b)
	FrSubMod(&aFr, &aFr, &bFr)
	return FrToBytes(&aFr)
}
