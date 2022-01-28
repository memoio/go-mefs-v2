package bls

import (
	"crypto/subtle"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
)

var _ common.PrivKey = (*PrivateKey)(nil)
var _ common.PubKey = (*PublicKey)(nil)

type PrivateKey struct {
	*PublicKey
	secretKey []byte
}

type PublicKey struct {
	pubKey []byte
}

// GenerateKey generates a new Secp256k1 private and public key pair
func GenerateKey() (common.PrivKey, error) {
	pub := &PublicKey{}

	priv := &PrivateKey{
		PublicKey: pub,
	}

	priv.secretKey, _ = bls.GenerateKey()
	pub.pubKey, _ = bls.PublicKey(priv.secretKey)

	return priv, nil
}

// Equals compares two private keys
func (k *PrivateKey) Equals(o common.Key) bool {
	if o.Type() != types.BLS {
		return false
	}

	a, err := k.Raw()
	if err != nil {
		return false
	}
	b, err := o.Raw()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

// Type returns the private key type
func (k *PrivateKey) Type() types.KeyType {
	return types.BLS
}

// Raw returns the bytes of the key
func (k *PrivateKey) Raw() ([]byte, error) {
	if len(k.secretKey) != bls.SecretKeySize {
		return nil, common.ErrBadPrivateKey
	}
	return k.secretKey, nil
}

// Sign returns a signature from input data
func (k *PrivateKey) Sign(data []byte) ([]byte, error) {
	sk, err := k.Raw()
	if err != nil {
		return nil, err
	}

	if len(data) != 32 {
		msg := blake3.Sum256(data)
		data = msg[:]
	}

	sig, err := bls.Sign(sk, data)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

// GetPublic returns a public key
func (k *PrivateKey) GetPublic() common.PubKey {
	if k.PublicKey != nil {
		return k.PublicKey
	}
	pubkey, err := bls.PublicKey(k.secretKey)
	if err != nil {
		return nil
	}
	k.PublicKey = &PublicKey{pubkey}
	return k.PublicKey
}

// GetPublic returns a public key
func (k *PrivateKey) Deserialize(data []byte) error {
	if len(data) != bls.SecretKeySize {
		return common.ErrBadPrivateKey
	}
	pubKey, err := bls.PublicKey(data)
	if err != nil {
		return nil
	}

	k.secretKey = data
	k.PublicKey = &PublicKey{pubKey}
	return nil
}

// Equals compares two public keys
func (k *PublicKey) Equals(o common.Key) bool {
	if o.Type() != types.BLS {
		return false
	}

	a, err := k.Raw()
	if err != nil {
		return false
	}
	b, err := o.Raw()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

// Type returns the public key type
func (k *PublicKey) Type() types.KeyType {
	return types.BLS
}

// Raw returns the bytes of the key
func (k *PublicKey) Raw() ([]byte, error) {
	if len(k.pubKey) != bls.PublicKeySize {
		return nil, common.ErrBadPrivateKey
	}

	return k.pubKey, nil
}

func (k *PublicKey) CompressedByte() ([]byte, error) {
	return k.Raw()
}

// GetPublic returns a public key
func (k *PublicKey) Deserialize(data []byte) error {
	if len(data) != bls.PublicKeySize {
		return common.ErrBadPrivateKey
	}
	k.pubKey = data
	return nil
}

// Verify compares a signature against the input data
func (k *PublicKey) Verify(data, sig []byte) (bool, error) {
	pubBytes, err := k.Raw()
	if err != nil {
		return false, err
	}

	if len(sig) != bls.SignatureSize {
		return false, common.ErrBadSign
	}

	if len(data) != 32 {
		msg := blake3.Sum256(data)
		data = msg[:]
	}

	err = bls.Verify(pubBytes, data, sig)
	if err != nil {
		return false, err
	}

	return true, nil
}
