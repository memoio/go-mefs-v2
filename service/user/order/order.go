package order

import (
	"context"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderState string

const (
	Order_Init    OrderState = "init"    //
	Order_Wait    OrderState = "wait"    // wait pro ack -> running
	Order_Running OrderState = "running" // time up -> close
	Order_Closing OrderState = "closing" // all seq done -> done
	Order_Done    OrderState = "done"    // new data -> init
)

func (os OrderState) String() string {
	return " " + string(os)
}

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
	OrderSeq_Init    OrderSeqState = "init"
	OrderSeq_Prepare OrderSeqState = "prepare" // wait pro ack -> send
	OrderSeq_Send    OrderSeqState = "send"    // can add data and send; time up -> commit
	OrderSeq_Commit  OrderSeqState = "commit"  // wait pro ack -> finish
	OrderSeq_Finish  OrderSeqState = "finish"  // new data -> prepare
)

func (oss OrderSeqState) String() string {
	return " " + string(oss)
}

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

	localID  uint64
	fsID     []byte
	location string

	pro        uint64
	availTime  int64 // last connect time
	updateTime int64

	nonce   uint64 // next nonce
	seqNum  uint32 // next seq
	prevEnd int64

	opi *types.OrderPayInfo

	base       *types.SignedOrder // quotation-> base
	orderTime  int64
	orderState OrderState

	seq      *types.SignedOrderSeq
	seqTime  int64
	seqState OrderSeqState

	sjq *types.SegJobsQueue

	inflight bool // data is sending
	buckets  []uint64
	jobs     map[uint64]*bucketJob // buf and persist?

	failCnt int // TODO: retry > 10; change pro?

	ready  bool // ready for service; network is ok
	inStop bool // stop receiving data; duo to high price
}

func filterProList(id uint64) bool {
	if os.Getenv("PROLIST") != "" {
		prolist := strings.Split(os.Getenv("PROLIST"), ",")
		for _, pro := range prolist {
			proi, _ := strconv.Atoi(pro)
			if uint64(proi) == id {
				return false
			}
		}
		return true
	}
	return false
}

func (m *OrderMgr) newProOrder(id uint64) {
	if id == math.MaxUint64 {
		return
	}

	if filterProList(id) {
		return
	}

	m.lk.Lock()
	_, has := m.orders[id]
	if has {
		m.lk.Unlock()
		return
	}

	_, ok := m.inCreation[id]
	if ok {
		m.lk.Unlock()
		return
	}

	m.inCreation[id] = struct{}{}
	m.lk.Unlock()

	logger.Debug("create order sat: ", id)
	of := m.loadProOrder(id)
	logger.Debug("load order sat: ", of.pro, of.nonce, of.seqNum, of.orderState, of.seqState)
	err := m.loadUnfinished(of)
	logger.Debug("finish create order sat: ", id, err)
	// resend tx msg
	m.proChan <- of
}

func (m *OrderMgr) loadProOrder(id uint64) *OrderFull {
	op := &OrderFull{
		IDataService: m.IDataService,

		ctx: m.ctx,

		localID: m.localID,
		fsID:    m.fsID,
		pro:     id,

		availTime: time.Now().Unix() - 301,

		prevEnd: time.Now().Unix(),

		opi: &types.OrderPayInfo{
			NeedPay: big.NewInt(0),
			Paid:    big.NewInt(0),
		},

		orderState: Order_Init,
		seqState:   OrderSeq_Init,
		sjq:        new(types.SegJobsQueue),

		buckets: make([]uint64, 0, 8),
		jobs:    make(map[uint64]*bucketJob),
	}

	ri, err := m.RoleGet(m.ctx, id, true)
	if err == nil {
		op.location = string(ri.GetDesc())
	}

	err = m.connect(id)
	if err == nil {
		op.ready = true
	}

	// TODO: add getOrderRemote, getSeqRemote if local has missing
	if false {
		m.getOrderRemote(id)
		m.getSeqRemote(id)
	}

	go m.sendData(op)

	key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, id)
	val, err := m.ds.Get(key)
	if err == nil {
		op.opi.Deserialize(val)
	} // recal iter all orders

	ns := new(NonceState)
	key = store.NewKey(pb.MetaType_OrderNonceKey, m.localID, id)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = ns.Deserialize(val)
	if err != nil {
		return op
	}

	if ns.State == Order_Init || ns.State == Order_Wait {
		op.nonce = ns.Nonce
	} else {
		op.nonce = ns.Nonce + 1
	}

	op.orderTime = ns.Time
	op.orderState = ns.State

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
	op.prevEnd = ob.End

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

	if os.Size > op.base.Size {
		op.base.Size = os.Size
		op.base.Price.Set(os.Price)
	}

	key = store.NewKey(pb.MetaType_OrderSeqJobKey, m.localID, id, ns.Nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}
	err = op.sjq.Deserialize(val)
	if err != nil {
		return op
	}

	return op
}

func (m *OrderMgr) check(o *OrderFull) {
	nt := time.Now().Unix()

	logger.Debugf("check state pro:%d, nonce: %d, seq: %d, order state: %s, seq state: %s, jobs: %d, ready: %t, stop: %t", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState, o.segCount(), o.ready, o.inStop)

	if o.inStop {
		return
	}

	if nt-o.availTime < 300 {
		o.ready = true
	} else {
		// not connect if pro is inStop
		// connect pro every 10 miniute
		if nt-o.updateTime > 600 {
			o.updateTime = nt
			go m.update(o.pro)
		}

		if nt-o.availTime > 3600 {
			o.ready = false
		}

		// for test
		if nt-o.availTime > m.orderLast {
			m.stopOrder(o)
		}
	}

	if !o.ready {
		return
	}

	if o.failCnt > minFailCnt {
		logger.Warnf("close order %d due to fail too many times", o.pro)
		m.stopOrder(o)
		return
	}

	switch o.orderState {
	case Order_Init:
		o.RLock()
		if o.hasSeg() {
			o.orderTime = time.Now().Unix()
			go m.getQuotation(o.pro)
		}
		o.RUnlock()
	case Order_Wait:
		if nt-o.orderTime > defaultAckWaiting {
			o.failCnt++
			err := m.createOrder(o, nil)
			if err != nil {
				logger.Debugf("%d order fail due to create order %d %s", o.pro, o.failCnt, err)
			}
		}
	case Order_Running:
		if nt-o.orderTime > m.orderLast {
			err := m.closeOrder(o)
			if err != nil {
				logger.Debugf("%d order fail due to close order %d %s", o.pro, o.failCnt, err)
			}
			return
		}
		switch o.seqState {
		case OrderSeq_Init:
			o.RLock()
			if o.hasSeg() {
				err := m.createSeq(o)
				o.RUnlock()
				if err != nil {
					logger.Debugf("%d order fail due to create seq %d %s", o.pro, o.failCnt, err)
				}
				return
			}
			o.RUnlock()
		case OrderSeq_Prepare:
			// not receive callback
			if nt-o.seqTime > defaultAckWaiting {
				o.failCnt++
				err := m.createSeq(o)
				if err != nil {
					logger.Debugf("%d order fail due to create seq %d %s", o.pro, o.failCnt, err)
				}
			}
		case OrderSeq_Send:
			// time is up for next seq, no new data
			if nt-o.seqTime > m.seqLast {
				err := m.commitSeq(o)
				if err != nil {
					logger.Debugf("%d order fail due to commit seq %d %s", o.pro, o.failCnt, err)
				}
			}
		case OrderSeq_Commit:
			// not receive callback
			if nt-o.seqTime > defaultAckWaiting {
				o.failCnt++
				err := m.commitSeq(o)
				if err != nil {
					logger.Debugf("%d order fail due to commit seq %d %s", o.pro, o.failCnt, err)
				}
			}
		case OrderSeq_Finish:
			o.seqState = OrderSeq_Init
		}
	case Order_Closing:
		switch o.seqState {
		case OrderSeq_Send:
			err := m.commitSeq(o)
			if err != nil {
				logger.Debugf("%d order fail due to commit seq %d %s", o.pro, o.failCnt, err)
			}
		case OrderSeq_Commit:
			// not receive callback
			if nt-o.seqTime > defaultAckWaiting {
				o.failCnt++
				err := m.commitSeq(o)
				if err != nil {
					logger.Debugf("%d order fail due to commit seq in closing %d %s", o.pro, o.failCnt, err)
				}
			}
		case OrderSeq_Init, OrderSeq_Prepare, OrderSeq_Finish:
			o.seqState = OrderSeq_Init
			err := m.doneOrder(o)
			if err != nil {
				logger.Debugf("%d order fail due to done order %d %s", o.pro, o.failCnt, err)
			}
		}
	case Order_Done:
		o.RLock()
		if o.hasSeg() {
			o.orderState = Order_Init
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

	ds.Put(key, val)
	key = store.NewKey(pb.MetaType_OrderNonceKey, o.localID, o.pro, ns.Nonce)
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

func saveSeqJob(o *OrderFull, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderSeqJobKey, o.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
	data, err := o.sjq.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

// create a new order
func (m *OrderMgr) createOrder(o *OrderFull, quo *types.Quotation) error {
	logger.Debug("handle create order sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	o.RLock()
	if o.inStop {
		o.RUnlock()
		return xerrors.Errorf("%d is stop", o.pro)
	}
	o.RUnlock()

	if o.orderState == Order_Init {
		// compare to set price
		if quo != nil {
			if quo.SegPrice.Cmp(m.segPrice) > 0 {
				m.stopOrder(o)
				return xerrors.Errorf("price is too high, expected equal or less than %d got %d", m.segPrice, quo.SegPrice)
			}
			if quo.TokenIndex != 0 {
				m.stopOrder(o)
				return xerrors.Errorf("token index is not right, expected zero got %d", quo.TokenIndex)
			}
		}

		start := time.Now().Unix()
		end := ((start+int64(m.orderDur))/types.Day + 1) * types.Day
		if end < o.prevEnd {
			end = o.prevEnd
		}

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

		// wait one hour
		if o.failCnt > 30 && o.base.Nonce == 0 {
			logger.Warnf("close order %d due to unable create new order", o.pro)
			go m.stopOrder(o)
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
	logger.Debug("handle run order sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Wait {
		return xerrors.Errorf("%d order state expectd %s, got %s", o.pro, Order_Wait, o.orderState)
	}

	// validate
	ok, err := m.RoleVerify(m.ctx, o.pro, o.base.Hash(), ob.Psign)
	if err != nil {
		return err
	}
	if !ok {
		logger.Debug("order sign is wrong: ", ob)
		logger.Debug("order sign is wrong: ", o.base)
		return xerrors.Errorf("%d order sign is wrong", o.pro)
	}

	logger.Debug("create new order: ", o.base.ProID, o.base.Nonce, o.base.Start, o.base.End)

	// nonce is add
	o.nonce++
	o.prevEnd = ob.End
	o.orderState = Order_Running
	o.orderTime = time.Now().Unix()
	o.base.Psign = ob.Psign

	err = saveOrderBase(o, m.ds)
	if err != nil {
		return err
	}

	// save nonce state
	err = saveOrderState(o, m.ds)
	if err != nil {
		return err
	}

	data, err := o.base.Serialize()
	if err != nil {
		return err
	}

	msg := &tx.Message{
		Version: 0,
		From:    o.base.UserID,
		To:      o.base.ProID,
		Method:  tx.PreDataOrder,
		Params:  data,
	}

	m.msgChan <- msg

	logger.Debug("push msg: ", msg.From, msg.To, msg.Method, o.base.Nonce)

	o.failCnt = 0

	return nil
}

// time up to close current order
func (m *OrderMgr) closeOrder(o *OrderFull) error {
	logger.Debug("handle close order sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil {
		return xerrors.Errorf("%d order is empty", o.pro)
	}

	if o.orderState != Order_Running {
		return xerrors.Errorf("%d order state expectd %s, got %s", o.pro, Order_Running, o.orderState)
	}

	if o.seq == nil || (o.seq != nil && o.seq.Size == 0) {
		// should not close empty seq
		logger.Debug("should not close empty order: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
		if o.base.End > time.Now().Unix()+m.orderLast+600 {
			// not close order when data is empty
			o.orderTime = time.Now().Unix()
			err := saveOrderState(o, m.ds)
			if err != nil {
				return err
			}
			return nil
		} else {
			m.stopOrder(o)
		}
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
	logger.Debug("handle done order sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	// order is closing
	if o.base == nil || o.orderState != Order_Closing {
		return xerrors.Errorf("%d order state expectd %s, got %s", o.pro, Order_Closing, o.orderState)
	}

	if o.base.Size == 0 {
		return xerrors.Errorf("%d has empty data at order %d", o.pro, o.base.Nonce)
	}

	// seq finished
	if o.seq != nil && o.seqState != OrderSeq_Init {
		return xerrors.Errorf("%d order seq state expectd %s, got %s", o.pro, OrderSeq_Init, o.seqState)
	}

	if o.seq == nil || (o.seq != nil && o.seq.Size == 0) {
		return xerrors.Errorf("%d has empty data at order %d", o.pro, o.base.Nonce)
	}

	ocp := tx.OrderCommitParas{
		UserID: o.base.UserID,
		ProID:  o.base.ProID,
		Nonce:  o.base.Nonce,
		SeqNum: o.seqNum,
	}

	// last seq is not start, so use it
	if o.sjq.Len() == 0 && (o.base.Size == o.seq.Size) && len(o.seq.ProDataSig.Data) == 0 {
		ocp.SeqNum = o.seq.SeqNum
	}

	if ocp.SeqNum == 0 {
		return xerrors.Errorf("empty data at order: %d %d", o.base.ProID, o.base.Nonce)
	}

	data, err := ocp.Serialize()
	if err != nil {
		return err
	}

	msg := &tx.Message{
		Version: 0,
		From:    o.base.UserID,
		To:      o.base.ProID,
		Method:  tx.CommitDataOrder,
		Params:  data,
	}

	m.msgChan <- msg

	logger.Debug("push msg: ", msg.From, msg.To, msg.Method, o.base.Nonce, o.seqNum, o.base.Size, o.seq.Size)

	o.orderState = Order_Done
	o.orderTime = time.Now().Unix()

	// save nonce state
	err = saveOrderState(o, m.ds)
	if err != nil {
		return err
	}

	// save and reset
	o.orderState = Order_Init
	o.orderTime = time.Now().Unix()
	o.base.Nonce++
	saveOrderState(o, m.ds)

	o.seqState = OrderSeq_Init
	o.seqTime = time.Now().Unix()
	o.seq.SeqNum = 0
	saveSeqState(o, m.ds)

	m.sizelk.Lock()
	pay := new(big.Int).SetInt64(o.base.End - o.base.Start)
	pay.Mul(pay, o.base.Price)

	o.opi.Size += o.base.Size
	o.opi.NeedPay.Add(o.opi.NeedPay, pay)
	key := store.NewKey(pb.MetaType_OrderPayInfoKey, o.localID, o.pro)
	val, _ := o.opi.Serialize()
	m.ds.Put(key, val)

	m.opi.Size += o.base.Size
	m.opi.NeedPay.Add(m.opi.NeedPay, pay)
	key = store.NewKey(pb.MetaType_OrderPayInfoKey, o.localID)
	val, _ = m.opi.Serialize()
	m.ds.Put(key, val)
	m.sizelk.Unlock()

	o.base = nil
	o.seq = nil
	o.sjq = new(types.SegJobsQueue)
	o.seqNum = 0

	// trigger a new order
	if o.hasSeg() && !o.inStop {
		go m.getQuotation(o.pro)
	}

	return nil
}

func (m *OrderMgr) stopOrder(o *OrderFull) {
	logger.Debug("handle stop order sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	o.Lock()

	// data is sending, waiting
	if o.inflight {
		o.Unlock()
		return
	}

	o.inStop = true

	if o.base == nil {
		o.Unlock()
		return
	}

	cnt := uint64(0)
	// add redo current seq
	if o.sjq != nil {
		// should not, TODO: fix
		sLen := o.sjq.Len()
		ss := *o.sjq

		for i := 0; i < sLen; i++ {
			cnt += ss[i].Length
			m.redoSegJob(ss[i])
		}

		o.sjq = new(types.SegJobsQueue)
	}

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

	// size is added to base, so reduce it here
	if o.seqState == OrderSeq_Commit && len(o.seq.UserDataSig.Data) != 0 {
		if o.seq.Size < cnt*build.DefaultSegSize {
			return
		}

		pr := big.NewInt(int64(cnt))
		pr.Mul(pr, o.base.SegPrice)
		o.seq.Price.Sub(o.seq.Price, pr)
		o.seq.Size -= cnt * build.DefaultSegSize

		o.base.Size = o.seq.Size
		o.base.Price.Set(o.seq.Price)
	}

	// clean state; need test
	if o.base.Size == 0 {
		if o.seq == nil {
			return
		}

		// reset seq state
		if o.seq.Size == 0 && (o.seqState == OrderSeq_Send || o.seqState == OrderSeq_Commit) {
			// reset
			o.seq.Segments = types.AggSegsQueue{}
			o.seqState = OrderSeq_Send
			saveOrderSeq(o, m.ds)

			saveSeqJob(o, m.ds)
		}

		return
	}

	if o.seq != nil {
		o.orderState = Order_Closing
		o.seqState = OrderSeq_Init
		m.doneOrder(o)
	}
}

// create a new orderseq for prepare
func (m *OrderMgr) createSeq(o *OrderFull) error {
	logger.Debug("handle create seq sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Running {
		return xerrors.Errorf("%d state: %s is not running", o.pro, o.orderState)
	}

	if o.seqState == OrderSeq_Init {
		// verify again
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
		o.sjq = new(types.SegJobsQueue)
	}

	if o.seq != nil && o.seqState == OrderSeq_Prepare {
		o.seq.Segments.Merge()
		o.sjq.Merge()

		// save order seq
		err := saveOrderSeq(o, m.ds)
		if err != nil {
			return err
		}

		err = saveSeqJob(o, m.ds)
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
	logger.Debug("handle send seq sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Running {
		return xerrors.Errorf("%d order state expectd %s, got %s", o.pro, Order_Running, o.orderState)
	}

	if o.seq != nil && o.seqState == OrderSeq_Prepare {
		o.seqState = OrderSeq_Send
		o.seqTime = time.Now().Unix()

		logger.Debug("seq send at: ", o.pro, o.seq.Nonce, o.seq.SeqNum, o.seq.Size)

		// save seq state
		err := saveSeqState(o, m.ds)
		if err != nil {
			return err
		}

		o.failCnt = 0

		return nil
	}

	return xerrors.Errorf("send seq fail")
}

// time is up
func (m *OrderMgr) commitSeq(o *OrderFull) error {
	logger.Debug("handle commit seq sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState == Order_Init || o.orderState == Order_Wait || o.orderState == Order_Done {
		return xerrors.Errorf("%d order state got %s", o.pro, o.orderState)
	}

	if o.seq == nil {
		return xerrors.Errorf("%d order seq state got %s", o.pro, o.seqState)
	}

	if o.seqState == OrderSeq_Send {
		o.seqState = OrderSeq_Commit
		o.seqTime = time.Now().Unix()
		return nil
	}

	if o.seqState == OrderSeq_Commit {
		o.RLock()
		if o.inflight {
			o.RUnlock()
			logger.Debug("order has running data: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
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
	logger.Debug("handle finish seq sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil {
		return xerrors.Errorf("order empty at %d %d %d %s %s", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	}

	if o.seq == nil || o.seqState != OrderSeq_Commit {
		return xerrors.Errorf("%d order seq state expected %s got %s", o.pro, OrderSeq_Commit, o.seqState)
	}

	oHash := o.seq.Hash()
	ok, _ := m.RoleVerify(m.ctx, o.pro, oHash.Bytes(), s.ProDataSig)
	if !ok {
		logger.Debug("handle seqIn local: ", o.seq.Segments.Len(), o.seq)
		logger.Debug("handle seqIn remote: ", s.Segments.Len(), s)
		return xerrors.Errorf("%d has %d %d, got %d %d seq sign is wrong", o.pro, o.seq.Nonce, o.seq.SeqNum, s.Nonce, s.SeqNum)
	}

	ok, _ = m.RoleVerify(m.ctx, o.pro, o.base.Hash(), s.ProSig)
	if !ok {
		logger.Debug("handle order seqIn local: ", o.seq)
		logger.Debug("handle order seqIn remote: ", s)
		return xerrors.Errorf("%d has %d %d, got %d %d order sign is wrong", o.pro, o.seq.Nonce, o.seq.SeqNum, s.Nonce, s.SeqNum)
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

	err = saveOrderBase(o, m.ds)
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

	logger.Debug("end seq send at: ", o.pro, o.seq.Nonce, o.seq.SeqNum, o.seq.Size)

	logger.Debugf("pro %d order %d seq %d count %d length %d", o.pro, o.seq.Nonce, o.seq.SeqNum, o.seq.Segments.Len(), len(data))

	msg := &tx.Message{
		Version: 0,
		From:    m.localID,
		To:      o.pro,
		Method:  tx.AddDataOrder,
		Params:  data,
	}

	m.msgChan <- msg

	key := store.NewKey(pb.MetaType_OrderSeqJobKey, msg.From, msg.To)
	nval := &types.NonceSeq{
		Nonce:  o.seq.Nonce,
		SeqNum: o.seq.SeqNum,
	}

	nData, err := nval.Serialize()
	if err == nil {
		m.ds.Put(key, nData)
	}

	logger.Debug("push msg: ", msg.From, msg.To, msg.Method, o.base.Nonce, o.seq.Nonce, o.seq.SeqNum, o.seq.Size)

	// reset
	o.seqState = OrderSeq_Init
	o.failCnt = 0

	// trigger new seq
	return nil
}
