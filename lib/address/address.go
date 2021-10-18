package address

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/minio/blake2b-simd"
	b58 "github.com/mr-tron/base58/base58"
	"golang.org/x/crypto/sha3"
)

var (
	// ErrUnknownAddrType is returned when encountering an unknown protocol in an address.
	ErrUnknownAddrType = errors.New("unknown address type")
	// ErrInvalidPayload is returned when encountering an invalid address payload.
	ErrInvalidPayload = errors.New("invalid address payload")
	// ErrInvalidLength is returned when encountering an address of invalid length.
	ErrInvalidLength = errors.New("invalid address length")
	// ErrInvalidChecksum is returned when encountering an invalid address checksum.
	ErrInvalidChecksum = errors.New("invalid address checksum")
)

// UndefAddressString is the string used to represent an empty address when encoded to a string.
var UndefAddressString = "<empty>"

// PayloadHashLength defines the hash length taken over addresses using the SECP256K1 protocols.
const PayloadHashLength = 20

// ChecksumHashLength defines the hash length used for calculating address checksums.
const ChecksumHashLength = 1

// MaxAddressStringLength is the max length of an address encoded as a string
// it include the network prefx, protocol, and bls publickey
const MaxAddressStringLength = 2 + 84

// BlsPublicKeyBytes is the length of a BLS public key
const BlsPublicKeyBytes = 48

var payloadHashConfig = &blake2b.Config{Size: PayloadHashLength}
var checksumHashConfig = &blake2b.Config{Size: ChecksumHashLength}

// Address is the go type that represents an address.
// Type + publickey or hash(public)
type Address struct{ str string }

// Undef is the type that represents an undefined address.
var Undef = Address{}

const AddrPrefix = "M"

// Protocol returns the protocol used by the address.
func (a Address) Type() types.KeyType {
	if len(a.str) == 0 {
		return types.Unknown
	}
	return a.str[0]
}

// Payload returns the payload of the address.
func (a Address) Payload() []byte {
	if len(a.str) == 0 {
		return nil
	}
	return []byte(a.str[1:])
}

// Bytes returns the address as bytes.
func (a Address) Bytes() []byte {
	return []byte(a.str)
}

// String returns an address encoded as a string.
func (a Address) String() string {
	str, err := encode(a)
	if err != nil {
		panic(err) // I don't know if this one is okay
	}
	return str
}

// Empty returns true if the address is empty, false otherwise.
func (a Address) Empty() bool {
	return a == Undef
}

// NewSecp256k1Address returns an address using the SECP256K1 protocol.
func NewSecp256k1Address(pubkey []byte) (Address, error) {
	d := sha3.NewLegacyKeccak256()
	d.Write(pubkey[1:])
	return newAddress(types.SECP256K1, d.Sum(nil)[12:])
}

// convert chain address to local address
func ToSecp256k1Address(payload []byte) (Address, error) {
	return newAddress(types.SECP256K1, payload)
}

// NewBLSAddress returns an address using the BLS protocol.
func NewBLSAddress(pubkey []byte) (Address, error) {
	return newAddress(types.BLS12, pubkey)
}

func newAddress(typ types.KeyType, payload []byte) (Address, error) {
	switch typ {
	case types.SECP256K1:
		if len(payload) != PayloadHashLength {
			return Undef, ErrInvalidPayload
		}
	case types.BLS12:
		if len(payload) != BlsPublicKeyBytes {
			return Undef, ErrInvalidPayload
		}
	default:
		return Undef, ErrUnknownAddrType
	}
	explen := 1 + len(payload)
	buf := make([]byte, explen)

	buf[0] = typ
	copy(buf[1:], payload)

	return Address{string(buf)}, nil
}

func encode(addr Address) (string, error) {
	if addr == Undef {
		return UndefAddressString, nil
	}

	var strAddr string
	switch addr.Type() {
	case types.SECP256K1, types.BLS12:
		cksm := Checksum(append([]byte{addr.Type()}, addr.Payload()...))
		strAddr = AddrPrefix + fmt.Sprintf("%d", addr.Type()) + b58.Encode(append(addr.Payload(), cksm[:]...))
	default:
		return UndefAddressString, ErrUnknownAddrType
	}
	return strAddr, nil
}

func decode(a string) (Address, error) {
	if len(a) == 0 {
		return Undef, nil
	}
	if a == UndefAddressString {
		return Undef, nil
	}
	if len(a) > MaxAddressStringLength || len(a) < 3 {
		return Undef, ErrInvalidLength
	}

	if string(a[0]) != AddrPrefix {
		return Undef, nil
	}

	var typ types.KeyType
	switch a[1] {
	case '2':
		typ = types.BLS12
	case '3':
		typ = types.SECP256K1

	default:
		return Undef, ErrUnknownAddrType
	}

	raw := a[2:]

	payloadcksm, err := b58.Decode(raw)
	if err != nil {
		return Undef, err
	}

	if len(payloadcksm)-ChecksumHashLength < 0 {
		return Undef, ErrInvalidChecksum
	}

	payload := payloadcksm[:len(payloadcksm)-ChecksumHashLength]
	cksm := payloadcksm[len(payloadcksm)-ChecksumHashLength:]

	if typ == types.SECP256K1 {
		if len(payload) != 20 {
			return Undef, ErrInvalidPayload
		}
	}

	if !ValidateChecksum(append([]byte{typ}, payload...), cksm) {
		return Undef, ErrInvalidChecksum
	}

	return newAddress(typ, payload)
}

// Checksum returns the checksum of `ingest`.
func Checksum(ingest []byte) []byte {
	return hash(ingest, checksumHashConfig)
}

func ValidateChecksum(ingest, expect []byte) bool {
	digest := Checksum(ingest)
	return bytes.Equal(digest, expect)
}

func hash(ingest []byte, cfg *blake2b.Config) []byte {
	hasher, err := blake2b.New(cfg)
	if err != nil {
		// If this happens sth is very wrong.
		panic(fmt.Sprintf("invalid address hash configuration: %v", err)) // ok
	}
	if _, err := hasher.Write(ingest); err != nil {
		// blake2bs Write implementation never returns an error in its current
		// setup. So if this happens sth went very wrong.
		panic(fmt.Sprintf("blake2b is unable to process hashes: %v", err)) // ok
	}
	return hasher.Sum(nil)
}
