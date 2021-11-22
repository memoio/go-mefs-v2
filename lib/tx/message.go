package tx

import (
	"errors"
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type MsgType = uint32

const MsgMaxLen = 1<<16 - 1

var (
	ErrMsgLen      = errors.New("message length too longth")
	ErrMsgLenShort = errors.New("message length too short")
)

const (
	DataTxErr MsgType = iota

	// register
	CreateRole     // 更新，在结算链上的信息改变的时候；by keeper/provider/user
	UpdateGasMoney // by keeper/provider/user/fs
	WithdrawFee    // 获取链上消息费; by keeper

	UpdateNetAddr // 更新网络地址; by provider; needed(?)； 或者user和provider私下协商

	SetEpoch // 进入下一个周期；by keeper

	// data tx; after user is added
	CreateBucket // by user

	// order
	DataPreOrder    // by user
	DataOrder       // contain piece and segment; by user
	DataCommitOrder // by user or keeper; collect sign for order?

	CommitSector // confirm piece; by provider
	SetChalEpoch // set chal epoch and seed
	SegmentProof // segment proof; by provider
	SectorProof  // sector proof; by provider
	SegmentFault // segment remove; by provider
	SectorFault  // sector remove; by provider
	PostIncome   // add post income for provider; by keeper
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
	Signature types.Signature // signed by Tx.From;
	id        types.MsgID
}

func (sm *SignedMessage) Serialize() ([]byte, error) {
	return cbor.Marshal(sm)
}

func (sm *SignedMessage) Deserialize(b []byte) error {
	err := cbor.Unmarshal(b, sm)
	if err != nil {
		return nil
	}

	id, err := sm.Hash()
	if err != nil {
		return nil
	}

	sm.id = id
	return nil
}

type MessageState struct {
	BlockID types.MsgID
	Height  uint64
}

func (ms *MessageState) Serialize() ([]byte, error) {
	return cbor.Marshal(ms)
}

func (ms *MessageState) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ms)
}
