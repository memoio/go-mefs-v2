// +build !cgo

package secp256k1

import (
	"math/big"

	"github.com/btcsuite/btcd/btcec"
)

var (
	secp256k1N, _  = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1halfN = new(big.Int).Div(secp256k1N, big.NewInt(2))
)

// Sign signs the given message, which must be 32 bytes long.
func Secp256K1Sign(sk, msg []byte) ([]byte, error) {
	key, _ := btcec.PrivKeyFromBytes(btcec.S256(), sk[:])

	sig, err := key.Sign(msg)
	if err != nil {
		return nil, err
	}

	return sig.Serialize(), nil
}

// Verify checks the given signature and returns true if it is valid.
func Secp256K1Verify(pk, msg, signature []byte) bool {
	if len(signature) >= SignatureSize-1 {
		// Drop the V (1byte) in [R | S | V] style signatures.
		// The V (1byte) is the recovery bit and is not apart of the signature verification.
		sig := &btcec.Signature{R: new(big.Int).SetBytes(signature[:32]), S: new(big.Int).SetBytes(signature[32 : SignatureSize-1])}
		key, err := btcec.ParsePubKey(pk, btcec.S256())
		if err != nil {
			return false
		}
		// Reject malleable signatures. libsecp256k1 does this check but btcec doesn't.
		if sig.S.Cmp(secp256k1halfN) > 0 {
			return false
		}
		return sig.Verify(msg, key)
	}

	return false
}

// EcRecover recovers the public key from a message, signature pair.
func Secp256K1EcRecover(msg, sig []byte) ([]byte, error) {
	btcsig := make([]byte, SignatureSize)
	btcsig[0] = sig[64] + 27
	copy(btcsig[1:], sig)

	pub, _, err := btcec.RecoverCompact(btcec.S256(), btcsig, msg)

	bytes := (*btcec.PublicKey)(pub).SerializeUncompressed()
	return bytes, err
}
