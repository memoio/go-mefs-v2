package utils

import "testing"

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
