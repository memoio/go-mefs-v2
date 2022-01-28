//go:build !herumi
// +build !herumi

package bls

import (
	"bytes"
	"crypto/rand"

	kbls "github.com/kilic/bls12-381"
	"github.com/zeebo/blake3"
)

var kilicGroupOrder = new(kbls.Fr).FromBytes(kbls.NewG1().Q().Bytes())

// PublicKey returns the public key for this private key.
func PublicKey(privateKey []byte) ([]byte, error) {
	if len(privateKey) != SecretKeySize {
		return nil, errSecretKeySize
	}

	if bytes.Equal(zeroSecretKey, privateKey) {
		return nil, errZeroSecretKey
	}

	sk := new(kbls.Fr).FromBytes(privateKey)
	if sk.Cmp(kilicGroupOrder) != -1 {
		return nil, errInvalidSecretKey
	}

	g := kbls.NewG1()
	pk := g.New()
	g.MulScalar(pk, g.One(), sk)

	return g.ToCompressed(pk), nil
}

// Sign signs the given message, which must be 32 bytes long.
func Sign(privateKey, msg []byte) ([]byte, error) {
	if len(privateKey) != SecretKeySize {
		return nil, errSecretKeySize
	}

	if bytes.Equal(zeroSecretKey, privateKey) {
		return nil, errZeroSecretKey
	}

	sk := new(kbls.Fr).FromBytes(privateKey)
	if sk.Cmp(kilicGroupOrder) != -1 {
		return nil, errInvalidSecretKey
	}

	g := kbls.NewG2()
	M, err := g.HashToCurve(msg, dst)
	if err != nil {
		return nil, err
	}
	sign := g.New()
	g.MulScalar(sign, M, sk)

	return g.ToCompressed(sign), nil
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

	g := kbls.NewG1()
	pk, err := g.FromCompressed(publicKey)
	if err != nil {
		return err
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

	g2 := kbls.NewG2()
	sign, err := g2.FromCompressed(signature)
	if err != nil {
		return err
	}

	e := kbls.NewEngine()
	M, err := e.G2.HashToCurve(msg, dst)
	if err != nil {
		return err
	}
	e.AddPair(pk, M)
	e.AddPairInv(e.G1.One(), sign)
	ok := e.Check()
	if ok {
		return nil
	}

	return errInvalidSignature
}

func AggregateSignature(signatures ...[]byte) ([]byte, error) {
	if len(signatures) == 0 {
		return nil, errZeroPublicKey
	}

	g := kbls.NewG2()
	aggregated, err := g.FromCompressed(signatures[0])
	if err != nil {
		return nil, err
	}
	for i := 1; i < len(signatures); i++ {
		sig, err := g.FromCompressed(signatures[i])
		if err != nil {
			return nil, err
		}

		g.Add(aggregated, aggregated, sig)
	}
	return g.ToCompressed(aggregated), nil
}

func AggregatePublicKey(publicKeys ...[]byte) ([]byte, error) {
	if len(publicKeys) == 0 {
		return nil, errZeroPublicKey
	}

	g := kbls.NewG1()
	aggregated, err := g.FromCompressed(publicKeys[0])
	if err != nil {
		return nil, err
	}
	for i := 1; i < len(publicKeys); i++ {
		pk, err := g.FromCompressed(publicKeys[i])
		if err != nil {
			return nil, err
		}

		g.Add(aggregated, aggregated, pk)
	}
	return g.ToCompressed(aggregated), nil
}

// GenerateKeyFromSeed generates a new key from the given reader.
func GenerateKeyFromSeed(seed []byte) ([]byte, error) {
	sk := new(kbls.Fr).FromBytes(blake3.New().Sum(seed))

	return sk.ToBytes(), nil
}

// GenerateKey creates a new key using secure randomness from crypto.rand.
func GenerateKey() ([]byte, error) {
	sk, err := new(kbls.Fr).Rand(rand.Reader)
	if err != nil {
		return nil, err
	}
	return sk.ToBytes(), nil
}
