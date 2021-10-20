package secp256k1

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/subtle"

	"github.com/btcsuite/btcd/btcec"
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

	key, err := ecdsa.GenerateKey(btcec.S256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	priv.secretKey = (*btcec.PrivateKey)(key).Serialize()

	pk := (*btcec.PublicKey)(&key.PublicKey)
	pub.pubKey = pk.SerializeUncompressed()

	return priv, pub, nil
}

// Equals compares two private keys
func (k *PrivateKey) Equals(o sig_common.Key) bool {
	if o.Type() != sig_common.Secp256k1 {
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
	return sig_common.Secp256k1
}

// Raw returns the bytes of the key
func (k *PrivateKey) Raw() ([]byte, error) {
	if len(k.secretKey) != SecretKeySize {
		return nil, sig_common.ErrBadPrivateKey
	}
	return k.secretKey, nil
}

// Sign returns a signature from input data
func (k *PrivateKey) Sign(msg []byte) ([]byte, error) {
	sk, err := k.Raw()
	if err != nil {
		return nil, err
	}
	sig, err := Secp256K1Sign(sk, msg)
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
	privkey, err := btcec.PrivKeyFromBytes(btcec.S256(), k.secretKey)
	if err != nil {
		return nil
	}

	pubkey := privkey.PublicKey
	pk := (*btcec.PublicKey)(&pubkey)

	k.PublicKey = &PublicKey{pk.SerializeUncompressed()}
	return k.PublicKey
}

// GetPublic returns a public key
func (k *PrivateKey) Deserialize(data []byte) error {
	if len(data) != SecretKeySize {
		return sig_common.ErrBadPrivateKey
	}
	k.secretKey = data

	_, pubkey := btcec.PrivKeyFromBytes(btcec.S256(), k.secretKey)

	pk := (*btcec.PublicKey)(pubkey)
	k.PublicKey = &PublicKey{pk.SerializeUncompressed()}

	return nil
}

// Equals compares two public keys
func (k *PublicKey) Equals(o sig_common.Key) bool {
	if o.Type() != sig_common.Secp256k1 {
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
	return sig_common.Secp256k1
}

// Raw returns the bytes of the key
func (k *PublicKey) Raw() ([]byte, error) {
	if len(k.pubKey) != PublicKeySize {
		return nil, sig_common.ErrBadPrivateKey
	}

	return k.pubKey, nil
}

func (k *PublicKey) CompressedByte() ([]byte, error) {
	key, err := btcec.ParsePubKey(k.pubKey, btcec.S256())
	if err != nil {
		return nil, err
	}
	return key.SerializeCompressed(), nil
}

// GetPublic returns a public key
func (k *PublicKey) Deserialize(data []byte) error {
	if len(data) == PublicKeySize {
		k.pubKey = data
		return nil
	} else if len(data) == PublicKeyCompressedSize {
		key, err := btcec.ParsePubKey(data, btcec.S256())
		if err != nil {
			return err
		}
		k.pubKey = key.SerializeUncompressed()
		return nil
	}

	return sig_common.ErrBadPublickKey
}

// Verify compares a signature against the input data
func (k *PublicKey) Verify(msg, sig []byte) (bool, error) {
	if len(sig) != SignatureSize {
		return false, sig_common.ErrBadSign
	}

	pubBytes, err := k.Raw()
	if err != nil {
		return false, err
	}

	rePub, err := Secp256K1EcRecover(msg, sig)
	if err != nil {
		return false, err
	}

	if bytes.Equal(pubBytes, rePub) {
		return true, nil
	}
	return false, nil
}
