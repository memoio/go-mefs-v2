//go:build !cgo
// +build !cgo

package secp256k1

import (
	"math/big"

	"github.com/btcsuite/btcd/btcec"
	"golang.org/x/xerrors"
)

var (
	secp256k1N, _  = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1halfN = new(big.Int).Div(secp256k1N, big.NewInt(2))
)

// Sign signs the given message, which must be 32 bytes long.
func Sign(sk, msg []byte) ([]byte, error) {
	if len(msg) != 32 {
		return nil, xerrors.Errorf("hash is required to be exactly 32 bytes (%d)", len(msg))
	}
	privkey, _ := btcec.PrivKeyFromBytes(btcec.S256(), sk[:])

	sig, err := btcec.SignCompact(btcec.S256(), (*btcec.PrivateKey)(privkey), msg, false)
	if err != nil {
		return nil, err
	}

	// Convert to Ethereum signature format with 'recovery id' v at the end.
	v := sig[0] - 27
	copy(sig, sig[1:])
	sig[64] = v

	return sig, nil
}

// Verify checks the given signature and returns true if it is valid.
func Verify(pk, msg, signature []byte) bool {
	if len(signature) == SignatureSize {
		signature = signature[:64]
	}
	if len(signature) != 64 {
		return false
	}

	// Drop the V (1byte) in [R | S | V] style signatures.
	// The V (1byte) is the recovery bit and is not apart of the signature verification.
	sig := &btcec.Signature{R: new(big.Int).SetBytes(signature[:32]), S: new(big.Int).SetBytes(signature[32 : SignatureSize-1])}
	pubKey, err := btcec.ParsePubKey(pk, btcec.S256())
	if err != nil {
		return false
	}
	// Reject malleable signatures. libsecp256k1 does this check but btcec doesn't.
	if sig.S.Cmp(secp256k1halfN) > 0 {
		return false
	}
	return sig.Verify(msg, pubKey)
}

// EcRecover recovers the public key from a message, signature pair.
func EcRecover(msg, sig []byte) ([]byte, error) {
	btcsig := make([]byte, SignatureSize)
	btcsig[0] = sig[64] + 27
	copy(btcsig[1:], sig)

	pub, _, err := btcec.RecoverCompact(btcec.S256(), btcsig, msg)
	if err != nil {
		return nil, err
	}

	bytes := (*btcec.PublicKey)(pub).SerializeUncompressed()
	return bytes, err
}
