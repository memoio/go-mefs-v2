package tx

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type BucketParams struct {
	pb.BucketOption
	BucketID uint64
}

func (bp *BucketParams) Serialize() ([]byte, error) {
	return cbor.Marshal(bp)
}

func (bp *BucketParams) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, bp)
}

type EpochParams struct {
	Epoch uint64
	Prev  types.MsgID // hash pre
}

func (ep *EpochParams) Hash() types.MsgID {
	res, err := ep.Serialize()
	if err != nil {
		return types.MsgIDUndef
	}

	return types.NewMsgID(res)
}

func (ep *EpochParams) Serialize() ([]byte, error) {
	return cbor.Marshal(ep)
}

func (ep *EpochParams) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ep)
}

// verify sign
// verify time
type SignedEpochParams struct {
	EpochParams
	Sig types.MultiSignature
}

func (sep *SignedEpochParams) Serialize() ([]byte, error) {
	return cbor.Marshal(sep)
}

func (sep *SignedEpochParams) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sep)
}

type SegChalParams struct {
	UserID     uint64
	ProID      uint64
	Epoch      uint64
	OrderStart uint64
	OrderEnd   uint64
	Size       uint64
	Price      *big.Int
	Proof      []byte
}

func (scp *SegChalParams) Serialize() ([]byte, error) {
	return cbor.Marshal(scp)
}

func (scp *SegChalParams) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, scp)
}

type PostIncomeParams struct {
	Epoch  uint64
	Income types.AccPostIncome
	Sig    types.Signature
}

func (pip *PostIncomeParams) Serialize() ([]byte, error) {
	return cbor.Marshal(pip)
}

func (pip *PostIncomeParams) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, pip)
}

type SegRemoveParas struct {
	UserID   uint64
	ProID    uint64
	Nonce    uint64
	SeqNum   uint32
	Segments types.AggSegsQueue
}

func (srp *SegRemoveParas) Serialize() ([]byte, error) {
	return cbor.Marshal(srp)
}

func (srp *SegRemoveParas) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, srp)
}

type OrderCommitParas struct {
	UserID uint64
	ProID  uint64
	Nonce  uint64
	SeqNum uint32
}

func (ocp *OrderCommitParas) Serialize() ([]byte, error) {
	return cbor.Marshal(ocp)
}

func (ocp *OrderCommitParas) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ocp)
}

type OrderSubParas struct {
	UserID uint64
	ProID  uint64
	Nonce  uint64
}

func (osp *OrderSubParas) Serialize() ([]byte, error) {
	return cbor.Marshal(osp)
}

func (osp *OrderSubParas) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, osp)
}
