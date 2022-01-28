package utils

import "testing"

func TestUtils(t *testing.T) {
	pros := []uint64{1, 4, 5, 7}
	t.Log(pros)
	DisorderUint(pros)
	t.Log(pros)
	t.Fatal("end")
}
