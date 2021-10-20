package bls

import (
	"testing"
)

// These are sanity tests, to see if whatever bignum library that is being
// used actually handles dst/arg overlaps well.

func TestInplaceAdd(t *testing.T) {
	var aVal, bVal Fr
	FrRandom(&aVal)
	FrRandom(&aVal)
	aPlusB := new(Fr)
	FrAddMod(aPlusB, &aVal, &bVal)
	twoA := new(Fr)
	FrMulMod(twoA, &aVal, &TWO)

	check := func(name string, fn func(a, b *Fr) bool) {
		t.Run(name, func(t *testing.T) {
			var a, b Fr
			FrCopy(&a, &aVal)
			FrCopy(&b, &bVal)
			if !fn(&a, &b) {
				t.Error("fail")
			}
		})
	}
	check("dst equals lhs", func(a *Fr, b *Fr) bool {
		FrAddMod(a, a, b)
		return FrEqual(a, aPlusB)
	})
	check("dst equals rhs", func(a *Fr, b *Fr) bool {
		FrAddMod(b, a, b)
		return FrEqual(b, aPlusB)
	})
	check("dst equals lhs and rhs", func(a *Fr, b *Fr) bool {
		FrAddMod(a, a, a)
		return FrEqual(a, twoA)
	})
}

func TestInplaceMul(t *testing.T) {
	var aVal, bVal Fr
	FrRandom(&aVal)
	FrRandom(&aVal)
	aMulB := new(Fr)
	FrMulMod(aMulB, &aVal, &bVal)
	squareA := new(Fr)
	FrMulMod(squareA, &aVal, &aVal)

	check := func(name string, fn func(a, b *Fr) bool) {
		t.Run(name, func(t *testing.T) {
			var a, b Fr
			FrCopy(&a, &aVal)
			FrCopy(&b, &bVal)
			if !fn(&a, &b) {
				t.Error("fail")
			}
		})
	}
	check("dst equals lhs", func(a *Fr, b *Fr) bool {
		FrMulMod(a, a, b)
		return FrEqual(a, aMulB)
	})
	check("dst equals rhs", func(a *Fr, b *Fr) bool {
		FrMulMod(b, a, b)
		return FrEqual(b, aMulB)
	})
	check("dst equals lhs and rhs", func(a *Fr, b *Fr) bool {
		FrMulMod(a, a, a)
		return FrEqual(a, squareA)
	})
}

// Sanity check the mod div function, some libraries do regular integer div.
func TestFrDivMod(t *testing.T) {
	var aVal Fr
	FrSetStr(&aVal, "26444158170683616486493062254748234829545368615823006596610545696213139843950")
	var bVal Fr
	FrSetStr(&bVal, "44429412392042760961177795624245903017634520927909329890010277843232676752272")
	var div Fr
	FrDivMod(&div, &aVal, &bVal)

	var invB Fr
	FrInvMod(&invB, &bVal)
	var mulInv Fr
	FrMulMod(&mulInv, &aVal, &invB)

	if !FrEqual(&div, &mulInv) {
		t.Fatalf("div num not equal to mul inv: %s, %s", FrStr(&div), FrStr(&mulInv))
	}
}

func TestValidFr(t *testing.T) {
	data := FrTo32(&MODULUS_MINUS1)
	if !ValidFr(data) {
		t.Fatal("expected mod-1 to be valid")
	}
	var tmp [32]byte
	for i := 0; i < 32; i++ {
		if data[i] == 0xff {
			continue
		}
		tmp = data
		tmp[i] += 1
		if ValidFr(tmp) {
			t.Fatal("expected anything larger than mod-1 to be invalid")
		}
	}
	var v Fr
	FrRandom(&v)
	data = FrTo32(&v)
	if !ValidFr(data) {
		t.Fatalf("expected generated Fr %s to be valid", v.String())
	}
	data = [32]byte{}
	if !ValidFr(data) {
		t.Fatal("expected zero to be valid")
	}
}

func TestFrDeserialize(t *testing.T) {
	x := []byte{254, 150, 120, 115, 187, 53, 205, 169, 13, 17, 244, 206, 123, 70, 62, 20, 47, 140, 1, 89, 50, 112, 93, 1, 211, 223, 149, 181, 189, 201, 46, 36}
	var a Fr
	FrFromBytes(&a, x)
	y := FrToBytes(&a)

	var b Fr
	FrFromBytes(&b, y)

	if !FrEqual(&a, &b) {
		t.Error("a")
	}
}
