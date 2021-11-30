package pdpcommon

import "errors"

var (
	ErrSplitSegmentToAtoms = errors.New("invalid segment")
	ErrKeyIsNil            = errors.New("the key is nil")
	ErrSetString           = errors.New("SetString is not true")
	ErrSetBigInt           = errors.New("SetBigInt is not true")
	ErrSetToBigInt         = errors.New("SetString (for big.Int) is not true")

	ErrInvalidSettings       = errors.New("setting is invalid")
	ErrNumOutOfRange         = errors.New("numOfAtoms is out of range")
	ErrDeserializeFailed     = errors.New("deserialize failed")
	ErrChalOutOfRange        = errors.New("numOfAtoms is out of chal range")
	ErrSegmentSize           = errors.New("the size of the segment is wrong")
	ErrGenTag                = errors.New("GenTag failed")
	ErrOffsetIsNegative      = errors.New("offset is negative")
	ErrProofVerifyInProvider = errors.New("proof is wrong")
	ErrVersionUnmatch        = errors.New("version unmatch")
	ErrVerifyFailed          = errors.New("verification failed")
)

// Tag constants
const (
	CRC32 = 1
	BLS   = 2
	PDPV0 = 3
	PDPV1 = 4
	PDPV2 = 5
)

// TagMap maps a hash code to it's default length
var TagMap = map[int]int{
	CRC32: 4,
	BLS:   32,
	PDPV0: 48,
	PDPV1: 48,
	PDPV2: 48,
}
