package pdpv2

import (
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"golang.org/x/xerrors"
)

// -------------------- proof related routines ------------------- //
func splitSegmentToAtoms(data []byte, typ int) ([]Fr, error) {
	if len(data) == 0 {
		return nil, xerrors.New("length is zero")
	}

	if typ > 32 || typ <= 0 {
		return nil, pdpcommon.ErrSegmentSize
	}

	num := (len(data)-1)/typ + 1

	atom := make([]Fr, num)

	for i := 0; i < num-1; i++ {
		bls.FrFromBytes(&atom[i], data[typ*i:typ*(i+1)])
	}

	// last one
	bls.FrFromBytes(&atom[num-1], data[typ*(num-1):])

	return atom, nil
}

func Polydiv(poly []Fr, fr_r Fr) []Fr {
	// poly(x) - poly(r) always divides (x - r) since the latter is a root of the former.
	// With that infomation we can jump into the division.
	quotient := make([]Fr, len(poly)-1)
	if len(quotient) > 0 {
		// q_(n - 1) = p_n
		quotient[len(quotient)-1] = poly[len(quotient)]
		for j := len(quotient) - 2; j >= 0; j-- {
			// q_j = p_(j + 1) + q_(j + 1) * r
			bls.FrMulMod(&quotient[j], &quotient[j+1], &fr_r)
			bls.FrAddMod(&quotient[j], &quotient[j], &poly[j+1])
		}
	}
	return quotient
}

func CheckTag(tag []byte) error {
	var tempG1 G1
	return bls.G1Deserialize(&tempG1, tag)
}
