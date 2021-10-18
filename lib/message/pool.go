package message

import (
	"math/big"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/types"
)

type MsgInfo struct {
	state uint8    // 1: in pool; 2: added to block; 3: confirmed in block
	hash  [32]byte // message hash
}

type msgSet struct {
	nonce uint64              // next
	msg   map[uint64]*MsgInfo // key: nonce; value: MsgInfo
}

type MessagePool struct {
	txMap   sync.Map           // key: txHash; value: *Message
	pending map[uint64]*msgSet // key: from; all currently processable tx
	queue   map[uint64]*msgSet // key: from; queued but non- processable tx
}

func (mp *MessagePool) AddMsg(m *SignedMessage) error {
	return nil
}

func (mp *MessagePool) ValidateMsg(m *SignedMessage) bool {
	return false
}

type Record struct {
	key   []byte
	value []byte
}

type SegStateTransition struct {
	height      uint64 // 块高度
	state       uint8  // 状态
	txHash      []byte
	keys        map[string]struct{}
	moneyChange map[uint64]*big.Int // 账户改变
	segChange   []*types.SegInfo
	root        []byte // new root of fs
}
