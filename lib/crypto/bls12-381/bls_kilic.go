// +build !herumi

package bls

import (
	"fmt"
	"math/big"
	"strings"

	kbls "github.com/kilic/bls12-381"
)

var ZERO_G1 G1Point

var GenG1 G1Point
var GenG2 G2Point

var ZeroG1 G1Point
var ZeroG2 G2Point

// Herumi BLS doesn't offer these points to us, so we have to work around it by declaring them ourselves.
func initG1G2() {
	GenG1 = G1Point(*kbls.NewG1().One())
	GenG2 = G2Point(*kbls.NewG2().One())
	ZeroG1 = G1Point(*kbls.NewG1().Zero())
	ZeroG2 = G2Point(*kbls.NewG2().Zero())
}

// TODO types file, swap BLS with build args
type G1Point kbls.PointG1

// zeroes the point (like herumi BLS does with theirs). This is not co-factor clearing.
func G1Clear(x *G1Point) {
	(*kbls.PointG1)(x).Zero()
}

func G1Copy(dst *G1Point, v *G1Point) {
	*dst = *v
}

func G1Mul(dst *G1Point, a *G1Point, b *Fr) {
	tmp := (kbls.Fr)(*b) // copy, we want to leave the original in mont-red form
	(&tmp).FromRed()
	kbls.NewG1().MulScalar((*kbls.PointG1)(dst), (*kbls.PointG1)(a), &tmp)
}

func G1Add(dst *G1Point, a *G1Point, b *G1Point) {
	kbls.NewG1().Add((*kbls.PointG1)(dst), (*kbls.PointG1)(a), (*kbls.PointG1)(b))
}

func G1Sub(dst *G1Point, a *G1Point, b *G1Point) {
	kbls.NewG1().Sub((*kbls.PointG1)(dst), (*kbls.PointG1)(a), (*kbls.PointG1)(b))
}

func G1Str(v *G1Point) string {
	data := kbls.NewG1().ToUncompressed((*kbls.PointG1)(v))
	var a, b big.Int
	a.SetBytes(data[:48])
	b.SetBytes(data[48:])
	return a.String() + "\n+\n" + b.String()
}

func G1Equal(a *G1Point, b *G1Point) bool {
	return kbls.NewG1().Equal((*kbls.PointG1)(a), (*kbls.PointG1)(b))
}

func G1IsZero(v *G1Point) bool {
	return kbls.NewG1().IsZero((*kbls.PointG1)(v))
}

func G1Neg(dst *G1Point) {
	// in-place should be safe here (TODO double check)
	kbls.NewG1().Neg((*kbls.PointG1)(dst), (*kbls.PointG1)(dst))
}

func G1ToCompressed(p *G1Point) []byte {
	return kbls.NewG1().ToCompressed((*kbls.PointG1)(p))
}

func G1Serialize(p *G1Point) []byte {
	return kbls.NewG1().ToCompressed((*kbls.PointG1)(p))
}

func G1Deserialize(p *G1Point, v []byte) error {
	res, err := kbls.NewG1().FromCompressed(v)
	if err != nil {
		return err
	}
	(*kbls.PointG1)(p).Set(res)
	return nil
}

func G1MulVec(out *G1Point, numbers []G1Point, factors []Fr) *G1Point {
	if len(numbers) != len(factors) {
		panic("got G1MulVec numbers/factors length mismatch")
	}
	tmpG1s := make([]*kbls.PointG1, len(numbers))
	for i := 0; i < len(numbers); i++ {
		tmpG1s[i] = (*kbls.PointG1)(&numbers[i])
	}
	tmpFrs := make([]*kbls.Fr, len(factors))
	for i := 0; i < len(factors); i++ {
		// copy, since we have to change from mont-red form to regular again, and don't want to mutate the input.
		v := *(*kbls.Fr)(&factors[i])
		v.FromRed()
		tmpFrs[i] = &v
	}
	_, _ = kbls.NewG1().MultiExp((*kbls.PointG1)(out), tmpG1s, tmpFrs)
	return out
}

func DebugG1s(msg string, values []G1Point) {
	var out strings.Builder
	for i := range values {
		out.WriteString(fmt.Sprintf("%s %d: %s\n", msg, i, G1Str(&values[i])))
	}
	fmt.Println(out.String())
}

type G2Point kbls.PointG2

// zeroes the point (like herumi BLS does with theirs). This is not co-factor clearing.
func G2Clear(x *G2Point) {
	(*kbls.PointG2)(x).Zero()
}

func G2Copy(dst *G2Point, v *G2Point) {
	*dst = *v
}

func G2Mul(dst *G2Point, a *G2Point, b *Fr) {
	tmp := (kbls.Fr)(*b) // copy, we want to leave the original in mont-red form
	(&tmp).FromRed()
	kbls.NewG2().MulScalar((*kbls.PointG2)(dst), (*kbls.PointG2)(a), &tmp)
}

func G2Add(dst *G2Point, a *G2Point, b *G2Point) {
	kbls.NewG2().Add((*kbls.PointG2)(dst), (*kbls.PointG2)(a), (*kbls.PointG2)(b))
}

func G2Sub(dst *G2Point, a *G2Point, b *G2Point) {
	kbls.NewG2().Sub((*kbls.PointG2)(dst), (*kbls.PointG2)(a), (*kbls.PointG2)(b))
}

func G2Neg(dst *G2Point) {
	// in-place should be safe here (TODO double check)
	kbls.NewG2().Neg((*kbls.PointG2)(dst), (*kbls.PointG2)(dst))
}

func G2Str(v *G2Point) string {
	data := kbls.NewG2().ToUncompressed((*kbls.PointG2)(v))
	var a, b big.Int
	a.SetBytes(data[:96])
	b.SetBytes(data[96:])
	return a.String() + "\n" + b.String()
}

func G2Equal(a *G2Point, b *G2Point) bool {
	return kbls.NewG2().Equal((*kbls.PointG2)(a), (*kbls.PointG2)(b))
}

func G2IsZero(v *G2Point) bool {
	return kbls.NewG2().IsZero((*kbls.PointG2)(v))
}

func G2Serialize(p *G2Point) []byte {
	return kbls.NewG2().ToCompressed((*kbls.PointG2)(p))
}

func G2Deserialize(p *G2Point, v []byte) error {
	res, err := kbls.NewG2().FromCompressed(v)
	if err != nil {
		return err
	}
	(*kbls.PointG2)(p).Set(res)
	return nil
}

func G2MulVec(out *G2Point, numbers []G2Point, factors []Fr) *G2Point {
	if len(numbers) != len(factors) {
		panic("got G2MulVec numbers/factors length mismatch")
	}
	tmpG2s := make([]*kbls.PointG2, len(numbers))
	for i := 0; i < len(numbers); i++ {
		tmpG2s[i] = (*kbls.PointG2)(&numbers[i])
	}
	tmpFrs := make([]*kbls.Fr, len(factors))
	for i := 0; i < len(factors); i++ {
		// copy, since we have to change from mont-red form to regular again, and don't want to mutate the input.
		v := *(*kbls.Fr)(&factors[i])
		v.FromRed()
		tmpFrs[i] = &v
	}
	_, _ = kbls.NewG2().MultiExp((*kbls.PointG2)(out), tmpG2s, tmpFrs)
	return out
}

type GTPoint kbls.E

func GTMul(dst *GTPoint, a *GTPoint, b *GTPoint) {
	kbls.NewGT().Mul((*kbls.E)(dst), (*kbls.E)(a), (*kbls.E)(b))
}

func GTAdd(dst *GTPoint, a *GTPoint, b *GTPoint) {
	kbls.NewGT().Add((*kbls.E)(dst), (*kbls.E)(a), (*kbls.E)(b))
}

func GTSub(dst *GTPoint, a *GTPoint, b *GTPoint) {
	kbls.NewGT().Sub((*kbls.E)(dst), (*kbls.E)(a), (*kbls.E)(b))
}

func GTEqual(a *GTPoint, b *GTPoint) bool {
	return (*kbls.E)(a).Equal((*kbls.E)(b))
}

// e(a1^(-1), a2) * e(b1,  b2) = 1_T
func PairingsVerify(a1 *G1Point, a2 *G2Point, b1 *G1Point, b2 *G2Point) bool {
	pairingEngine := kbls.NewEngine()
	pairingEngine.AddPairInv((*kbls.PointG1)(a1), (*kbls.PointG2)(a2))
	pairingEngine.AddPair((*kbls.PointG1)(b1), (*kbls.PointG2)(b2))
	return pairingEngine.Check()
}

func PairingsVerifyMulti(a1 []*G1Point, a2 []*G2Point, b1 []*G1Point, b2 []*G2Point) bool {
	if len(a1) != len(a2) || len(b1) != len(b2) {
		return false
	}
	pairingEngine := kbls.NewEngine()
	for i := 0; i < len(a1); i++ {
		pairingEngine.AddPairInv((*kbls.PointG1)(a1[i]), (*kbls.PointG2)(a2[i]))
	}
	for i := 0; i < len(b1); i++ {
		pairingEngine.AddPair((*kbls.PointG1)(b1[i]), (*kbls.PointG2)(b2[i]))
	}
	return pairingEngine.Check()
}

func Pairing(out *GTPoint, a *G1Point, b *G2Point) {
	pairingEngine := kbls.NewEngine()
	pairingEngine.AddPair((*kbls.PointG1)(a), (*kbls.PointG2)(b))
	res := pairingEngine.Result()
	(*kbls.E)(out).Set(res)
}
