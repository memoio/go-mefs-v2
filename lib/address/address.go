package address

import (
	"bytes"
	"encoding/json"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

// UndefAddressString is the string used to represent an empty address when encoded to a string.
var UndefAddressString = "<empty>"

// Secp256k1PublicKeyBytes defines of a SECP256K1 public key.
const Secp256k1PublicKeyBytes = 65

// BlsPublicKeyBytes is the length of a BLS public key
const BlsPublicKeyBytes = 48

// ChecksumHashLength defines the hash length used for calculating address checksums.
const ChecksumHashLength = 4

// MaxAddressStringLength is the max length of an address encoded as a string
// it include the network prefx, and publickey encode ength
// 2 + 1.36*(publickey length + ChecksumHashLength)
const MaxAddressStringLength = AddrPrefixLen + 94

// Address is the go type that represents an address.
// publickey or hash(public)
type Address struct{ str string }

// Undef is the type that represents an undefined address.
var Undef = Address{}

const AddrPrefix = "Me"
const AddrPrefixLen = 2

func (a Address) Len() int {
	return len(a.str)
}

// Bytes returns the address as bytes.
func (a Address) Bytes() []byte {
	return []byte(a.str)
}

// String returns an address encoded as a string.
func (a Address) String() string {
	str, err := encode(a)
	if err != nil {
		panic(err)
	}
	return str
}

// Empty returns true if the address is empty, false otherwise.
func (a Address) Empty() bool {
	return a == Undef
}

// for jsonrpc
// UnmarshalJSON implements the json unmarshal interface.
func (a *Address) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	addr, err := decode(s)
	if err != nil {
		return err
	}
	*a = addr
	return nil
}

// MarshalJSON implements the json marshal interface.
func (a Address) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

// for cbor
func (a *Address) UnmarshalBinary(data []byte) error {
	id, err := newAddress(data)
	if err != nil {
		return err
	}
	*a = id
	return nil
}

func (a Address) MarshalBinary() ([]byte, error) {
	return a.Bytes(), nil
}

func NewAddress(payload []byte) (Address, error) {
	return newAddress(payload)
}

func newAddress(payload []byte) (Address, error) {
	buf := make([]byte, len(payload))
	copy(buf[:], payload)
	return Address{string(buf)}, nil
}

func NewFromString(s string) (Address, error) {
	return decode(s)
}

func encode(addr Address) (string, error) {
	if addr == Undef {
		return UndefAddressString, nil
	}

	cksm := Checksum(addr.Bytes())
	strAddr := AddrPrefix + b58.Encode(append(addr.Bytes(), cksm[:]...))

	return strAddr, nil
}

func decode(a string) (Address, error) {
	if len(a) == 0 {
		return Undef, xerrors.New("invalid address length")
	}
	if a == UndefAddressString {
		return Undef, xerrors.New("invalid address length")
	}
	if len(a) > MaxAddressStringLength || len(a) < 3 {
		return Undef, xerrors.New("invalid address length")
	}

	if string(a[0:AddrPrefixLen]) != AddrPrefix {
		return Undef, xerrors.New("unknown address type")
	}

	raw := a[AddrPrefixLen:]

	payloadcksm, err := b58.Decode(raw)
	if err != nil {
		return Undef, err
	}

	if len(payloadcksm)-ChecksumHashLength < 0 {
		return Undef, xerrors.New("invalid address checksum")
	}

	payload := payloadcksm[:len(payloadcksm)-ChecksumHashLength]
	cksm := payloadcksm[len(payloadcksm)-ChecksumHashLength:]

	if !ValidateChecksum(payload, cksm) {
		return Undef, xerrors.New("invalid address checksum")
	}

	return newAddress(payload)
}

// Checksum returns the checksum of `ingest`.
func Checksum(ingest []byte) []byte {
	h := blake3.New()
	h.Write(ingest)
	res := h.Sum(nil)
	return res[:ChecksumHashLength]
}

func ValidateChecksum(ingest, expect []byte) bool {
	digest := Checksum(ingest)
	return bytes.Equal(digest, expect)
}
