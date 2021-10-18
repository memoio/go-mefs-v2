package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"io"
)

type KeyType = byte

const (
	// RSA is an enum for the supported RSA key type
	RSA = iota
	// Secp256k1 is an enum for the supported Secp256k1 key type
	Secp256k1
	BLS
)

const (
	MsgBytes = 32
)

var (
	// ErrBadKeyType is returned when a key is not supported
	ErrBadKeyType    = errors.New("invalid or unsupported key type")
	ErrBadSign       = errors.New("invalid signature")
	ErrBadMsg        = errors.New("invalid message")
	ErrBadPrivateKey = errors.New("invalid private key")
	ErrBadPublickKey = errors.New("invalid public key")
)

// Key represents a crypto key that can be compared to another key
type Key interface {
	// Equals checks whether two PubKeys are the same
	Equals(Key) bool

	// Raw
	Raw() ([]byte, error)

	// Type returns the protobuf key type.
	Type() KeyType
}

// PrivKey represents a private key that can be used to generate a public key and sign data
type PrivKey interface {
	Key

	// Cryptographically sign the given bytes
	Sign([]byte) ([]byte, error)

	// Return a public key paired with this private key
	GetPublic() PubKey
}

// PubKey is a public key that can be used to verifiy data signed with the corresponding private key
type PubKey interface {
	Key

	CompressedByte() ([]byte, error)
	// Verify that 'sig' is the signed hash of 'data'
	Verify(data []byte, sig []byte) (bool, error)
}

// GenerateKeyFromSeed generates a new key from the given reader.
func GenerateKeyFromSeed(typ KeyType, seed io.Reader) (PrivKey, PubKey, error) {
	switch typ {
	case Secp256k1:
		return SecpGenerateKeyFromSeed(seed)
	case BLS:
		return BLSGenerateKeyFromSeed(seed)
	default:
		return nil, nil, ErrBadKeyType
	}
}

// GenerateKey generates a new Secp256k1 private and public key pair
func GenerateKey(typ KeyType) (PrivKey, PubKey, error) {
	return GenerateKeyFromSeed(typ, rand.Reader)
}

// Serialize
func Serialize(k Key) ([]byte, error) {
	keyByte, _ := k.Raw()
	buf := make([]byte, 1+len(keyByte))
	buf[0] = k.Type()
	copy(buf[1:], keyByte)
	return buf, nil
}

// Deserialize
func Deserialize(data []byte) (PrivKey, PubKey, error) {
	dlen := len(data)
	if dlen <= 1 {
		return nil, nil, ErrBadKeyType
	}

	switch data[0] {
	case Secp256k1:
		return SecpKeyFromBytes(data[1:])
	case BLS:
		return BLSKeyFromByte(data[1:])
	default:
		return nil, nil, ErrBadKeyType
	}
}

func cmp(k1, k2 Key) bool {
	if k1.Type() != k2.Type() {
		return false
	}

	a, err := k1.Raw()
	if err != nil {
		return false
	}
	b, err := k2.Raw()
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}
