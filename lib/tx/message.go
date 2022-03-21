package tx

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/types"
)

type MsgType = uint32

// 512KB ok ?
const MsgMaxLen = 1 << 19

const (
	DataTxErr MsgType = 0

	AddRole   MsgType = 1 // by each role
	UpdateNet MsgType = 2

	UpdateChalEpoch   MsgType = 11 // next epoch, by keeper
	ConfirmPostIncome MsgType = 12 // add post income for provider; by keeper
	ConfirmDataOrder  MsgType = 13 // commit by keepers

	CreateFs        MsgType = 21 // register, by user
	CreateBucket    MsgType = 22 // by user
	PreDataOrder    MsgType = 23 // by user
	AddDataOrder    MsgType = 24 // contain piece and segment; by user
	CommitDataOrder MsgType = 25 // commit by user

	SegmentProof MsgType = 31 // segment proof; by provider
	SegmentFault MsgType = 32 // segment remove; by provider
	SubDataOrder MsgType = 33 // order is expired; by provider
)

// MsgID(message) as key
// gasLimit: 根据数据量，非线性
type Message struct {
	Version uint32

	From  uint64
	To    uint64
	Nonce uint64
	Value *big.Int

	GasLimit uint64
	GasPrice *big.Int

	Method uint32
	Params []byte // decode accoording to method
}

func NewMessage() Message {
	return Message{
		Version:  1,
		Value:    big.NewInt(0),
		GasPrice: big.NewInt(0),
	}
}

func (m *Message) Serialize() ([]byte, error) {
	res, err := cbor.Marshal(m)
	if err != nil {
		return nil, err
	}

	if len(res) > int(MsgMaxLen) {
		return nil, xerrors.Errorf("message length %d is longer than %d", len(res), MsgMaxLen)
	}
	return res, nil
}

// get message hash for sign
func (m *Message) Hash() types.MsgID {
	res, err := m.Serialize()
	if err != nil {
		return types.MsgIDUndef
	}

	return types.NewMsgID(res)
}

func (m *Message) Deserialize(b []byte) (types.MsgID, error) {
	err := cbor.Unmarshal(b, m)
	if err != nil {
		return types.MsgIDUndef, err
	}

	return types.NewMsgID(b), nil
}

// verify:
// check size; anti ddos?
// 1. signature is right according to from
// 2. nonce is right
// 3. value is enough
// 4. gas is enough
type SignedMessage struct {
	Message
	Signature types.Signature // signed by Tx.From
}

func (sm *SignedMessage) Serialize() ([]byte, error) {
	res, err := cbor.Marshal(sm)
	if err != nil {
		return nil, err
	}
	if len(res) > int(MsgMaxLen) {
		return nil, xerrors.Errorf("message length %d is longer than %d", len(res), MsgMaxLen)
	}
	return res, nil
}

func (sm *SignedMessage) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sm)
}
