package bls

import (
	"testing"
)

func TestSign(t *testing.T) {
	sk, err := GenerateKey()
	if err != nil {
		panic(err)
	}

	msg := []byte{134, 87, 163, 76, 148, 138, 55, 228, 171, 85, 80, 116, 242, 13, 169, 151, 167, 6, 219, 183, 108, 254, 214, 99, 184, 231, 210, 201, 39, 69, 184, 188, 105, 194, 22, 32, 9, 57, 220, 81, 82, 164, 97, 236, 201, 116, 2, 83}

	sig, err := Sign(sk, msg)
	if err != nil {
		panic(err)
	}

	pk, err := PublicKey(sk)
	if err != nil {
		panic(err)
	}

	err = Verify(pk, msg, sig)
	if err != nil {
		panic(err)
	}
}
