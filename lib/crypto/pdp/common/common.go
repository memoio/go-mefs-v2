package pdpcommon

import "golang.org/x/xerrors"

var (
	ErrSplitSegmentToAtoms = xerrors.New("invalid segment")
	ErrKeyIsNil            = xerrors.New("the key is nil")
	ErrSetString           = xerrors.New("SetString is not true")
	ErrSetBigInt           = xerrors.New("SetBigInt is not true")
	ErrSetToBigInt         = xerrors.New("SetString (for big.Int) is not true")

	ErrInvalidSettings       = xerrors.New("setting is invalid")
	ErrNumOutOfRange         = xerrors.New("numOfAtoms is out of range")
	ErrDeserializeFailed     = xerrors.New("deserialize failed")
	ErrChalOutOfRange        = xerrors.New("numOfAtoms is out of chal range")
	ErrSegmentSize           = xerrors.New("the size of the segment is wrong")
	ErrGenTag                = xerrors.New("GenTag failed")
	ErrOffsetIsNegative      = xerrors.New("offset is negative")
	ErrProofVerifyInProvider = xerrors.New("proof is wrong")
	ErrVersionUnmatch        = xerrors.New("version unmatch")
	ErrVerifyFailed          = xerrors.New("verification failed")
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
