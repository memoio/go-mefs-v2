package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"io"

	"github.com/btcsuite/btcd/btcec"
	secp256k1 "github.com/memoio/go-mefs-v2/lib/crypto/secp256k1"
)

// Secp256k1PrivateKey is an Secp256k1 private key
type Secp256k1PrivateKey btcec.PrivateKey

// Secp256k1PublicKey is an Secp256k1 public key
type Secp256k1PublicKey btcec.PublicKey

// GenerateKeyFromSeed generates a new key from the given reader.
func SecpGenerateKeyFromSeed(seed io.Reader) (PrivKey, PubKey, error) {
	key, err := ecdsa.GenerateKey(btcec.S256(), seed)
	if err != nil {
		return nil, nil, err
	}

	k := (*Secp256k1PrivateKey)(key)
	return k, k.GetPublic(), nil
}

// GenerateSecp256k1Key generates a new Secp256k1 private and public key pair
func SecpGenerateSecp256k1Key() (PrivKey, PubKey, error) {
	return SecpGenerateKeyFromSeed(rand.Reader)
}

func SecpKeyFromBytes(data []byte) (PrivKey, PubKey, error) {
	dlen := len(data)
	switch dlen {
	case secp256k1.SecretKeySize:
		key, _ := btcec.PrivKeyFromBytes(btcec.S256(), data)
		k := (*Secp256k1PrivateKey)(key)
		return k, k.GetPublic(), nil
	case secp256k1.PublicKeySize, secp256k1.PublicKeyCompressedSize:
		pubKey, err := btcec.ParsePubKey(data, btcec.S256())
		if err != nil {
			return nil, nil, err
		}

		k := (*Secp256k1PublicKey)(pubKey)
		return nil, k, nil
	default:
		return nil, nil, ErrBadKeyType
	}
}

// Equals compares two private keys
func (k *Secp256k1PrivateKey) Equals(o Key) bool {
	sk, ok := o.(*Secp256k1PrivateKey)
	if !ok {
		return cmp(k, o)
	}

	return k.GetPublic().Equals(sk.GetPublic())
}

// Type returns the private key type
func (k *Secp256k1PrivateKey) Type() KeyType {
	return Secp256k1
}

// Raw returns the bytes of the key
func (k *Secp256k1PrivateKey) Raw() ([]byte, error) {
	return (*btcec.PrivateKey)(k).Serialize(), nil
}

// Sign returns a signature from input data
func (k *Secp256k1PrivateKey) Sign(data []byte) ([]byte, error) {
	sk, _ := k.Raw()
	sig, err := secp256k1.Secp256K1Sign(sk, data[:])
	if err != nil {
		return nil, err
	}

	return sig, nil
}

// GetPublic returns a public key
func (k *Secp256k1PrivateKey) GetPublic() PubKey {
	return (*Secp256k1PublicKey)((*btcec.PrivateKey)(k).PubKey())
}

// Equals compares two public keys
func (k *Secp256k1PublicKey) Equals(o Key) bool {
	sk, ok := o.(*Secp256k1PublicKey)
	if !ok {
		return cmp(k, o)
	}

	return (*btcec.PublicKey)(k).IsEqual((*btcec.PublicKey)(sk))
}

// Type returns the public key type
func (k *Secp256k1PublicKey) Type() KeyType {
	return Secp256k1
}

// Raw returns the bytes of the key
func (k *Secp256k1PublicKey) Raw() ([]byte, error) {
	return (*btcec.PublicKey)(k).SerializeUncompressed(), nil
}

// 20 bytes address
func (k *Secp256k1PublicKey) CompressedByte() ([]byte, error) {
	return (*btcec.PublicKey)(k).SerializeCompressed(), nil
}

// Verify compares a signature against the input data
func (k *Secp256k1PublicKey) Verify(msg, signature []byte) (bool, error) {
	if len(signature) != secp256k1.SignatureSize {
		return false, ErrBadSign
	}

	rePub, err := secp256k1.Secp256K1EcRecover(msg, signature)
	if err != nil {
		return false, err
	}

	pubBytes := (*btcec.PublicKey)(k).SerializeUncompressed()

	if bytes.Equal(pubBytes, rePub) {
		return true, nil
	}

	return false, nil
}
