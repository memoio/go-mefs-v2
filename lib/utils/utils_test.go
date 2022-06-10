package utils

import (
	"sort"
	"testing"
)

func TestUtils(t *testing.T) {
	pros := []uint64{1, 4, 5, 7}
	t.Log(pros)
	DisorderUint(pros)
	t.Log(pros)
	t.Fatal("end")
}

func TestReliability(t *testing.T) {
	res := CalReliabilty(4, 3, 0.9)
	t.Fatal(res)
}

func TestBino(t *testing.T) {
	res := Binomial(50, 50)
	t.Fatal(res)
}

func TestSort(t *testing.T) {

	sLen := 3

	kis := []uint64{8, 18, 4}
	ksigns := [][]byte{[]byte("a"), []byte("b"), []byte("c")}

	type pks struct {
		ki uint64
		s  []byte
	}

	ks := make([]*pks, sLen)

	for i := 0; i < sLen; i++ {
		ks[i] = &pks{
			ki: kis[i],
			s:  ksigns[i],
		}
	}

	sort.Slice(ks, func(i, j int) bool {
		return ks[i].ki < ks[j].ki
	})

	nkis := make([]uint64, sLen)
	nksigns := make([][]byte, sLen)
	for i := 0; i < sLen; i++ {
		nkis[i] = ks[i].ki
		nksigns[i] = ks[i].s
	}
	t.Fatal(nkis, nksigns)
}
