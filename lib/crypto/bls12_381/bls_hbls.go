// +build herumi

package bls

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/herumi/bls-eth-go-binary/bls"
	hbls "github.com/herumi/bls-eth-go-binary/bls"
)

var ZERO_G1 G1Point

var GenG1 G1Point
var GenG2 G2Point

var ZeroG1 G1Point
var ZeroG2 G2Point

// Herumi BLS doesn't offer these points to us, so we have to work around it by declaring them ourselves.
func initG1G2() {
	GenG1.X.SetString("3685416753713387016781088315183077757961620795782546409894578378688607592378376318836054947676345821548104185464507", 10)
	GenG1.Y.SetString("1339506544944476473020471379941921221584933875938349620426543736416511423956333506472724655353366534992391756441569", 10)
	GenG1.Z.SetInt64(1)

	GenG2.X.D[0].SetString("352701069587466618187139116011060144890029952792775240219908644239793785735715026873347600343865175952761926303160", 10)
	GenG2.X.D[1].SetString("3059144344244213709971259814753781636986470325476647558659373206291635324768958432433509563104347017837885763365758", 10)
	GenG2.Y.D[0].SetString("1985150602287291935568054521177171638300868978215655730859378665066344726373823718423869104263333984641494340347905", 10)
	GenG2.Y.D[1].SetString("927553665492332455747201965776037880757740193453592970025027978793976877002675564980949289727957565575433344219582", 10)
	GenG2.Z.D[0].SetInt64(1)
	GenG2.Z.D[1].Clear()

	ZeroG1.X.SetInt64(1)
	ZeroG1.Y.SetInt64(1)
	ZeroG1.Z.SetInt64(0)

	ZeroG2.X.D[0].SetInt64(1)
	ZeroG2.X.D[1].SetInt64(0)
	ZeroG2.Y.D[0].SetInt64(1)
	ZeroG2.Y.D[1].SetInt64(0)
	ZeroG2.Z.D[0].SetInt64(0)
	ZeroG2.Z.D[1].SetInt64(0)
}

// TODO types file, swap BLS with build args
type G1Point hbls.G1

func G1Clear(x *G1Point) {
	(*hbls.G1)(x).Clear()
}

func G1Copy(dst *G1Point, v *G1Point) {
	*dst = *v
}

func G1Mul(dst *G1Point, a *G1Point, b *Fr) {
	hbls.G1Mul((*hbls.G1)(dst), (*hbls.G1)(a), (*hbls.Fr)(b))
}

func G1Add(dst *G1Point, a *G1Point, b *G1Point) {
	hbls.G1Add((*hbls.G1)(dst), (*hbls.G1)(a), (*hbls.G1)(b))
}

func G1Sub(dst *G1Point, a *G1Point, b *G1Point) {
	hbls.G1Sub((*hbls.G1)(dst), (*hbls.G1)(a), (*hbls.G1)(b))
}

func G1Str(v *G1Point) string {
	return (*hbls.G1)(v).GetString(10)
}

func G1Equal(a *G1Point, b *G1Point) bool {
	return (*hbls.G1)(a).IsEqual((*hbls.G1)(b))
}

func G1IsZero(v *G1Point) bool {
	return (*hbls.G1)(v).IsZero()
}

func G1Neg(dst *G1Point) {
	// in-place should be safe here (TODO double check)
	hbls.G1Neg((*hbls.G1)(dst), (*hbls.G1)(dst))
}

func G1ToCompressed(p *G1Point) []byte {
	return (*hbls.G1)(p).Serialize()
}

func G1Serialize(p *G1Point) []byte {
	return (*hbls.G1)(p).Serialize()
}

func G1Deserialize(p *G1Point, v []byte) error {
	return (*hbls.G1)(p).Deserialize(v)
}

func G1MulVec(out *G1Point, numbers []G1Point, factors []Fr) *G1Point {
	// We're just using unsafe to cast elements that are an alias anyway, no problem.
	// Go doesn't let us do the cast otherwise without copy.
	hbls.G1MulVec((*hbls.G1)(out), *(*[]hbls.G1)(unsafe.Pointer(&numbers)), *(*[]hbls.Fr)(unsafe.Pointer(&factors)))
	return out
}

func DebugG1s(msg string, values []G1Point) {
	var out strings.Builder
	for i := range values {
		out.WriteString(fmt.Sprintf("%s %d: %s\n", msg, i, G1Str(&values[i])))
	}
	fmt.Println(out.String())
}

type G2Point hbls.G2

func G2Clear(x *G2Point) {
	(*hbls.G2)(x).Clear()
}

func G2Copy(dst *G2Point, v *G2Point) {
	*dst = *v
}

func G2Mul(dst *G2Point, a *G2Point, b *Fr) {
	hbls.G2Mul((*hbls.G2)(dst), (*hbls.G2)(a), (*hbls.Fr)(b))
}

func G2Add(dst *G2Point, a *G2Point, b *G2Point) {
	hbls.G2Add((*hbls.G2)(dst), (*hbls.G2)(a), (*hbls.G2)(b))
}

func G2Sub(dst *G2Point, a *G2Point, b *G2Point) {
	hbls.G2Sub((*hbls.G2)(dst), (*hbls.G2)(a), (*hbls.G2)(b))
}

func G2Neg(dst *G2Point) {
	// in-place should be safe here (TODO double check)
	hbls.G2Neg((*hbls.G2)(dst), (*hbls.G2)(dst))
}

func G2Str(v *G2Point) string {
	return (*hbls.G2)(v).GetString(10)
}

func G2Equal(a *G2Point, b *G2Point) bool {
	return (*hbls.G2)(a).IsEqual((*hbls.G2)(b))
}

func G2IsZero(v *G2Point) bool {
	return (*hbls.G2)(v).IsZero()
}

func G2Serialize(p *G2Point) []byte {
	return (*hbls.G2)(p).Serialize()
}

func G2Deserialize(p *G2Point, v []byte) error {
	return (*hbls.G2)(p).Deserialize(v)
}

func G2MulVec(out *G2Point, numbers []G2Point, factors []Fr) *G2Point {
	// We're just using unsafe to cast elements that are an alias anyway, no problem.
	// Go doesn't let us do the cast otherwise without copy.
	hbls.G2MulVec((*hbls.G2)(out), *(*[]hbls.G2)(unsafe.Pointer(&numbers)), *(*[]hbls.Fr)(unsafe.Pointer(&factors)))
	return out
}

type GTPoint hbls.GT

func GTMul(dst *GTPoint, a *GTPoint, b *GTPoint) {
	hbls.GTMul((*hbls.GT)(dst), (*hbls.GT)(a), (*hbls.GT)(b))
}

func GTAdd(dst *GTPoint, a *GTPoint, b *GTPoint) {
	hbls.GTAdd((*hbls.GT)(dst), (*hbls.GT)(a), (*hbls.GT)(b))
}

func GTSub(dst *GTPoint, a *GTPoint, b *GTPoint) {
	hbls.GTSub((*hbls.GT)(dst), (*hbls.GT)(a), (*hbls.GT)(b))
}

func GTEqual(a *GTPoint, b *GTPoint) bool {
	return (*hbls.GT)(a).IsEqual((*hbls.GT)(b))
}

// e(a1^(-1), a2) * e(b1,  b2) = 1_T
func PairingsVerify(a1 *G1Point, a2 *G2Point, b1 *G1Point, b2 *G2Point) bool {
	var tmp hbls.GT
	hbls.Pairing(&tmp, (*hbls.G1)(a1), (*hbls.G2)(a2))
	var tmp2 hbls.GT
	hbls.Pairing(&tmp2, (*hbls.G1)(b1), (*hbls.G2)(b2))

	return tmp.IsEqual(&tmp2)
}

func PairingsVerifyMulti(a1 []*G1Point, a2 []*G2Point, b1 []*G1Point, b2 []*G2Point) bool {
	if len(a1) != len(a2) || len(b1) != len(b2) {
		return false
	}

	var lhs, rhs bls.GT
	var lhst, rhst bls.GT
	for i := 0; i < len(a1); i++ {
		hbls.Pairing(&lhst, (*hbls.G1)(a1[i]), (*hbls.G2)(a2[i]))
		hbls.GTMul(&lhs, &lhs, &lhst)
	}
	for i := 0; i < len(b1); i++ {
		hbls.Pairing(&rhst, (*hbls.G1)(b1[i]), (*hbls.G2)(b2[i]))
		hbls.GTMul(&rhs, &rhs, &rhst)
	}
	return lhs.IsEqual(&rhs)
}

func Pairing(out *GTPoint, a *G1Point, b *G2Point) {
	hbls.Pairing((*hbls.GT)(out), (*hbls.G1)(a), (*hbls.G2)(b))
}
