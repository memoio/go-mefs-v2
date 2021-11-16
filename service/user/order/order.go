package order

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderState uint8

const (
	Order_Init    OrderState = iota // wait pro ack -> running
	Order_Running                   // time up -> close
	Order_Closing                   // all seq done -> done
	Order_Done                      // new data -> init
)

type NonceState struct {
	Nonce uint64
	Time  int64
	State OrderState
}

type OrderSeqState uint8

const (
	OrderSeq_Prepare OrderSeqState = iota // wait pro ack -> send
	OrderSeq_Send                         // can add data and send; time up -> commit
	OrderSeq_Commit                       // wait pro ack -> finish
	OrderSeq_Finish                       // new data -> prepare
)

type SeqState struct {
	Number uint32
	Time   int64
	State  OrderSeqState
}

// per provider
type OrderFull struct {
	sync.RWMutex

	api.IDataService // segment

	ctx context.Context
	ds  store.KVStore

	localID uint64
	fsID    []byte

	pro       uint64
	availTime int64 // last connect time

	nonce  uint64 // next nonce
	seqNum uint32 // next seq

	base       *types.OrderBase // quotation-> base
	orderTime  int64
	orderState OrderState

	seq      *types.OrderSeq
	seqTime  int64
	seqState OrderSeqState

	inflight bool            // data is sending
	inStop   bool            // stop receiving data; duo to high price or long unavil
	segs     []*types.SegJob // buf and persist?

	segDoneChan chan *types.SegJob
}

func (m *OrderMgr) newProOrder(id uint64) {
	logger.Debug("create order for provider: ", id)
	of := m.loadProOrder(id)
	m.proChan <- of
}

func (m *OrderMgr) loadProOrder(id uint64) *OrderFull {

	op := &OrderFull{
		IDataService: m.IDataService,

		ctx: m.ctx,
		ds:  m.ds,

		localID: m.localID,
		fsID:    m.fsID,
		pro:     id,

		availTime: time.Now().Unix(),

		segs: make([]*types.SegJob, 0, 16),

		orderState: Order_Done,

		segDoneChan: m.segDoneChan,
	}

	go op.sendData()

	ns := new(NonceState)
	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, id)
	val, err := m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ns)
	if err != nil {
		return op
	}

	op.nonce = ns.Nonce + 1
	op.orderState = ns.State

	if ns.State == Order_Done {
		return op
	}

	ob := new(types.OrderBase)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, id, op.nonce)
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
	key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, id, op.nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ss)
	if err != nil {
		return op
	}

	op.seqNum = ss.Number + 1
	op.seqState = ss.State
	if ss.State == OrderSeq_Finish {
		return op
	}

	os := new(types.OrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, id, op.nonce, ss.Number)
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

	return op
}

func (m *OrderMgr) check(o *OrderFull) {
	// 1 init; wait confirm
	// 2 running;
	// 2.1 seq prepare: wait pro ack
	// 2.2 seq send: wait to commit;
	// 2.3 seq commit: wait pro ack
	// 2.4 seq done: trigger new seq
	// 3 closing; push seq to done
	// 4. done -> new

	logger.Debug("check state for: ", o.pro, o.orderState, o.seqState, len(o.segs), o.inStop)

	nt := time.Now().Unix()
	switch o.orderState {
	case Order_Init:
		if o.base == nil {
			o.RLock()
			if len(o.segs) > 0 && !o.inStop {
				go m.getQuotation(o.pro)
				o.RUnlock()
				return
			}
			o.RUnlock()
		} else {
			if nt-o.orderTime > DefaultAckWaiting {
				m.createOrder(o, nil)
			}
		}
	case Order_Running:
		if nt-o.orderTime > DefaultOrderLast {
			m.closeOrder(o)
		}
		switch o.seqState {
		case OrderSeq_Prepare:
			// not receive callback
			if nt-o.seqTime > DefaultAckWaiting {
				m.createSeq(o)
			}
		case OrderSeq_Send:
			// time is up for next seq
			if nt-o.seqTime > DefaultOrderSeqLast {
				m.commitSeq(o)
			}
		case OrderSeq_Commit:
			// not receive callback
			if nt-o.seqTime > DefaultAckWaiting {
				m.commitSeq(o)
			}
		case OrderSeq_Finish:
			o.RLock()
			if len(o.segs) > 0 && !o.inStop {
				m.createSeq(o)
				o.RUnlock()
				return
			}
			o.RUnlock()
		}
	case Order_Closing:
		if o.seqState == OrderSeq_Finish {
			m.doneOrder(o)
		}
	case Order_Done:
		o.RLock()
		if len(o.segs) > 0 && !o.inStop {
			go m.getQuotation(o.pro)
			o.RUnlock()
			return
		}
		o.RUnlock()
	}

	if nt-o.availTime > 1800 {
		go m.connect(o.pro)
	}

	if nt-o.availTime > 3600 {
		m.stopOrder(o)
	}
}

// create a new order
func (m *OrderMgr) createOrder(o *OrderFull, quo *types.Quotation) error {
	o.RLock()
	if o.inStop {
		o.RUnlock()
		return ErrState
	}
	o.RUnlock()

	if o.base == nil && o.orderState == Order_Done {
		// compare to set price
		if quo != nil && quo.SegPrice.Cmp(m.segPrice) > 0 {
			m.stopOrder(o)
			return ErrPrice
		}

		o.base = &types.OrderBase{
			UserID:     o.localID,
			ProID:      quo.ProID,
			Nonce:      o.nonce,
			TokenIndex: quo.TokenIndex,
			SegPrice:   quo.SegPrice,
			PiecePrice: quo.PiecePrice,
			Start:      time.Now().Unix(),
			End:        time.Now().Unix() + 8640000,
		}

		o.nonce++
		o.orderState = Order_Init
		o.orderTime = time.Now().Unix()
	}

	if o.base != nil && o.orderState == Order_Init {
		o.orderTime = time.Now().Unix()

		data, err := o.base.Serialize()
		if err != nil {
			return err
		}

		key := store.NewKey(pb.MetaType_OrderBaseKey, m.localID, o.pro, o.nonce)
		err = m.ds.Put(key, data)
		if err != nil {
			return err
		}

		ns := &NonceState{
			Nonce: o.base.Nonce,
			Time:  o.orderTime,
			State: o.orderState,
		}

		val, err := cbor.Marshal(ns)
		if err != nil {
			return err
		}

		key = store.NewKey(pb.MetaType_OrderNonceKey, m.localID, o.pro)
		err = m.ds.Put(key, val)
		if err != nil {
			return err
		}

		// send to pro
		go m.getNewOrderAck(o.pro, data)

		return nil
	}

	return ErrState
}

// confirm base when receive pro ack; init -> running
func (m *OrderMgr) runOrder(o *OrderFull, ob *types.OrderBase) error {
	if o.base == nil || o.orderState != Order_Init {
		return ErrState
	}

	o.orderState = Order_Running
	o.orderTime = time.Now().Unix()

	ns := &NonceState{
		Nonce: o.base.Nonce,
		Time:  o.orderTime,
		State: o.orderState,
	}

	val, err := cbor.Marshal(ns)
	if err != nil {
		return err
	}

	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, o.pro)
	err = m.ds.Put(key, val)
	if err != nil {
		return err
	}

	return nil
}

// time up to close current order
func (m *OrderMgr) closeOrder(o *OrderFull) error {
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}

	o.orderState = Order_Closing
	o.orderTime = time.Now().Unix()

	ns := &NonceState{
		Nonce: o.base.Nonce,
		Time:  o.orderTime,
		State: o.orderState,
	}

	val, err := cbor.Marshal(ns)
	if err != nil {
		return err
	}

	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, o.pro)
	err = m.ds.Put(key, val)
	if err != nil {
		return err
	}

	return nil
}

// finish all seqs
func (m *OrderMgr) doneOrder(o *OrderFull) error {
	if o.base == nil || o.orderState != Order_Closing {
		return ErrState
	}

	// seq finished
	if o.seq != nil && o.seqState != OrderSeq_Finish {
		return ErrState
	}

	o.orderState = Order_Done
	o.orderTime = time.Now().Unix()

	ns := &NonceState{
		Nonce: o.base.Nonce,
		Time:  o.orderTime,
		State: o.orderState,
	}

	val, err := cbor.Marshal(ns)
	if err != nil {
		return err
	}

	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, o.pro)
	err = m.ds.Put(key, val)
	if err != nil {
		return err
	}

	o.base = nil

	// trigger a new order
	if len(o.segs) > 0 && !o.inStop {
		go m.getQuotation(o.pro)
	}

	return nil
}

func (m *OrderMgr) stopOrder(o *OrderFull) {
	o.Lock()
	defer o.Unlock()
	o.inStop = true

	for i := 0; i < len(o.segs); i++ {
		m.redoSegJob(o.segs[i])
	}

	o.segs = o.segs[:0]
}

// create a new orderseq for prepare
func (m *OrderMgr) createSeq(o *OrderFull) error {
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}

	if o.seq == nil && o.seqState == OrderSeq_Finish {
		if len(o.segs) == 0 {
			return ErrEmpty
		}
		id := o.base.Hash()

		s := &types.OrderSeq{
			ID:     id,
			SeqNum: o.seqNum,
			Price:  new(big.Int).Set(o.seq.Price),
			Size:   o.seq.Size,
		}

		o.seq = s
		o.seqNum++
		o.seqState = OrderSeq_Prepare
		o.seqTime = time.Now().Unix()
	}

	if o.seq != nil && o.seqState == OrderSeq_Prepare {
		data, err := o.seq.Serialize()
		if err != nil {
			return err
		}

		key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
		err = m.ds.Put(key, data)
		if err != nil {
			return err
		}

		ss := SeqState{
			Number: o.seq.SeqNum,
			Time:   o.seqTime,
			State:  o.seqState,
		}
		key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, o.pro, o.base.Nonce)
		val, err := cbor.Marshal(ss)
		if err != nil {
			return err
		}

		err = m.ds.Put(key, val)
		if err != nil {
			return err
		}

		// send to pro
		go m.getNewSeqAck(o.pro, data)

		return nil
	}

	return ErrState
}

// init -> send; when receive confirm ack
func (m *OrderMgr) sendSeq(o *OrderFull, s *types.OrderSeq) error {
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}

	if o.seq != nil && o.seqState == OrderSeq_Prepare {
		o.seqState = OrderSeq_Send
		o.seqTime = time.Now().Unix()

		ss := SeqState{
			Number: o.seq.SeqNum,
			Time:   o.seqTime,
			State:  o.seqState,
		}
		key := store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, o.pro, o.base.Nonce)
		val, err := cbor.Marshal(ss)
		if err != nil {
			return err
		}

		err = m.ds.Put(key, val)
		if err != nil {
			return err
		}

		return nil
	}

	return ErrState
}

// time is up
func (m *OrderMgr) commitSeq(o *OrderFull) error {
	if o.base == nil || o.orderState == Order_Init || o.orderState == Order_Done {
		return ErrState
	}

	if o.seq == nil {
		return ErrState
	}

	if o.seqState == OrderSeq_Send {
		o.seqState = OrderSeq_Commit
		o.seqTime = time.Now().Unix()
		if o.inflight {
			return nil
		}
	}

	if o.seqState == OrderSeq_Commit {
		if o.inflight {
			return nil
		}

		o.seqTime = time.Now().Unix()
		ss := SeqState{
			Number: o.seq.SeqNum,
			Time:   o.seqTime,
			State:  o.seqState,
		}
		key := store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, o.pro, o.base.Nonce)
		val, err := cbor.Marshal(ss)
		if err != nil {
			return err
		}

		err = m.ds.Put(key, val)
		if err != nil {
			return err
		}

		data, err := o.seq.Serialize()
		if err != nil {
			return err
		}

		key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
		err = m.ds.Put(key, data)
		if err != nil {
			return err
		}

		// send to pro
		go m.getSeqFinishAck(o.pro, data)
	}

	return ErrState
}

// when recieve pro seq done ack; confirm -> done
func (m *OrderMgr) finishSeq(o *OrderFull, s *types.OrderSeq) error {
	if o.base == nil || o.orderState != Order_Closing {
		return ErrState
	}

	if o.seq == nil || o.seqState != OrderSeq_Commit {
		return ErrState
	}

	o.seqState = OrderSeq_Finish
	o.seqTime = time.Now().Unix()

	ss := SeqState{
		Number: o.seq.SeqNum,
		Time:   o.seqTime,
		State:  o.seqState,
	}
	key := store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, o.pro, o.base.Nonce)
	val, err := cbor.Marshal(ss)
	if err != nil {
		return err
	}

	err = m.ds.Put(key, val)
	if err != nil {
		return err
	}

	// trigger new seq
	return m.createSeq(o)
}
