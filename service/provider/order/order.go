package order

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderState uint8

const (
	Order_Init OrderState = iota //
	Order_Ack                    // order is acked
	Order_Done                   // order is done
)

type NonceState struct {
	Nonce uint64
	Time  int64
	State OrderState
}

type OrderSeqState uint8

const (
	OrderSeq_Init OrderSeqState = iota // can receiving data
	OrderSeq_Ack                       // seq is acked
	OrderSeq_Done                      // finished
)

type SeqState struct {
	Number uint32
	Time   int64
	State  OrderSeqState
}

type OrderFull struct {
	userID uint64
	fsID   []byte

	base       *types.SignedOrder
	orderTime  int64
	orderState OrderState
	segPrice   *big.Int

	seq      *types.SignedOrderSeq // 当前处理
	seqTime  int64
	seqState OrderSeqState

	nonce  uint64 // next nonce
	seqNum uint32 // next seq

	dv pdpcommon.DataVerifier

	ready bool
}

func (m *OrderMgr) createOrder(op *OrderFull) *OrderFull {

	pk, err := m.GetPublicKey(op.userID)
	if err != nil {
		logger.Warn("create order bls pk err: ", err)
		return op
	}

	op.dv, err = pdp.NewDataVerifier(pk, nil)
	if err != nil {
		logger.Warn("create order data verifier err: ", err)
		return op
	}

	op.fsID = pk.VerifyKey().Hash()

	op.ready = true

	return op
}

func (m *OrderMgr) loadOrder(userID uint64) *OrderFull {
	op := &OrderFull{
		userID: userID,
	}

	pk, err := m.GetPublicKey(userID)
	if err == nil {
		op.dv, err = pdp.NewDataVerifier(pk, nil)
		if err != nil {
			return op
		}
		op.fsID = pk.VerifyKey().Hash()
		op.ready = true
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

	ob := new(types.SignedOrder)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, userID, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ob)
	if err != nil {
		return op
	}

	op.base = ob
	op.orderState = ns.State
	op.orderTime = ns.Time
	op.nonce = ns.Nonce + 1
	op.segPrice = new(big.Int).Mul(ob.SegPrice, big.NewInt(build.DefaultSegSize))

	ss := new(SeqState)
	key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, userID, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ss)
	if err != nil {
		return op
	}

	os := new(types.SignedOrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, userID, ns.Nonce, ss.Number)
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
	op.seqState = ss.State
	op.seqNum = ss.Number + 1

	return op

}
