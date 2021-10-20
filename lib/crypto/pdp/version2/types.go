package pdpv2

import (
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
)

// 自/data-format/common.go，目前segment的default size为124KB
const (
	SCount         = 8192
	DefaultType    = 31
	DefaultSegSize = 248 * 1024
)

type G1 = bls.G1Point
type G2 = bls.G2Point
type GT = bls.GTPoint
type Fr = bls.Fr

const G1Size = bls.G1Size
const G2Size = bls.G2Size
const FrSize = bls.FrSize

var ZERO_G1 = bls.ZERO_G1

var GenG1 = bls.GenG1
var GenG2 = bls.GenG2

var ZeroG1 = bls.ZeroG1
var ZeroG2 = bls.ZeroG2

// Challenge gives
type Challenge struct {
	r       int64
	indices [][]byte
}

func (chal *Challenge) Version() int {
	return 1
}

func (chal *Challenge) Random() int64 {
	return chal.r
}

func (chal *Challenge) Indices() [][]byte {
	return chal.indices
}

func (chal *Challenge) Deserialize(data []byte) error {
	return nil
}

func (chal *Challenge) Serialize() []byte {
	return nil
}

// Proof is result
type Proof struct {
	Psi   []byte `json:"delta"`
	Kappa []byte `json:"kappa"`
}

func (pf *Proof) Version() int {
	return 2
}

func (pf *Proof) Serialize() []byte {
	buf := make([]byte, 0, len(pf.Psi)+len(pf.Kappa))
	buf = append(buf, pf.Psi...)
	buf = append(buf, pf.Kappa...)

	return buf
}
func (pf *Proof) Deserialize(buf []byte) error {
	if len(buf) != 2*G1Size {
		return pdpcommon.ErrNumOutOfRange
	}
	pf.Psi = make([]byte, G1Size)
	pf.Kappa = make([]byte, G1Size)
	copy(pf.Psi, buf[0:G1Size])
	copy(pf.Kappa, buf[G1Size:2*G1Size])
	return nil
}

type FaultBlocks struct {
	ID       string
	BucketID uint64
	CID      map[uint64][]uint64 //stripe指向一个[]uint64，这是一个位图，第几位标记上了，就代表stripe内的该块损坏
	//当然，其实这里只记录stripe号，因为按理说，一个节点只能存储该stripe内的一个块，不可能冲突
}

type ChallengeSeed struct {
	UserID      []byte
	BucketID    int64
	Seed        int64
	StripeStart int64
	StripeEnd   int64
	SegEnd      int64
}

//TODO:
func (chal *ChallengeSeed) Serialize() ([]byte, error) {
	return nil, nil
}

func (chal *ChallengeSeed) Deserialize(data []byte) error {
	return nil
}
