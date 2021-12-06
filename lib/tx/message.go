package tx

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/types"
)

type MsgType = uint32

// 1MB ok ?
const MsgMaxLen = 1 << 20

var (
	ErrMsgLen      = xerrors.New("message length too longth")
	ErrMsgLenShort = xerrors.New("message length too short")
)

const (
	DataTxErr MsgType = iota

	UpdateChalEpoch // next epoch, by keeper
	AddRole         // by each role
	CreateFs        // register, by user
	CreateBucket    // by user
	DataPreOrder    // by user
	DataOrder       // contain piece and segment; by user
	DataOrderCommit // commit
	SegmentProof    // segment proof; by provider
	SegmentFault    // segment remove; by provider
	PostIncome      // add post income for provider; by keeper

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
		return nil, ErrMsgLen
	}
	return res, nil
}

// get message hash for sign
func (m *Message) Hash() (types.MsgID, error) {
	res, err := m.Serialize()
	if err != nil {
		return types.Undef, err
	}

	return types.NewMsgID(res), nil
}

func (m *Message) Deserialize(b []byte) (types.MsgID, error) {
	err := cbor.Unmarshal(b, m)
	if err != nil {
		return types.Undef, err
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
	return cbor.Marshal(sm)
}

func (sm *SignedMessage) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sm)
}
