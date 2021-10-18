package crypto

import (
	"crypto/rand"
	"io"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12-381"
)

type BLSPrivateKey struct {
	secretKey []byte
	pubKey    []byte
}
type BLSPublicKey struct {
	pubKey []byte
}

// GenerateKeyFromSeed generates a new key from the given reader.
func BLSGenerateKeyFromSeed(seed io.Reader) (PrivKey, PubKey, error) {
	secretKey, _ := bls.GenerateKey()
	pubKey, _ := bls.PublicKey(secretKey)

	privk := &BLSPrivateKey{
		secretKey: secretKey,
		pubKey:    pubKey,
	}

	pubK := &BLSPublicKey{
		pubKey: pubKey,
	}

	return privk, pubK, nil
}

// GenerateSecp256k1Key generates a new Secp256k1 private and public key pair
func BLSGenerateSecp256k1Key() (PrivKey, PubKey, error) {
	return BLSGenerateKeyFromSeed(rand.Reader)
}

func BLSKeyFromByte(data []byte) (PrivKey, PubKey, error) {
	dlen := len(data)
	switch dlen {
	case bls.SecretKeySize:
		pubKey, err := bls.PublicKey(data)
		if err != nil {
			return nil, nil, err
		}
		return &BLSPrivateKey{data, pubKey}, &BLSPublicKey{pubKey}, nil

	case bls.PublicKeySize:
		return nil, &BLSPublicKey{data}, nil
	default:
		return nil, nil, ErrBadKeyType
	}
}

// Equals compares two private keys
func (k *BLSPrivateKey) Equals(o Key) bool {
	return cmp(k, o)
}

// Type returns the private key type
func (k *BLSPrivateKey) Type() KeyType {
	return BLS
}

// Raw returns the bytes of the key
func (k *BLSPrivateKey) Raw() ([]byte, error) {
	return k.secretKey, nil
}

// Sign returns a signature from input data
func (k *BLSPrivateKey) Sign(data []byte) ([]byte, error) {
	sig, err := bls.Sign(k.secretKey, data[:])
	if err != nil {
		return nil, err
	}

	return sig, nil
}

// GetPublic returns a public key
func (k *BLSPrivateKey) GetPublic() PubKey {
	return &BLSPublicKey{k.pubKey}
}

// Equals compares two public keys
func (k *BLSPublicKey) Equals(o Key) bool {
	return cmp(k, o)
}

// Type returns the public key type
func (k *BLSPublicKey) Type() KeyType {
	return BLS
}

// Raw returns the bytes of the key
func (k *BLSPublicKey) Raw() ([]byte, error) {
	return k.pubKey, nil
}

func (k *BLSPublicKey) CompressedByte() ([]byte, error) {
	return k.Raw()
}

// Verify compares a signature against the input data
func (k *BLSPublicKey) Verify(msg, signature []byte) (bool, error) {
	if len(signature) != bls.SignatureSize {
		return false, ErrBadSign
	}

	err := bls.Verify(k.pubKey, msg, signature)
	if err != nil {
		return false, err
	}

	return true, nil
}
