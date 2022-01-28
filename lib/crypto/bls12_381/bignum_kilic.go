// +build !herumi

package bls

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"

	kbls "github.com/kilic/bls12-381"
	"golang.org/x/xerrors"
)

func init() {
	initGlobals()
	G1Clear(&ZERO_G1)
	initG1G2()
}

// Note: with Kilic BLS, we exclusively represent Fr in mont-red form.
// Whenever it is used with G1/G2, it needs to be normalized first.
type Fr kbls.Fr

func FrVersion() int64 {
	return 1
}

func FrSetStr(dst *Fr, v string) {
	var bv big.Int
	bv.SetString(v, 10)
	(*kbls.Fr)(dst).RedFromBytes(bv.Bytes())
	// big don't support neg to bytes
	if bv.Sign() < 0 {
		(*kbls.Fr)(dst).Neg((*kbls.Fr)(dst))
	}
}

func FrSetInt64(dst *Fr, v int64) {
	var bv big.Int
	bv.SetInt64(v)
	(*kbls.Fr)(dst).RedFromBytes(bv.Bytes())
	if v < 0 {
		(*kbls.Fr)(dst).Neg((*kbls.Fr)(dst))
	}
}

func FrClear(dst *Fr) {
	(*kbls.Fr)(dst).Zero()
}

func FrFromBytes(dst *Fr, v []byte) error {
	if len(v) > 32 {
		return xerrors.New("invalid Fr")
	}
	b := make([]byte, 32)
	copy(b, v)
	// reverse endianness, Kilic Fr takes big-endian bytes
	last := 31
	for i := 0; i < 16; i++ {
		b[i], b[last-i] = b[last-i], b[i]
	}

	(*kbls.Fr)(dst).RedFromBytes(b)
	return nil
}

func FrToBytes(src *Fr) []byte {
	v := FrTo32(src)
	return v[:]
}

// FrFrom32 mutates the fr num. The value v is little-endian 32-bytes.
// Returns false, without modifying dst, if the value is out of range.
func FrFrom32(dst *Fr, v [32]byte) error {
	if !ValidFr(v) {
		return xerrors.New("invalid Fr")
	}
	// reverse endianness, Kilic Fr takes big-endian bytes
	for i := 0; i < 16; i++ {
		v[i], v[31-i] = v[31-i], v[i]
	}
	(*kbls.Fr)(dst).RedFromBytes(v[:])
	return nil
}

// FrTo32 serializes a fr number to 32 bytes. Encoded little-endian.
func FrTo32(src *Fr) (v [32]byte) {
	b := (*kbls.Fr)(src).RedToBytes()
	last := len(b) - 1
	// reverse endianness, Kilic Fr outputs big-endian bytes
	for i := 0; i < 16; i++ {
		b[i], b[last-i] = b[last-i], b[i]
	}
	copy(v[:], b)
	return
}

func FrCopy(dst *Fr, v *Fr) {
	*dst = *v
}

func AsFr(dst *Fr, i uint64) {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], i)
	(*kbls.Fr)(dst).RedFromBytes(data[:])
}

func FrStr(b *Fr) string {
	if b == nil {
		return "<nil>"
	}
	return (*kbls.Fr)(b).RedToBig().String()
}

func EqualOne(v *Fr) bool {
	return (*kbls.Fr)(v).IsRedOne()
}

func EqualZero(v *Fr) bool {
	return (*kbls.Fr)(v).IsZero()
}

func FrEqual(a *Fr, b *Fr) bool {
	return (*kbls.Fr)(a).Equal((*kbls.Fr)(b))
}

func FrRandom(a *Fr) *Fr {
	if _, err := (*kbls.Fr)(a).Rand(rand.Reader); err != nil {
		panic(err)
	}
	(*kbls.Fr)(a).ToRed()
	return a
}

func FrSubMod(dst *Fr, a, b *Fr) {
	(*kbls.Fr)(dst).Sub((*kbls.Fr)(a), (*kbls.Fr)(b))
}

func FrAddMod(dst *Fr, a, b *Fr) {
	(*kbls.Fr)(dst).Add((*kbls.Fr)(a), (*kbls.Fr)(b))
}

func FrDivMod(dst *Fr, a, b *Fr) {
	var tmp kbls.Fr
	tmp.RedInverse((*kbls.Fr)(b))
	(*kbls.Fr)(dst).RedMul(&tmp, (*kbls.Fr)(a))
}

func FrMulMod(dst *Fr, a, b *Fr) {
	(*kbls.Fr)(dst).RedMul((*kbls.Fr)(a), (*kbls.Fr)(b))
}

func FrInvMod(dst *Fr, v *Fr) {
	(*kbls.Fr)(dst).RedInverse((*kbls.Fr)(v))
}

func FrNegMod(dst *Fr, v *Fr) {
	(*kbls.Fr)(dst).Neg((*kbls.Fr)(v))
}

func SqrModFr(dst *Fr, v *Fr) {
	(*kbls.Fr)(dst).RedSquare((*kbls.Fr)(v))
}

func EvalPolyAt(dst *Fr, p []Fr, x *Fr) {
	// TODO: kilic BLS has no optimized evaluation function
	EvalPolyAtUnoptimized(dst, p, x)
}
