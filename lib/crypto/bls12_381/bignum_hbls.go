// +build herumi

package bls

import (
	"errors"
	"unsafe"

	hbls "github.com/herumi/bls-eth-go-binary/bls"
)

func init() {
	hbls.Init(hbls.BLS12_381)
	initGlobals()
	G1Clear(&ZERO_G1)
	initG1G2()
}

type Fr hbls.Fr

func FrVersion() int64 {
	return 0
}

func FrSetStr(dst *Fr, v string) {
	err := (*hbls.Fr)(dst).SetString(v, 10)
	if err != nil {
		panic(err)
	}
}

func FrSetInt64(dst *Fr, v int64) {
	(*hbls.Fr)(dst).SetInt64(v)
}

func FrClear(dst *Fr) {
	(*hbls.Fr)(dst).Clear()
}

func FrFromBytes(dst *Fr, v []byte) error {
	return (*hbls.Fr)(dst).SetLittleEndianMod(v)
}

// FrFrom32 mutates the fr num. The value v is little-endian 32-bytes.
// Returns false, without modifying dst, if the value is out of range.
func FrFrom32(dst *Fr, v [32]byte) error {
	if !ValidFr(v) {
		return xerrors.New("invalid Fr")
	}
	return (*hbls.Fr)(dst).SetLittleEndianMod(v[:])
}

func FrToBytes(src *Fr) []byte {
	b := (*hbls.Fr)(src).Serialize()
	last := len(b) - 1
	// reverse endianness, Herumi outputs big-endian bytes
	for i := 0; i < 16; i++ {
		b[i], b[last-i] = b[last-i], b[i]
	}
	return b
}

// FrTo32 serializes a fr number to 32 bytes. Encoded little-endian.
func FrTo32(src *Fr) (v [32]byte) {
	b := (*hbls.Fr)(src).Serialize()
	last := len(b) - 1
	// reverse endianness, Herumi outputs big-endian bytes
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
	(*hbls.Fr)(dst).SetInt64(int64(i))
}

func FrStr(b *Fr) string {
	if b == nil {
		return "<nil>"
	}
	return (*hbls.Fr)(b).GetString(10)
}

func EqualOne(v *Fr) bool {
	return (*hbls.Fr)(v).IsOne()
}

func EqualZero(v *Fr) bool {
	return (*hbls.Fr)(v).IsZero()
}

func FrEqual(a *Fr, b *Fr) bool {
	return (*hbls.Fr)(a).IsEqual((*hbls.Fr)(b))
}

func FrRandom(a *Fr) *Fr {
	(*hbls.Fr)(a).SetByCSPRNG()
	return a
}

func FrSubMod(dst *Fr, a, b *Fr) {
	hbls.FrSub((*hbls.Fr)(dst), (*hbls.Fr)(a), (*hbls.Fr)(b))
}

func FrAddMod(dst *Fr, a, b *Fr) {
	hbls.FrAdd((*hbls.Fr)(dst), (*hbls.Fr)(a), (*hbls.Fr)(b))
}

func FrDivMod(dst *Fr, a, b *Fr) {
	hbls.FrDiv((*hbls.Fr)(dst), (*hbls.Fr)(a), (*hbls.Fr)(b))
}

func FrMulMod(dst *Fr, a, b *Fr) {
	hbls.FrMul((*hbls.Fr)(dst), (*hbls.Fr)(a), (*hbls.Fr)(b))
}

func FrInvMod(dst *Fr, v *Fr) {
	hbls.FrInv((*hbls.Fr)(dst), (*hbls.Fr)(v))
}

func FrNegMod(dst *Fr, v *Fr) {
	hbls.FrNeg((*hbls.Fr)(dst), (*hbls.Fr)(v))
}

func SqrModFr(dst *Fr, v *Fr) {
	hbls.FrSqr((*hbls.Fr)(dst), (*hbls.Fr)(v))
}

func EvalPolyAt(dst *Fr, p []Fr, x *Fr) {
	err := hbls.FrEvaluatePolynomial(
		(*hbls.Fr)(dst),
		*(*[]hbls.Fr)(unsafe.Pointer(&p)),
		(*hbls.Fr)(x),
	)
	if err != nil {
		panic(err) // TODO: why does the herumi API return an error? When coefficients are empty?
	}
}
