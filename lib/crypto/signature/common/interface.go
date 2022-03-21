package common

import (
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/types"
)

const (
	MsgBytes = 32
)

var (
	ErrBadKeyType    = xerrors.New("invalid or unsupported key type")
	ErrBadSign       = xerrors.New("invalid signature")
	ErrBadPrivateKey = xerrors.New("invalid private key")
	ErrBadPublickKey = xerrors.New("invalid public key")
)

// Key represents a crypto key that can be compared to another key
type Key interface {
	// Equals checks whether two PubKeys are the same
	Equals(Key) bool

	// Raw
	Raw() ([]byte, error)

	// Type returns the protobuf key type.
	Type() types.KeyType
}

// PrivKey represents a private key that can be used to generate a public key and sign data
type PrivKey interface {
	Key

	// Cryptographically sign the given bytes
	Sign([]byte) ([]byte, error)

	// Return a public key paired with this private key
	GetPublic() PubKey

	Deserialize([]byte) error
}

// PubKey is a public key that can be used to verifiy data signed with the corresponding private key
type PubKey interface {
	Key

	CompressedByte() ([]byte, error)
	// Verify that 'sig' is the signed hash of 'data'
	Verify(data []byte, sig []byte) (bool, error)

	Deserialize([]byte) error
}
