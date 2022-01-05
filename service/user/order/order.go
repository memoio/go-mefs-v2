package order

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
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

func (ns *NonceState) Serialize() ([]byte, error) {
	return cbor.Marshal(ns)
}

func (ns *NonceState) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ns)
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

func (ss *SeqState) Serialize() ([]byte, error) {
	return cbor.Marshal(ss)
}

func (ss *SeqState) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ss)
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
	quoretry  int   // todo: retry > 10; change pro?

	nonce  uint64 // next nonce
	seqNum uint32 // next seq

	base       *types.SignedOrder // quotation-> base
	orderTime  int64
	orderState OrderState
	segPrice   *big.Int

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

		localID:  m.localID,
		fsID:     m.fsID,
		pro:      id,
		quoretry: 1, // set to 0 when get desired quotation

		availTime: time.Now().Unix() - 300,

		buckets: make([]uint64, 0, 8),
		jobs:    make(map[uint64]*bucketJob),

		segDoneChan: m.segDoneChan,
	}

	err := m.connect(id)
	if err == nil {
		op.ready = true
	}

	go m.sendData(op)

	ns := new(NonceState)
	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, id)
	val, err := m.ds.Get(key)
	if err != nil {
		return op
	}
	err = ns.Deserialize(val)
	if err != nil {
		return op
	}

	ob := new(types.SignedOrder)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, id, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = ob.Deserialize(val)
	if err != nil {
		return op
	}

	op.base = ob
	op.orderTime = ns.Time
	op.orderState = ns.State
	if ns.State > Order_Wait {
		op.nonce = ns.Nonce + 1
	} else {
		op.nonce = ns.Nonce
	}

	op.segPrice = new(big.Int).Mul(ob.SegPrice, big.NewInt(build.DefaultSegSize))

	ss := new(SeqState)
	key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, id, ns.Nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = ss.Deserialize(val)
	if err != nil {
		return op
	}

	os := new(types.SignedOrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, id, ns.Nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = os.Deserialize(val)
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
	val, err := ns.Serialize()
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
	val, err := ss.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, val)
}

// create a new order
func (m *OrderMgr) createOrder(o *OrderFull, quo *types.Quotation) error {
	logger.Debug("handle create order: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	o.RLock()
	if o.inStop {
		o.RUnlock()
		return xerrors.Errorf("%d is stop", o.pro)
	}
	o.RUnlock()

	if o.orderState == Order_Init {
		// compare to set price
		if quo != nil && quo.SegPrice.Cmp(m.segPrice) > 0 {
			m.stopOrder(o)
			return xerrors.Errorf("price is not right, expected less than %d got %d", m.segPrice, quo.SegPrice)
		}

		start := time.Now().Unix()
		end := ((start+build.OrderDuration)/types.Day + 1) * types.Day

		o.base = &types.SignedOrder{
			OrderBase: types.OrderBase{
				UserID:     o.localID,
				ProID:      quo.ProID,
				Nonce:      o.nonce,
				TokenIndex: quo.TokenIndex,
				SegPrice:   quo.SegPrice,
				PiecePrice: quo.PiecePrice,
				Start:      start,
				End:        end,
			},
			Size:  0,
			Price: big.NewInt(0),
		}

		o.segPrice = new(big.Int).Mul(o.base.SegPrice, big.NewInt(build.DefaultSegSize))

		osig, err := m.RoleSign(m.ctx, m.localID, o.base.Hash(), types.SigSecp256k1)
		if err != nil {
			return err
		}
		o.base.Usign = osig

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
		nt := time.Now().Unix()
		if (o.base.Start < nt && nt-o.base.Start > types.Hour/2) || (o.base.Start > nt && o.base.Start-nt > types.Hour/2) {
			logger.Debugf("re-create order for %d at nonce %d", o.pro, o.nonce)
			o.orderState = Order_Init
			return nil
		}

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
	logger.Debug("handle run order: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Wait {
		return xerrors.Errorf("%d order state expectd %d, got %d", o.pro, Order_Wait, o.orderState)
	}

	// validate
	ok, err := m.RoleVerify(m.ctx, o.pro, o.base.Hash(), ob.Psign)
	if err != nil {
		return err
	}
	if !ok {
		return xerrors.Errorf("%d order sign is wrong", o.pro)
	}

	// nonce is add
	o.nonce++
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
	logger.Debug("handle close order: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Running {
		return xerrors.Errorf("%d order state expectd %d, got %d", o.pro, Order_Running, o.orderState)
	}

	if o.seq == nil || (o.seq != nil && o.seq.Size == 0) {
		// should not close empty seq
		logger.Debug("should not close empty order: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
		o.orderTime += 600
		return nil
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
	logger.Debug("handle done order: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	// order is closing
	if o.base == nil || o.orderState != Order_Closing {
		return xerrors.Errorf("%d order state expectd %d, got %d", o.pro, Order_Closing, o.orderState)
	}

	// seq finished
	if o.seq != nil && o.seqState != OrderSeq_Init {
		return xerrors.Errorf("%d order seq state expectd %d, got %d", o.pro, OrderSeq_Init, o.seqState)
	}

	ocp := tx.OrderCommitParas{
		UserID: o.base.UserID,
		ProID:  o.base.ProID,
		Nonce:  o.base.Nonce,
		SeqNum: o.seqNum,
	}

	data, err := ocp.Serialize()
	if err != nil {
		return err
	}

	msg := &tx.Message{
		Version: 0,
		From:    o.base.UserID,
		To:      o.base.ProID,
		Method:  tx.DataOrderCommit,
		Params:  data,
	}

	m.msgChan <- msg

	o.orderState = Order_Done
	o.orderTime = time.Now().Unix()

	// save nonce state
	err = saveOrderState(o, m.ds)
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
	logger.Debug("handle stop order: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
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
	logger.Debug("handle create seq: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Running {
		return xerrors.Errorf("%d state: %d %d is not running", o.pro, o.orderState, o.seqState)
	}

	if o.seqState == OrderSeq_Init {
		if !o.hasSeg() {
			return nil
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

	return xerrors.Errorf("create seq fail")
}

func (m *OrderMgr) sendSeq(o *OrderFull, s *types.SignedOrderSeq) error {
	logger.Debug("handle send seq: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Running {
		return xerrors.Errorf("%d order state expectd %d, got %d", o.pro, Order_Running, o.orderState)
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

	return xerrors.Errorf("send seq fail")
}

// time is up
func (m *OrderMgr) commitSeq(o *OrderFull) error {
	logger.Debug("handle commit seq: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState == Order_Init || o.orderState == Order_Wait || o.orderState == Order_Done {
		return xerrors.Errorf("%d order state got %d", o.pro, o.orderState)
	}

	if o.seq == nil {
		return xerrors.Errorf("%d order seq state got %d", o.pro, o.seqState)
	}

	if o.seqState == OrderSeq_Send {
		o.seqState = OrderSeq_Commit
		o.seqTime = time.Now().Unix()
	}

	if o.seqState == OrderSeq_Commit {
		o.RLock()
		if o.inflight {
			o.RUnlock()
			logger.Debug("order has running data", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
			return nil
		}
		o.RUnlock()

		o.seqTime = time.Now().Unix()

		o.seq.Segments.Merge()

		shash := o.seq.Hash()
		ssig, err := m.RoleSign(m.ctx, m.localID, shash.Bytes(), types.SigSecp256k1)
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

		// todo: add money

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

		data, err := o.seq.Serialize()
		if err != nil {
			return err
		}

		// send to pro
		go m.getSeqFinishAck(o.pro, data)

		return nil
	}

	return xerrors.Errorf("commit seq fail")
}

// when recieve pro seq done ack; confirm -> done
func (m *OrderMgr) finishSeq(o *OrderFull, s *types.SignedOrderSeq) error {
	logger.Debug("handle finish seq: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil {
		return xerrors.Errorf("order empty")
	}

	if o.seq == nil || o.seqState != OrderSeq_Commit {
		return xerrors.Errorf("%d order seq state expected %d got %d", o.pro, OrderSeq_Commit, o.seqState)
	}

	oHash := o.seq.Hash()
	ok, _ := m.RoleVerify(m.ctx, o.pro, oHash.Bytes(), s.ProDataSig)
	if !ok {
		return xerrors.Errorf("%d seq sign is wrong", o.pro)
	}

	ok, _ = m.RoleVerify(m.ctx, o.pro, o.base.Hash(), s.ProSig)
	if !ok {
		return xerrors.Errorf("%d order sign is wrong", o.pro)
	}

	o.seq.ProDataSig = s.ProDataSig
	o.seq.ProSig = s.ProSig

	// change state
	o.seqState = OrderSeq_Finish
	o.seqTime = time.Now().Unix()

	o.base.Price.Set(o.seq.Price)
	o.base.Size = o.seq.Size

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

	// push out

	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}

	logger.Debugf("pro %d order %d seq %d count %d length %d", o.pro, o.seq.Nonce, o.seq.SeqNum, o.seq.Segments.Len(), len(data))

	msg := &tx.Message{
		Version: 0,
		From:    m.localID,
		To:      o.pro,
		Method:  tx.DataOrder,
		Params:  data,
	}

	m.msgChan <- msg

	// reset
	o.seqState = OrderSeq_Init

	// trigger new seq
	return m.createSeq(o)
}
