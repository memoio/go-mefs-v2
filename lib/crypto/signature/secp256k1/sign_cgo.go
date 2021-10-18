// +build cgo

package secp256k1

import (
	secp256k1 "github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// Sign signs the given message, which must be 32 bytes long.
func Secp256K1Sign(sk, msg []byte) ([]byte, error) {
	return secp256k1.Sign(msg, sk)
}

// Verify checks the given signature and returns true if it is valid.
func Secp256K1Verify(pk, msg, signature []byte) bool {
	return secp256k1.VerifySignature(pk[:], msg, signature)
}

// EcRecover recovers the public key from a message, signature pair.
func Secp256K1EcRecover(msg, signature []byte) ([]byte, error) {
	return secp256k1.RecoverPubkey(msg, signature)
}
