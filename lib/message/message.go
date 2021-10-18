package message

import (
	"math/big"
)

const (
	DataTxErr uint32 = iota

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

// hash(message) as key
// gasLimit: 根据数据量，非线性
type Message struct {
	Version uint32

	To   uint64
	From uint64 // userID/providerID/keeperID

	Nonce uint64

	Value *big.Int

	GasLimit uint64
	GasPrice *big.Int

	Method uint32
	Params []byte // Record bytes?
}

func NewMessage() Message {
	return Message{
		Version:  0,
		Value:    big.NewInt(0),
		GasPrice: big.NewInt(0),
	}
}

func (m *Message) Serialize() []byte {
	// cbor?
	return nil
}

// verify:
// check size; anti ddos?
// 1. signature is right according to from
// 2. nonce is right
// 3. value is enough
// 4. gas is enough
type SignedMessage struct {
	Message
	Signature []byte // signed by Tx.From;
}

func (sm *SignedMessage) Serialize() []byte {
	return nil
}
