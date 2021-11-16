package order

import (
	"github.com/fxamacker/cbor/v2"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderState uint8

const (
	Order_Init OrderState = iota //
	Order_Done                   // order is done
)

type NonceState struct {
	Nonce uint64
	Time  int64
	State OrderState
}

type OrderSeqState uint8

const (
	OrderSeq_Init   OrderSeqState = iota // can receiving data
	OrderSeq_Finish                      // finished
)

type SeqState struct {
	Number uint32
	Time   int64
	State  OrderSeqState
}

type OrderFull struct {
	ds store.KVStore

	userID uint64
	fsID   []byte

	base       *types.OrderBase
	orderTime  int64
	orderState OrderState

	seq      *types.OrderSeq // 当前处理
	seqTime  int64
	seqState OrderSeqState

	nonce  uint64 // next nonce
	seqNum uint32 // next seq

	pk pdpcommon.PublicKey
	dv pdpcommon.DataVerifier

	ready bool
}

func (m *OrderMgr) createOrder(op *OrderFull) *OrderFull {
	pk, err := m.getBlsPubkey(op.userID)
	if err != nil {
		return op
	}

	op.pk = pk
	op.dv = pdpv2.NewDataVerifier(pk, nil)

	op.fsID = pk.VerifyKey().Hash()

	op.ready = true

	return op
}

func (m *OrderMgr) loadOrder(userID uint64) *OrderFull {
	op := &OrderFull{
		userID:     userID,
		orderState: Order_Done,
	}

	ns := new(NonceState)
	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, userID)
	val, err := m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ns)
	if err != nil {
		return op
	}

	op.orderState = ns.State
	op.nonce = ns.Nonce

	ob := new(types.OrderBase)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, userID, op.nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ob)
	if err != nil {
		return op
	}

	op.base = ob
	op.orderTime = ns.Time

	ss := new(SeqState)
	key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, userID, op.nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ss)
	if err != nil {
		return op
	}

	op.seqState = ss.State
	op.seqNum = ss.Number

	os := new(types.OrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, userID, op.nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, os)
	if err != nil {
		return op
	}

	op.seq = os
	op.seqTime = ss.Time

	op.nonce = ns.Nonce + 1
	op.seqNum = ss.Number + 1

	return op

}
