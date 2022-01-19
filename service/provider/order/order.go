package order

import (
	"math/big"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderState string

const (
	Order_Init OrderState = "init" //
	Order_Ack  OrderState = "ack"  // order is acked
	Order_Done OrderState = "done" // order is done
)

type NonceState struct {
	Nonce uint64
	Time  int64
	State OrderState
}

func (ns *NonceState) Serialize() ([]byte, error) {
	return cbor.Marshal(ns)
}

func (ns *NonceState) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ns)
}

type OrderSeqState string

const (
	OrderSeq_Init OrderSeqState = "init" // can receiving data
	OrderSeq_Ack  OrderSeqState = "ack"  // seq is acked
	OrderSeq_Done OrderSeqState = "done" // finished
)

type SeqState struct {
	Number uint32
	Time   int64
	State  OrderSeqState
}

func (ss *SeqState) Serialize() ([]byte, error) {
	return cbor.Marshal(ss)
}

func (ss *SeqState) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ss)
}

// todo: check order
type OrderFull struct {
	lw        sync.Mutex
	localID   uint64
	userID    uint64
	fsID      []byte
	availTime int64

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

	active time.Time // remove it when it inactive for long time
	ready  bool
	pause  bool // if nonce is far from now, not create order
}

func (m *OrderMgr) runCheck() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.check()
		}
	}
}

func (m *OrderMgr) check() error {
	ulen := len(m.users)
	for i := 0; i < ulen; i++ {
		m.lk.RLock()
		uid := m.users[i]
		of := m.orders[uid]
		m.lk.RUnlock()

		if !of.ready {
			continue
		}

		ns := m.ics.GetOrderState(m.ctx, uid, m.localID)
		oi, err := m.is.GetStoreInfo(m.ctx, uid, m.localID)
		if err != nil {
			continue
		}

		if ns.Nonce+1 < of.nonce || oi.Nonce+2 < of.nonce {
			of.lw.Lock()
			of.pause = true
			of.lw.Unlock()
			logger.Warn("order is not submit to data or settle chain: ", of.nonce, ns.Nonce, oi.Nonce)
		}

		if ns.Nonce+1 >= of.nonce && oi.Nonce+2 >= of.nonce {
			of.lw.Lock()
			of.pause = false
			of.lw.Unlock()
			logger.Debug("order is submit to data or settle chain: ", of.nonce, ns.Nonce, oi.Nonce)
		}
	}

	return nil
}

func (m *OrderMgr) createOrder(op *OrderFull) {
	pk, err := m.ics.GetPDPPublicKey(m.ctx, op.userID)
	if err != nil {
		logger.Warnf("create order for user %d bls pk fail %s", op.userID, err)
		return
	}

	op.dv, err = pdp.NewDataVerifier(pk, nil)
	if err != nil {
		logger.Warn("create order data verifier err: ", err)
		return
	}

	op.fsID = pk.VerifyKey().Hash()

	op.ready = true
}

// todo: load from data chain
// todo: fix missing if provider has fault
func (m *OrderMgr) getOrder(userID uint64) *OrderFull {
	m.lk.Lock()
	op, ok := m.orders[userID]
	if ok {
		m.lk.Unlock()
		return op
	}

	op = &OrderFull{
		localID: m.localID,
		userID:  userID,
		active:  time.Now(),
	}
	m.users = append(m.users, userID)
	m.orders[userID] = op
	m.lk.Unlock()

	pk, err := m.ics.GetPDPPublicKey(m.ctx, userID)
	if err == nil {
		op.dv, err = pdp.NewDataVerifier(pk, nil)
		if err != nil {
			return op
		}
		op.fsID = pk.VerifyKey().Hash()
		op.ready = true
	}

	dns := m.ics.GetOrderState(m.ctx, userID, m.localID)

	ns := new(NonceState)
	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, userID)
	val, err := m.ds.Get(key)
	if err == nil {
		err = ns.Deserialize(val)
		if err != nil {
			return op
		}
	} else {
		ns.State = Order_Init
		ns.Nonce = dns.Nonce
	}

	ob := new(types.SignedOrder)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, userID, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = ob.Deserialize(val)
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
	if err == nil {
		err = ss.Deserialize(val)
		if err != nil {
			return op
		}
	} else {
		ss.State = OrderSeq_Init
		ss.Number = dns.SeqNum
	}

	os := new(types.SignedOrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, userID, ns.Nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = os.Deserialize(val)
	if err != nil {
		return op
	}

	op.seq = os
	op.seqTime = ss.Time
	op.seqState = ss.State
	op.seqNum = ss.Number + 1

	return op
}

func saveOrderBase(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderBaseKey, o.localID, o.userID, o.base.Nonce)
	data, err := o.base.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

func saveOrderState(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderNonceKey, o.localID, o.userID)
	ns := &NonceState{
		Nonce: o.base.Nonce,
		Time:  o.orderTime,
		State: o.orderState,
	}
	val, err := ns.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, val)
}

func saveOrderSeq(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderSeqKey, o.localID, o.userID, o.base.Nonce, o.seq.SeqNum)
	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

func saveSeqState(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderSeqNumKey, o.localID, o.userID, o.base.Nonce)
	ss := SeqState{
		Number: o.seq.SeqNum,
		Time:   o.seqTime,
		State:  o.seqState,
	}
	val, err := ss.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, val)
}
