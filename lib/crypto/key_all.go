package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"

	"github.com/btcsuite/btcd/btcec"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12-381"
	secp256k1 "github.com/memoio/go-mefs-v2/lib/crypto/secp256k1"
)

type PrivateKey struct {
	*PublicKey
	secretKey []byte
}
type PublicKey struct {
	keyType KeyType
	pubKey  []byte
}

// GenerateKey generates a new Secp256k1 private and public key pair
func GenerateKeyForAll(typ KeyType) (PrivKey, PubKey, error) {
	pub := &PublicKey{
		keyType: typ,
	}

	priv := &PrivateKey{
		PublicKey: pub,
	}

	switch typ {
	case Secp256k1:
		key, err := ecdsa.GenerateKey(btcec.S256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}

		priv.secretKey = (*btcec.PrivateKey)(key).Serialize()

		pk := (*btcec.PublicKey)(&key.PublicKey)
		pub.pubKey = pk.SerializeUncompressed()
	case BLS:
		priv.secretKey, _ = bls.GenerateKey()
		pub.pubKey, _ = bls.PublicKey(priv.secretKey)
	default:
		return nil, nil, ErrBadKeyType
	}

	return priv, pub, nil
}

func Unmarshal(data []byte) (PrivKey, PubKey, error) {
	dlen := len(data)
	if dlen <= 1 {
		return nil, nil, ErrBadKeyType
	}

	pub := &PublicKey{
		keyType: data[0],
	}

	priv := &PrivateKey{
		PublicKey: pub,
	}

	switch data[0] {
	case Secp256k1:
		switch dlen {
		case 1 + secp256k1.SecretKeySize:
			sk, pk := btcec.PrivKeyFromBytes(btcec.S256(), data[1:])
			priv.secretKey = sk.Serialize()
			pub.pubKey = pk.SerializeUncompressed()

		case 1 + secp256k1.PublicKeySize, 1 + secp256k1.PublicKeyCompressedSize:
			pubKey, err := btcec.ParsePubKey(data[1:], btcec.S256())
			if err != nil {
				return nil, nil, err
			}

			pub.pubKey = pubKey.SerializeUncompressed()

			return nil, pub, nil
		default:
			return nil, nil, ErrBadPrivateKey
		}
	case BLS:
		switch dlen {
		case 1 + bls.SecretKeySize:
			priv.secretKey = data[1:]
			pubKey, err := bls.PublicKey(priv.secretKey)
			if err != nil {
				return nil, nil, err
			}

			pub.pubKey = pubKey

		case bls.PublicKeySize:
			pub.pubKey = data[1:]
			return nil, pub, nil
		default:
			return nil, nil, ErrBadPrivateKey
		}
	default:
		return nil, nil, ErrBadKeyType
	}
	return priv, pub, nil
}

// Marshal
func Marshal(k Key) ([]byte, error) {
	keyByte, _ := k.Raw()
	buf := make([]byte, 1+len(keyByte))
	buf[0] = k.Type()
	copy(buf[1:], keyByte)
	return buf, nil
}

// Equals compares two private keys
func (k *PrivateKey) Equals(o Key) bool {
	return cmp(k, o)
}

// Type returns the private key type
func (k *PrivateKey) Type() KeyType {
	return k.keyType
}

// Raw returns the bytes of the key
func (k *PrivateKey) Raw() ([]byte, error) {
	switch k.Type() {
	case Secp256k1:
		if len(k.secretKey) != secp256k1.SecretKeySize {
			return nil, ErrBadPrivateKey
		}
	case BLS:
		if len(k.secretKey) != bls.SecretKeySize {
			return nil, ErrBadPrivateKey
		}
	default:
		return nil, ErrBadKeyType

	}
	return k.secretKey, nil
}

// Sign returns a signature from input data
func (k *PrivateKey) Sign(data []byte) ([]byte, error) {
	switch k.Type() {
	case Secp256k1:
		sk, err := k.Raw()
		if err != nil {
			return nil, err
		}
		sig, err := secp256k1.Secp256K1Sign(sk, data[:])
		if err != nil {
			return nil, err
		}

		return sig, nil
	case BLS:
		sk, err := k.Raw()
		if err != nil {
			return nil, err
		}

		sig, err := bls.Sign(sk, data[:])
		if err != nil {
			return nil, err
		}
		return sig, nil
	default:
		return nil, ErrBadSign
	}
}

// GetPublic returns a public key
func (k *PrivateKey) GetPublic() PubKey {
	return k.PublicKey
}

// Equals compares two public keys
func (k *PublicKey) Equals(o Key) bool {
	return cmp(k, o)
}

// Type returns the public key type
func (k *PublicKey) Type() KeyType {
	return k.keyType
}

// Raw returns the bytes of the key
func (k *PublicKey) Raw() ([]byte, error) {
	switch k.Type() {
	case Secp256k1:
		if len(k.pubKey) != secp256k1.PublicKeySize {
			return nil, ErrBadPrivateKey
		}
	case BLS:
		if len(k.pubKey) != bls.PublicKeySize {
			return nil, ErrBadPrivateKey
		}
	default:
		return nil, ErrBadKeyType

	}
	return k.pubKey, nil
}

func (k *PublicKey) CompressedByte() ([]byte, error) {
	switch k.Type() {
	case Secp256k1:
		pbyte, err := k.Raw()
		if err != nil {
			return nil, err
		}

		buf := make([]byte, btcec.PubKeyBytesLenCompressed)
		buf[0] = pbyte[33] | 2
		copy(buf[1:], pbyte[1:33])
		return buf, nil
	case BLS:
		return k.Raw()
	default:
		return nil, ErrBadKeyType
	}

}

// Verify compares a signature against the input data
func (k *PublicKey) Verify(msg, signature []byte) (bool, error) {
	pubBytes, err := k.Raw()
	if err != nil {
		return false, err
	}

	switch k.Type() {
	case Secp256k1:
		if len(signature) != secp256k1.SignatureSize {
			return false, ErrBadSign
		}

		rePub, err := secp256k1.Secp256K1EcRecover(msg, signature)
		if err != nil {
			return false, err
		}

		if bytes.Equal(pubBytes, rePub) {
			return true, nil
		}
	case BLS:
		if len(signature) != bls.SignatureSize {
			return false, ErrBadSign
		}

		err := bls.Verify(pubBytes, msg, signature)
		if err != nil {
			return false, err
		}

		return true, nil
	default:
		return false, ErrBadKeyType
	}

	return false, nil
}
