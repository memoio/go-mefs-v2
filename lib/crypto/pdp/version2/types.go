package pdpv2

import (
	"encoding/binary"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/zeebo/blake3"
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
	r        int64
	pubInput bls.Fr
}

func NewChallenge(r int64) pdpcommon.Challenge {
	var temp Fr
	return &Challenge{r, temp}
}

func (chal *Challenge) Version() uint16 {
	return pdpcommon.PDPV2
}

func (chal *Challenge) Random() int64 {
	return chal.r
}

func (chal *Challenge) PublicInput() []byte {
	return bls.FrToBytes(&chal.pubInput)
}

func (chal *Challenge) Add(b []byte) error {
	var temp Fr
	if len(b) != 32 {
		tmp := blake3.Sum256(b)
		b = tmp[:]
	}
	err := bls.FrFromBytes(&temp, b)
	if err != nil {
		return err
	}

	bls.FrAddMod(&chal.pubInput, &chal.pubInput, &temp)
	return nil
}

func (chal *Challenge) Delete(b []byte) error {
	var temp Fr
	if len(b) != 32 {
		tmp := blake3.Sum256(b)
		b = tmp[:]
	}
	err := bls.FrFromBytes(&temp, b)
	if err != nil {
		return err
	}

	bls.FrSubMod(&chal.pubInput, &chal.pubInput, &temp)
	return nil
}

func (chal *Challenge) Deserialize(buf []byte) error {
	if len(buf) != 10+FrSize {
		return pdpcommon.ErrDeserializeFailed
	}

	ver := binary.BigEndian.Uint16(buf[:2])
	if ver != pdpcommon.PDPV2 {
		return pdpcommon.ErrVersionUnmatch
	}

	chal.r = int64(binary.BigEndian.Uint64(buf[2:10]))

	var temp Fr
	err := bls.FrFromBytes(&temp, buf[10:10+FrSize])
	if err != nil {
		return err
	}
	chal.pubInput = temp

	return nil
}

func (chal *Challenge) Serialize() []byte {
	buf := make([]byte, 10+FrSize)
	binary.BigEndian.PutUint16(buf[:2], chal.Version())
	binary.BigEndian.PutUint64(buf[2:10], uint64(chal.r))
	copy(buf[10:10+FrSize], bls.FrToBytes(&chal.pubInput))
	return buf
}

// Proof is result
type Proof struct {
	Psi   G1 `json:"psi"`
	Kappa G1 `json:"kappa"`
}

func NewProof(psi, kappa G1) pdpcommon.Proof {
	return &Proof{Psi: psi, Kappa: kappa}
}

func (pf *Proof) Version() uint16 {
	return pdpcommon.PDPV2
}

func (pf *Proof) Serialize() []byte {
	buf := make([]byte, 2+2*G1Size)

	binary.BigEndian.PutUint16(buf[:2], pf.Version())
	copy(buf[2:2+G1Size], bls.G1Serialize(&pf.Psi))
	copy(buf[2+G1Size:2+2*G1Size], bls.G1Serialize(&pf.Kappa))

	return buf
}
func (pf *Proof) Deserialize(buf []byte) error {
	if len(buf) != 2+2*G1Size {
		return pdpcommon.ErrNumOutOfRange
	}

	ver := binary.BigEndian.Uint16(buf[:2])
	if ver != pdpcommon.PDPV2 {
		return pdpcommon.ErrVersionUnmatch
	}

	err := bls.G1Deserialize(&pf.Psi, buf[2:2+G1Size])
	if err != nil {
		return err
	}

	err = bls.G1Deserialize(&pf.Kappa, buf[2+G1Size:2+2*G1Size])
	if err != nil {
		return err
	}
	return nil
}
