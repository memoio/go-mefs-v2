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
	OrderSeq_Lock                         // incase data is inflight
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
	api.IDataService // segment

	ctx context.Context

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
	dataLock sync.Mutex

	segDoneChan chan *types.SegJob
}

func (m *OrderMgr) newOrder(id uint64) {
	of := m.loadOrder(id)
	m.proChan <- of
}

func (m *OrderMgr) loadOrder(id uint64) *OrderFull {
	op := &OrderFull{
		IDataService: m.IDataService,

		pro:     id,
		fsID:    m.fsID,
		localID: m.localID,

		ctx: m.ctx,

		segs: make([]*types.SegJob, 0, 16),

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
	op.orderState = ns.State
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
	op.seqState = ss.State
	op.seqTime = ss.Time

	return op
}

func (m *OrderMgr) check(o *OrderFull) {
	// 1. done -> new

	// 2  closing; push seq to done

	// 3  running;
	// 3.1 seq prepare: wait pro ack
	// 3.2 seq send: wait to commit;
	// 3.3 seq commit: wait pro ack
	// 3.4 seq done: trigger new seq

	// 4 init; wait confirm

	nt := time.Now().Unix()
	switch o.orderState {
	case Order_Init:
		if nt-o.orderTime > DefaultAckWaiting {
			m.init(o, nil)
		}
	case Order_Running:
		if nt-o.orderTime > DefaultOrderLast {
			m.close(o)
		}
		switch o.seqState {
		case OrderSeq_Prepare:
			if nt-o.seqTime > DefaultAckWaiting {
				m.prepare(o)
			}
		case OrderSeq_Send:
			if nt-o.seqTime > DefaultOrderSeqLast {
				m.commit(o)
			}
		case OrderSeq_Lock:
			if nt-o.seqTime > 10 {
				m.commit(o)
			}
		case OrderSeq_Commit:
			if nt-o.seqTime > DefaultAckWaiting {
				m.commit(o)
			}
		case OrderSeq_Finish:
		}

	case Order_Closing:
		if o.seqState == OrderSeq_Finish {
			m.done(o)
		}
	case Order_Done:
		o.dataLock.Lock()
		if len(o.segs) == 0 {
			o.dataLock.Unlock()
		}
		o.dataLock.Unlock()
		// init, inform higher
	}

	if nt-o.availTime > 1800 {
		go m.connect(o.pro)
	}

	if nt-o.availTime > 3600 {
		m.stop(o)
	}
}

// create a new order
func (m *OrderMgr) init(o *OrderFull, quo *types.Quotation) error {
	if quo != nil && o.base == nil && o.orderState == Order_Done {
		if quo.SegPrice.Cmp(m.segPrice) > 0 {
			m.stop(o)
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

		// send to pro
		data, err := o.base.Serialize()
		if err != nil {
			return err
		}

		go m.getNewOrderAck(o.pro, data)

		return nil
	}

	if o.base != nil && o.orderState == Order_Init {
		o.orderState = Order_Init
		o.orderTime = time.Now().Unix()

		// send to pro
		data, err := o.base.Serialize()
		if err != nil {
			return err
		}

		go m.getNewOrderAck(o.pro, data)

		return nil
	}

	return ErrState
}

// confirm base when receive pro ack; init -> running
func (m *OrderMgr) run(o *OrderFull, ob *types.OrderBase) error {
	if o.base == nil || o.orderState != Order_Init {
		return ErrState
	}

	o.orderState = Order_Running
	o.orderTime = time.Now().Unix()
	return nil
}

// time up to close current order
func (m *OrderMgr) close(o *OrderFull) error {
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}
	o.orderState = Order_Closing
	o.orderTime = time.Now().Unix()
	// save

	return nil
}

// finish all seqs
func (m *OrderMgr) done(o *OrderFull) error {
	if o.base == nil || o.orderState != Order_Closing {
		return ErrState
	}

	// verify current has done all seq
	o.orderState = Order_Done
	o.orderTime = time.Now().Unix()

	// save

	o.base = nil

	// trigger a new order

	if len(o.segs) > 0 {
		go m.getQuotation(o.pro)
	}

	return nil
}

func (m *OrderMgr) stop(o *OrderFull) {
	o.dataLock.Lock()
	o.inStop = true

	for i := 0; i < len(o.segs); i++ {
		m.redoSegJob(o.segs[i])
	}

	o.segs = o.segs[:0]
	o.dataLock.Unlock()
}

// create a new orderseq for prepare
func (m *OrderMgr) prepare(o *OrderFull) error {
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}

	if o.seq == nil && o.seqState == OrderSeq_Finish {
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

		// send to pro

		data, err := o.seq.Serialize()
		if err != nil {
			return err
		}

		go m.getNewSeqAck(o.pro, data)

		return nil
	}

	return ErrState
}

// init -> send; when receive confirm ack
func (m *OrderMgr) send(o *OrderFull, s *types.OrderSeq) error {
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}

	if o.seq != nil && o.seqState == OrderSeq_Prepare {
		o.seqState = OrderSeq_Send
		o.seqTime = time.Now().Unix()
		// send to pro
		return nil
	}

	return ErrState
}

// time is up
func (m *OrderMgr) commit(o *OrderFull) error {
	if o.base == nil || o.orderState == Order_Init || o.orderState == Order_Done {
		return ErrState
	}

	o.seqState = OrderSeq_Lock
	o.seqTime = time.Now().Unix()
	if o.inflight {
		return nil
	}

	// send to pro
	o.seqState = OrderSeq_Commit
	o.seqTime = time.Now().Unix()

	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}

	go m.getSeqFinishAck(o.pro, data)

	return nil
}

// when recieve pro seq done ack; confirm -> done
func (m *OrderMgr) finish(o *OrderFull, s *types.OrderSeq) error {
	if o.base == nil || o.orderState != Order_Closing {
		return ErrState
	}

	if o.seq == nil || o.seqState != OrderSeq_Commit {
		return ErrState
	}

	o.seqState = OrderSeq_Finish
	o.seqTime = time.Now().Unix()
	// persist

	// trigger new()
	return nil
}
