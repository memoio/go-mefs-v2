package order

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

type OrderState uint8

const (
	Order_Init    OrderState = iota //
	Order_Wait                      // wait pro ack -> running
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
	OrderSeq_Init    OrderSeqState = iota
	OrderSeq_Prepare               // wait pro ack -> send
	OrderSeq_Send                  // can add data and send; time up -> commit
	OrderSeq_Commit                // wait pro ack -> finish
	OrderSeq_Finish                // new data -> prepare
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

	base       *types.SignedOrder // quotation-> base
	orderTime  int64
	orderState OrderState

	seq      *types.SignedOrderSeq
	seqTime  int64
	seqState OrderSeqState

	inflight bool // data is sending
	inStop   bool // stop receiving data; duo to high price or long unavil
	buckets  []uint64
	jobs     map[uint64]*bucketJob // buf and persist?

	segDoneChan chan *types.SegJob

	ready bool
}

func (m *OrderMgr) newProOrder(id uint64) {
	logger.Debug("create order for provider: ", id)
	of := m.loadProOrder(id)
	m.loadUnfinished(of)
	// resend tx msg
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

		availTime: time.Now().Unix() - 300,

		buckets: make([]uint64, 0, 8),
		jobs:    make(map[uint64]*bucketJob),

		segDoneChan: m.segDoneChan,
	}

	err := m.connect(id)
	if err == nil {
		op.ready = true
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

	ob := new(types.SignedOrder)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, id, ns.Nonce)
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
	op.orderState = ns.State
	op.nonce = ns.Nonce + 1

	ss := new(SeqState)
	key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, id, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, ss)
	if err != nil {
		return op
	}

	os := new(types.SignedOrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, id, ns.Nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = cbor.Unmarshal(val, os)
	if err != nil {
		return op
	}

	op.seq = os
	op.seqState = ss.State
	op.seqTime = ss.Time
	op.seqNum = ss.Number + 1

	return op
}

func (m *OrderMgr) check(o *OrderFull) {
	nt := time.Now().Unix()

	if nt-o.availTime < 30 {
		o.ready = true
		o.inStop = false
	}

	if nt-o.availTime > 1800 {
		go m.update(o.pro)
	}

	if nt-o.availTime > 3600 {
		m.stopOrder(o)
	}

	if o.ready {
		logger.Debug("check state for: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState, o.segCount(), o.ready, o.inStop)
	}

	switch o.orderState {
	case Order_Init:
		o.RLock()
		if o.hasSeg() && !o.inStop {
			go m.getQuotation(o.pro)
			o.RUnlock()
			return
		}
		o.RUnlock()
	case Order_Wait:
		if nt-o.orderTime > DefaultAckWaiting {
			m.createOrder(o, nil)
		}
	case Order_Running:
		if nt-o.orderTime > DefaultOrderLast {
			m.closeOrder(o)
		}
		switch o.seqState {
		case OrderSeq_Init:
			o.RLock()
			if o.hasSeg() && !o.inStop {
				m.createSeq(o)
				o.RUnlock()
				return
			}
			o.RUnlock()
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
			o.seqState = OrderSeq_Init
		}
	case Order_Closing:
		switch o.seqState {
		case OrderSeq_Send:
			m.commitSeq(o)
		case OrderSeq_Commit:
			// not receive callback
			if nt-o.seqTime > DefaultAckWaiting {
				m.commitSeq(o)
			}
		case OrderSeq_Init, OrderSeq_Prepare, OrderSeq_Finish:
			o.seqState = OrderSeq_Init
			m.doneOrder(o)
		}
	case Order_Done:
		o.RLock()
		if o.hasSeg() && !o.inStop {
			o.orderState = Order_Init
			go m.getQuotation(o.pro)
			o.RUnlock()
			return
		}
		o.RUnlock()
	}
}

func saveOrderBase(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderBaseKey, o.localID, o.pro, o.base.Nonce)
	data, err := o.base.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

func saveOrderState(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderNonceKey, o.localID, o.pro)
	ns := &NonceState{
		Nonce: o.base.Nonce,
		Time:  o.orderTime,
		State: o.orderState,
	}
	val, err := cbor.Marshal(ns)
	if err != nil {
		return err
	}
	return ds.Put(key, val)
}

func saveOrderSeq(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderSeqKey, o.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

func saveSeqState(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderSeqNumKey, o.localID, o.pro, o.base.Nonce)
	ss := SeqState{
		Number: o.seq.SeqNum,
		Time:   o.seqTime,
		State:  o.seqState,
	}
	val, err := cbor.Marshal(ss)
	if err != nil {
		return err
	}
	return ds.Put(key, val)
}

// create a new order
func (m *OrderMgr) createOrder(o *OrderFull, quo *types.Quotation) error {
	logger.Debug("handle create order")
	o.RLock()
	if o.inStop {
		o.RUnlock()
		return ErrState
	}
	o.RUnlock()

	if o.orderState == Order_Init {
		// compare to set price
		if quo != nil && quo.SegPrice.Cmp(m.segPrice) > 0 {
			m.stopOrder(o)
			return ErrPrice
		}

		o.base = &types.SignedOrder{
			OrderBase: types.OrderBase{
				UserID:     o.localID,
				ProID:      quo.ProID,
				Nonce:      o.nonce,
				TokenIndex: quo.TokenIndex,
				SegPrice:   quo.SegPrice,
				PiecePrice: quo.PiecePrice,
				Start:      time.Now().Unix(),
				End:        time.Now().Unix() + 8640000,
			},
			Size:  0,
			Price: big.NewInt(0),
		}

		osig, err := m.RoleSign(m.ctx, m.localID, o.base.Hash(), types.SigSecp256k1)
		if err != nil {
			return err
		}
		o.base.Usign = osig

		o.nonce++
		o.orderState = Order_Wait
		o.orderTime = time.Now().Unix()

		// reset seq
		o.seqNum = 0
		o.seqState = OrderSeq_Init

		// save signed order base; todo
		err = saveOrderBase(o, m.ds)
		if err != nil {
			return err
		}

		// save nonce state
		err = saveOrderState(o, m.ds)
		if err != nil {
			return err
		}
	}

	if o.orderState == Order_Wait {
		// send to pro
		data, err := o.base.Serialize()
		if err != nil {
			return err
		}
		go m.getNewOrderAck(o.pro, data)
	}

	return nil
}

// confirm base when receive pro ack; init -> running
func (m *OrderMgr) runOrder(o *OrderFull, ob *types.SignedOrder) error {
	logger.Debug("handle run order")
	if o.base == nil || o.orderState != Order_Wait {
		return ErrState
	}

	// validate
	ok, err := m.RoleVerify(m.ctx, o.pro, o.base.Hash(), ob.Psign)
	if err != nil {
		return err
	}
	if !ok {
		return ErrDataSign
	}

	o.orderState = Order_Running
	o.orderTime = time.Now().Unix()

	o.base.Psign = ob.Psign

	// save signed order base; todo
	err = saveOrderBase(o, m.ds)
	if err != nil {
		return err
	}

	// save nonce state
	err = saveOrderState(o, m.ds)
	if err != nil {
		return err
	}

	// push out; todo
	data, err := o.base.Serialize()
	if err != nil {
		return err
	}

	msg := &tx.Message{
		Version: 0,
		From:    o.base.UserID,
		To:      o.base.ProID,
		Method:  tx.DataPreOrder,
		Params:  data,
	}

	m.msgChan <- msg

	return nil
}

// time up to close current order
func (m *OrderMgr) closeOrder(o *OrderFull) error {
	logger.Debug("handle close order")
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}

	o.orderState = Order_Closing
	o.orderTime = time.Now().Unix()

	// save nonce state
	err := saveOrderState(o, m.ds)
	if err != nil {
		return err
	}

	return nil
}

// finish all seqs
func (m *OrderMgr) doneOrder(o *OrderFull) error {
	logger.Debug("handle done order")
	// order is closing
	if o.base == nil || o.orderState != Order_Closing {
		return ErrState
	}

	// seq finished
	if o.seq != nil && o.seqState != OrderSeq_Init {
		return ErrState
	}

	o.orderState = Order_Done
	o.orderTime = time.Now().Unix()

	// save nonce state
	err := saveOrderState(o, m.ds)
	if err != nil {
		return err
	}

	// reset
	o.base = nil
	o.orderState = Order_Init

	o.seq = nil
	o.seqNum = 0
	o.seqState = OrderSeq_Init

	// trigger a new order
	if o.hasSeg() && !o.inStop {
		go m.getQuotation(o.pro)
	}

	return nil
}

func (m *OrderMgr) stopOrder(o *OrderFull) {
	logger.Debug("handle stop order")
	o.Lock()
	o.inStop = true
	for _, bid := range o.buckets {
		bjob, ok := o.jobs[bid]
		if ok {
			for _, seg := range bjob.jobs {
				m.redoSegJob(seg)
			}
		}
		bjob.jobs = bjob.jobs[:0]
	}
	o.Unlock()

	m.closeOrder(o)
}

// create a new orderseq for prepare
func (m *OrderMgr) createSeq(o *OrderFull) error {
	logger.Debug("handle create seq")
	if o.base == nil || o.orderState != Order_Running {
		return xerrors.Errorf("state: %d %d %w", o.orderState, o.seqState, ErrState)
	}

	if o.seqState == OrderSeq_Init {
		if !o.hasSeg() {
			return ErrEmpty
		}

		s := &types.SignedOrderSeq{
			OrderSeq: types.OrderSeq{
				UserID: o.base.UserID,
				ProID:  o.base.ProID,
				Nonce:  o.base.Nonce,
				SeqNum: o.seqNum,
				Price:  new(big.Int).Set(o.base.Price),
				Size:   o.base.Size,
			},
		}

		o.seq = s
		o.seqNum++
		o.seqState = OrderSeq_Prepare
		o.seqTime = time.Now().Unix()
	}

	if o.seq != nil && o.seqState == OrderSeq_Prepare {
		o.seq.Segments.Merge()

		// save order seq
		err := saveOrderSeq(o, m.ds)
		if err != nil {
			return err
		}

		// save seq state
		err = saveSeqState(o, m.ds)
		if err != nil {
			return err
		}

		data, err := o.seq.Serialize()
		if err != nil {
			return err
		}

		// send to pro
		go m.getNewSeqAck(o.pro, data)

		return nil
	}

	return ErrState
}

func (m *OrderMgr) sendSeq(o *OrderFull, s *types.SignedOrderSeq) error {
	logger.Debug("handle send seq")
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}

	if o.seq != nil && o.seqState == OrderSeq_Prepare {
		o.seqState = OrderSeq_Send
		o.seqTime = time.Now().Unix()

		// save seq state
		err := saveSeqState(o, m.ds)
		if err != nil {
			return err
		}

		return nil
	}

	return ErrState
}

// time is up
func (m *OrderMgr) commitSeq(o *OrderFull) error {
	logger.Debug("handle commit seq")
	if o.base == nil || o.orderState == Order_Init || o.orderState == Order_Wait || o.orderState == Order_Done {
		return ErrState
	}

	if o.seq == nil {
		return ErrState
	}

	if o.seqState == OrderSeq_Send {
		o.seqState = OrderSeq_Commit
		o.seqTime = time.Now().Unix()
	}

	if o.seqState == OrderSeq_Commit {
		if o.inflight {
			return nil
		}

		o.seqTime = time.Now().Unix()

		o.seq.Segments.Merge()

		shash, err := o.seq.Hash()
		if err != nil {
			return err
		}
		ssig, err := m.RoleSign(m.ctx, m.localID, shash, types.SigSecp256k1)
		if err != nil {
			return err
		}

		o.base.Size = o.seq.Size
		o.base.Price.Set(o.seq.Price)
		osig, err := m.RoleSign(m.ctx, m.localID, o.base.Hash(), types.SigSecp256k1)
		if err != nil {
			return err
		}

		o.seq.UserDataSig = ssig
		o.seq.UserSig = osig

		// save order seq
		// save seq state
		err = saveOrderSeq(o, m.ds)
		if err != nil {
			return err
		}

		// save seq state
		err = saveSeqState(o, m.ds)
		if err != nil {
			return err
		}

		data, err := o.seq.Serialize()
		if err != nil {
			return err
		}

		// send to pro
		go m.getSeqFinishAck(o.pro, data)

		return nil
	}

	return ErrState
}

// when recieve pro seq done ack; confirm -> done
func (m *OrderMgr) finishSeq(o *OrderFull, s *types.SignedOrderSeq) error {
	if o.base == nil {
		return ErrState
	}

	if o.seq == nil || o.seqState != OrderSeq_Commit {
		return ErrState
	}

	oHash, err := o.seq.Hash()
	if err != nil {
		return err
	}
	ok, _ := m.RoleVerify(m.ctx, o.pro, oHash, s.ProDataSig)
	if !ok {
		return ErrDataSign
	}

	ok, _ = m.RoleVerify(m.ctx, o.pro, o.base.Hash(), s.ProSig)
	if !ok {
		return ErrDataSign
	}

	o.seq.ProDataSig = s.ProDataSig
	o.seq.ProSig = s.ProSig

	// change state
	o.seqState = OrderSeq_Finish
	o.seqTime = time.Now().Unix()

	o.base.Price.Set(o.seq.Price)
	o.base.Size = o.seq.Size

	// save order seq
	err = saveOrderSeq(o, m.ds)
	if err != nil {
		return err
	}

	// save seq state
	err = saveSeqState(o, m.ds)
	if err != nil {
		return err
	}

	// push out

	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}

	msg := &tx.Message{
		Version: 0,
		From:    m.localID,
		To:      m.localID,
		Method:  tx.DataOrder,
		Params:  data,
	}

	m.msgChan <- msg

	// reset
	o.seqState = OrderSeq_Init
	o.seq = nil

	// trigger new seq
	return m.createSeq(o)
}
