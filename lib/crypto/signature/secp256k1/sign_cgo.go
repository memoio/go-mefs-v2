//go:build cgo
// +build cgo

package secp256k1

import (
	secp256k1 "github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// Sign signs the given message, which must be 32 bytes long.
func Sign(sk, msg []byte) ([]byte, error) {
	return secp256k1.Sign(msg, sk)
}

// Verify checks the given signature and returns true if it is valid.
func Verify(pk, msg, signature []byte) bool {
	if len(signature) == SignatureSize {
		signature = signature[:64]
	}
	return secp256k1.VerifySignature(pk[:], msg, signature)
}

// EcRecover recovers the public key from a message, signature pair.
func EcRecover(msg, signature []byte) ([]byte, error) {
	return secp256k1.RecoverPubkey(msg, signature)
}
