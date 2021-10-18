package bls

import (
	"crypto/subtle"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12-381"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
)

var _ sig_common.PrivKey = (*PrivateKey)(nil)
var _ sig_common.PubKey = (*PublicKey)(nil)

type PrivateKey struct {
	*PublicKey
	secretKey []byte
}

type PublicKey struct {
	pubKey []byte
}

// GenerateKey generates a new Secp256k1 private and public key pair
func GenerateKey() (sig_common.PrivKey, sig_common.PubKey, error) {
	pub := &PublicKey{}

	priv := &PrivateKey{
		PublicKey: pub,
	}

	priv.secretKey, _ = bls.GenerateKey()
	pub.pubKey, _ = bls.PublicKey(priv.secretKey)

	return priv, pub, nil
}

// Equals compares two private keys
func (k *PrivateKey) Equals(o sig_common.Key) bool {
	if o.Type() != sig_common.BLS {
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
func (k *PrivateKey) Type() sig_common.KeyType {
	return sig_common.BLS
}

// Raw returns the bytes of the key
func (k *PrivateKey) Raw() ([]byte, error) {
	if len(k.secretKey) != bls.SecretKeySize {
		return nil, sig_common.ErrBadPrivateKey
	}
	return k.secretKey, nil
}

// Sign returns a signature from input data
func (k *PrivateKey) Sign(data []byte) ([]byte, error) {
	sk, err := k.Raw()
	if err != nil {
		return nil, err
	}

	sig, err := bls.Sign(sk, data[:])
	if err != nil {
		return nil, err
	}
	return sig, nil
}

// GetPublic returns a public key
func (k *PrivateKey) GetPublic() sig_common.PubKey {
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
		return sig_common.ErrBadPrivateKey
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
func (k *PublicKey) Equals(o sig_common.Key) bool {
	if o.Type() != sig_common.BLS {
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
func (k *PublicKey) Type() sig_common.KeyType {
	return sig_common.BLS
}

// Raw returns the bytes of the key
func (k *PublicKey) Raw() ([]byte, error) {
	if len(k.pubKey) != bls.PublicKeySize {
		return nil, sig_common.ErrBadPrivateKey
	}

	return k.pubKey, nil
}

func (k *PublicKey) CompressedByte() ([]byte, error) {
	return k.Raw()
}

// GetPublic returns a public key
func (k *PublicKey) Deserialize(data []byte) error {
	if len(data) != bls.PublicKeySize {
		return sig_common.ErrBadPrivateKey
	}
	k.pubKey = data
	return nil
}

// Verify compares a signature against the input data
func (k *PublicKey) Verify(msg, sig []byte) (bool, error) {
	pubBytes, err := k.Raw()
	if err != nil {
		return false, err
	}

	if len(sig) != bls.SignatureSize {
		return false, sig_common.ErrBadSign
	}

	err = bls.Verify(pubBytes, msg, sig)
	if err != nil {
		return false, err
	}

	return true, nil
}
