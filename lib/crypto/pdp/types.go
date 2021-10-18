package pdp

import (
	"encoding/binary"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
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

type KeySetWithVersion struct {
	Ver uint16
	Sk  pdpcommon.SecretKey
	Pk  pdpcommon.PublicKey
}

//将proof序列化后，加个版本号
type ProofWithVersion struct {
	Ver   uint16
	Proof pdpcommon.Proof
}

func (pfv *ProofWithVersion) Version() int {
	return int(pfv.Ver)
}

func (pfv *ProofWithVersion) Serialize() ([]byte, error) {
	if pfv == nil {
		return nil, pdpcommon.ErrKeyIsNil
	}
	lenBuf := make([]byte, 2)
	binary.LittleEndian.PutUint16(lenBuf, pfv.Ver)
	pf := pfv.Proof.Serialize()
	buf := make([]byte, 0, 2+len(pf))
	buf = append(buf, lenBuf[:2]...)
	buf = append(buf, pf...)
	return buf, nil
}

func (pfv *ProofWithVersion) Deserialize(data []byte) error {
	if len(data) <= 2 {
		return pdpcommon.ErrNumOutOfRange
	}
	v := binary.LittleEndian.Uint16(data[:2])
	var proof pdpcommon.Proof
	switch v {
	case PDPV2:
		proof = new(pdpv2.Proof)
	default:
		return pdpcommon.ErrInvalidSettings
	}

	err := proof.Deserialize(data[2:])
	if err != nil {
		return err
	}
	pfv.Proof = proof
	pfv.Ver = v
	return err
}

//将Challenge序列化后，加个版本号
type ChallengeWithVersion struct {
	Ver  uint16
	Chal pdpcommon.Challenge
}

func (ch *ChallengeWithVersion) Version() int {
	return int(ch.Ver)
}

func (ch *ChallengeWithVersion) Serialize() ([]byte, error) {
	if ch == nil {
		return nil, pdpcommon.ErrKeyIsNil
	}
	lenBuf := make([]byte, 2)
	binary.LittleEndian.PutUint16(lenBuf, ch.Ver)
	chal := ch.Chal.Serialize()
	buf := make([]byte, 0, 2+len(chal))
	buf = append(buf, lenBuf[:2]...)
	buf = append(buf, chal...)
	return buf, nil
}

func (ch *ChallengeWithVersion) Deserialize(data []byte) error {
	v := binary.LittleEndian.Uint16(data[:2])
	var chal pdpcommon.Challenge
	switch v {
	case PDPV2:
		chal = new(pdpv2.Challenge)
	default:
		return pdpcommon.ErrInvalidSettings
	}
	err := chal.Deserialize(data[2:])
	if err != nil {
		return err
	}
	ch.Chal = chal
	ch.Ver = v
	return err
}
