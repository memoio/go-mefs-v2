package pdpcommon

import "golang.org/x/xerrors"

var (
	ErrKeyIsNil        = xerrors.New("the key is nil")
	ErrInvalidSettings = xerrors.New("setting is invalid")
	ErrNumOutOfRange   = xerrors.New("numOfAtoms is out of range")
	ErrSegmentSize     = xerrors.New("the size of the segment is wrong")
	ErrVersionUnmatch  = xerrors.New("version unmatch")
	ErrVerifyFailed    = xerrors.New("verification failed")
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
