//go:build herumi
// +build herumi

package bls

import (
	"bytes"

	hbls "github.com/herumi/bls-eth-go-binary/bls"
	"github.com/zeebo/blake3"
)

// PublicKey returns the public key for this private key.
func PublicKey(privateKey []byte) ([]byte, error) {
	if len(privateKey) != SecretKeySize {
		return nil, errSecretKeySize
	}

	if bytes.Equal(zeroSecretKey, privateKey) {
		return nil, errZeroSecretKey
	}

	sk := new(hbls.SecretKey)
	err := sk.Deserialize(privateKey)
	if err != nil {
		return nil, err
	}

	pk, err := sk.GetSafePublicKey()
	if err != nil {
		return nil, err
	}
	return pk.Serialize(), nil
}

// Sign signs the given message, which must be 32 bytes long.
func Sign(privateKey, msg []byte) ([]byte, error) {
	if len(privateKey) != SecretKeySize {
		return nil, errSecretKeySize
	}

	if bytes.Equal(zeroSecretKey, privateKey) {
		return nil, errZeroSecretKey
	}

	sk := new(hbls.SecretKey)
	err := sk.Deserialize(privateKey)
	if err != nil {
		return nil, err
	}

	sign := sk.SignByte(msg)

	return sign.Serialize(), nil
}

// Equals compares two private key for equality and returns true if they are the same.
func Equals(sk, other []byte) bool {
	return bytes.Equal(sk, other)
}

// Verify checks the given signature and returns true if it is valid.
func Verify(publicKey, msg, signature []byte) error {
	if len(publicKey) != PublicKeySize {
		return errPublicKeySize
	}

	if bytes.Equal(zeroPublicKey, publicKey) {
		return errZeroPublicKey
	}

	if bytes.Equal(infinitePublicKey, publicKey) {
		return errInfinitePublicKey
	}

	pk := new(hbls.PublicKey)
	err := pk.Deserialize(publicKey)
	if err != nil {
		return err
	}

	if !pk.IsValidOrder() {
		return errInvalidPublicKey
	}

	if pk.IsZero() {
		return errZeroPublicKey
	}

	if len(signature) != SignatureSize {
		return errSignatureSize
	}
	if bytes.Equal(zeroSignature, signature) {
		return errZeroSignature
	}
	if bytes.Equal(infiniteSignature, signature) {
		return errInfiniteSignature
	}

	sig := new(hbls.Sign)
	err := sig.Deserialize(signature)
	if err != nil {
		return err
	}

	ok := sig.VerifyByte(pk, msg)
	if ok {
		return nil
	}

	return errInvalidSignature
}

func AggregateSignature(signatures ...[]byte) ([]byte, error) {
	size := len(signatures)
	signs := make([]hbls.Sign, size)
	for i := 0; i < size; i++ {
		err := signs[i].Deserialize(signatures[i])
		if err != nil {
			return nil, err
		}
	}

	aggregatedSignature := new(hbls.Sign)
	aggregatedSignature.Aggregate(signs)
	return aggregatedSignature.Serialize(), nil
}

func AggregatePublicKey(publicKeys ...[]byte) ([]byte, error) {
	size := len(publicKeys)
	aggregatedPublicKey := new(hbls.PublicKey)
	for i := 0; i < size; i++ {
		pk := new(hbls.PublicKey)
		err := pk.Deserialize(publicKeys[i])
		if err != nil {
			return nil, err
		}

		aggregatedPublicKey.Add(pk)
	}
	return aggregatedPublicKey.Serialize(), nil
}

// GenerateKeyFromSeed generates a new key from the given reader.
func GenerateKeyFromSeed(seed []byte) ([]byte, error) {
	sk := new(hbls.SecretKey)
	blake3
	sk.SetLittleEndian(blake3.New().Sum(seed))

	return sk.Serialize(), nil
}

// GenerateKey creates a new key using secure randomness from crypto.rand.
func GenerateKey() ([]byte, error) {
	sk := new(hbls.SecretKey)
	sk.SetByCSPRNG()
	return sk.Serialize(), nil
}
