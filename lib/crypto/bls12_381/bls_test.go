package bls

import (
	"bytes"
	"fmt"
	"testing"
)

func TestG1Compression(t *testing.T) {
	var x Fr
	FrSetStr(&x, "44689111813071777962210527909085028157792767057343609826799812096627770269092")
	var point G1Point
	G1Mul(&point, &GenG1, &x)
	got := G1Serialize(&point)

	expected := []byte{134, 87, 163, 76, 148, 138, 55, 228, 171, 85, 80, 116, 242, 13, 169, 151, 167, 6, 219, 183, 108, 254, 214, 99, 184, 231, 210, 201, 39, 69, 184, 188, 105, 194, 22, 32, 9, 57, 220, 81, 82, 164, 97, 236, 201, 116, 2, 83}

	if !bytes.Equal(expected, got) {
		t.Fatalf("Invalid compression result, %v \n!=\n %v", got, expected)
	}
	var dG1 G1Point
	fmt.Println("dG1:", dG1)
	err := G1Deserialize(&dG1, expected)
	if err != nil || !G1Equal(&point, &dG1) {
		t.Fatalf("Invalid deserialize result, \n%v \n!=\n %v", point.String(), dG1.String())
	}
}

func TestG2Compression(t *testing.T) {
	var x Fr
	FrSetStr(&x, "44689111813071777962210527909085028157792767057343609826799812096627770269092")
	var point G2Point
	G2Mul(&point, &GenG2, &x)
	got := G2Serialize(&point)

	expected := []byte{172, 33, 85, 102, 50, 91, 180, 123, 82, 82, 202, 136, 250, 89, 49, 245, 196, 249, 36, 254, 24, 224, 208, 137, 131, 180, 21, 2, 13, 56, 44, 16, 74, 29, 143, 64, 254, 64, 129, 227, 171, 157, 242, 249, 148, 228, 87, 76, 6, 244, 40, 13, 135, 93, 77, 12, 110, 15, 7, 30, 157, 224, 68, 157, 40, 102, 50, 74, 85, 23, 93, 248, 61, 143, 55, 10, 60, 209, 163, 42, 167, 70, 54, 233, 36, 61, 17, 255, 132, 129, 78, 70, 129, 96, 119, 169}

	if !bytes.Equal(expected, got) {
		t.Fatalf("Invalid compression result, %v \n!=\n %v", got, expected)
	}
	var dG2 G2Point
	fmt.Println("dG2:", dG2)
	err := G2Deserialize(&dG2, expected)
	if err != nil || !G2Equal(&point, &dG2) {
		t.Fatalf("Invalid deserialize result, \n%v \n!=\n %v", point.String(), dG2.String())
	}
}

func BenchmarkPairingVerify(be *testing.B) {
	var a, an G1Point
	var b, bn G2Point
	var n Fr
	FrSetInt64(&n, 108)
	G1Mul(&a, &GenG1, &n)
	G1Mul(&an, &a, &n)

	G2Mul(&b, &GenG2, &n)
	G2Mul(&bn, &b, &n)
	for i := 0; i < be.N; i++ {
		if !PairingsVerify(&an, &b, &a, &bn) {
			panic("not equal")
		}
	}
}

func TestPairingVerify(t *testing.T) {
	var a, an G1Point
	var b, bn G2Point
	var n Fr
	FrSetInt64(&n, 108)
	G1Mul(&a, &GenG1, &n)
	G1Mul(&an, &a, &n)

	G2Mul(&b, &GenG2, &n)
	G2Mul(&bn, &b, &n)
	if !PairingsVerify(&an, &b, &a, &bn) {
		panic("not equal")
	}
}

func BenchmarkMultiPairing(b *testing.B) {
	var a1, b1 G1Point
	var a2, b2 G2Point
	var ab1 G1Point
	var ab2 G2Point

	// e(a1,b2) = e(b1,a2)
	// e(ab1,g2n) = e(g1,ab2)
	var n Fr
	FrSetInt64(&n, 1025)
	G1Mul(&a1, &GenG1, &n)
	G1Mul(&b1, &a1, &n)

	G2Mul(&a2, &GenG2, &n)
	G2Mul(&b2, &a2, &n)

	G1Add(&ab1, &a1, &b1)
	G2Add(&ab2, &a2, &b2)

	for i := 0; i < b.N; i++ {
		if !PairingsVerifyMulti([]*G1Point{&a1, &ab1}, []*G2Point{&b2, &GenG2}, []*G1Point{&b1, &GenG1}, []*G2Point{&a2, &ab2}) {
			panic("not equal")
		}
	}
}

func TestPairingSinple(t *testing.T) {
	var a, an G1Point
	var b, bn G2Point
	var lhs, rhs GTPoint

	var n Fr
	FrSetInt64(&n, 108)
	G1Mul(&a, &GenG1, &n)
	G1Mul(&an, &a, &n)

	G2Mul(&b, &GenG2, &n)
	G2Mul(&bn, &b, &n)

	Pairing(&lhs, &a, &bn)
	Pairing(&rhs, &an, &b)
	if !GTEqual(&lhs, &rhs) {
		panic("not equal")
	}
}

func TestMultiPairing(t *testing.T) {
	var a1, b1 G1Point
	var a2, b2 G2Point
	var ab1 G1Point
	var ab2 G2Point

	// e(a1,b2) = e(b1,a2)
	// e(ab1,g2n) = e(g1,ab2)
	var n Fr
	FrSetInt64(&n, 1025)
	G1Mul(&a1, &GenG1, &n)
	G1Mul(&b1, &a1, &n)

	G2Mul(&a2, &GenG2, &n)
	G2Mul(&b2, &a2, &n)

	G1Add(&ab1, &a1, &b1)
	G2Add(&ab2, &a2, &b2)

	if !PairingsVerify(&a1, &b2, &b1, &a2) {
		panic("not equal 0")
	}

	if !PairingsVerify(&ab1, &GenG2, &GenG1, &ab2) {
		panic("not equal 1")
	}

	if !PairingsVerifyMulti([]*G1Point{&a1, &ab1}, []*G2Point{&b2, &GenG2}, []*G1Point{&b1, &GenG1}, []*G2Point{&a2, &ab2}) {
		panic("not equal")
	}
}

func TestPairingGt(t *testing.T) {
	var a1, b1 G1Point
	var a2, b2 G2Point
	var ab1 G1Point
	var ab2 G2Point

	// e(a1,b2) = e(b1,a2)
	// e(ab1,g2n) = e(g1,ab2)
	var n Fr
	FrSetInt64(&n, 108)
	G1Mul(&a1, &GenG1, &n)
	G1Mul(&b1, &a1, &n)

	G2Mul(&a2, &GenG2, &n)
	G2Mul(&b2, &a2, &n)

	G1Add(&ab1, &a1, &b1)
	G2Add(&ab2, &a2, &b2)

	if !PairingsVerify(&a1, &b2, &b1, &a2) {
		panic("not equal 0")
	}

	if !PairingsVerify(&ab1, &GenG2, &GenG1, &ab2) {
		panic("not equal 1")
	}

	if !PairingsVerifyMulti([]*G1Point{&a1, &ab1}, []*G2Point{&b2, &GenG2}, []*G1Point{&b1, &GenG1}, []*G2Point{&a2, &ab2}) {
		panic("not equal")
	}
}
