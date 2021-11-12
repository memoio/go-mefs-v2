package order

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/zeebo/blake3"
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
	api.IRole        // sign and veirfy
	api.INetService  // netmessage
	api.IDataService // segment

	ctx context.Context
	ds  store.KVStore

	localID  uint64
	fsID     []byte
	segPrice *big.Int // price set

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

	segDoneChan chan *types.SegJob // info higher
	segRedoChan chan *types.SegJob // info higher to redispatch

	quoChan       chan *types.Quotation // to init
	orderChan     chan *types.OrderBase // confirm new order
	seqNewChan    chan *types.OrderSeq  // confirm new seq
	seqFinishChan chan *types.OrderSeq  // confirm current seq
}

func (m *OrderMgr) newOrder(id uint64) *OrderFull {
	op := &OrderFull{
		IRole:        m.IRole,
		INetService:  m.INetService,
		IDataService: m.IDataService,

		pro:     id,
		fsID:    m.fsID,
		localID: m.localID,

		ds:       m.ds,
		segPrice: m.segPrice,

		ctx: m.ctx,

		segs: make([]*types.SegJob, 0, 16),

		segDoneChan: m.segDoneChan,
		segRedoChan: m.segRedoChan,

		quoChan:       make(chan *types.Quotation),
		orderChan:     make(chan *types.OrderBase),
		seqNewChan:    make(chan *types.OrderSeq),
		seqFinishChan: make(chan *types.OrderSeq),
	}

	go op.check()
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

func (o *OrderFull) check() {
	// 1. done -> new

	// 2  closing; push seq to done

	// 3  running;
	// 3.1 seq prepare: wait pro ack
	// 3.2 seq send: wait to commit;
	// 3.3 seq commit: wait pro ack
	// 3.4 seq done: trigger new seq

	// 4 init; wait confirm

	t := time.NewTicker(30 * time.Second)
	defer t.Stop()

	for {
		select {
		case quo := <-o.quoChan:
			o.init(quo)
		case ob := <-o.orderChan:
			o.run(ob)
		case s := <-o.seqNewChan:
			o.send(s)
		case s := <-o.seqFinishChan:
			o.finish(s)
		case <-t.C:
			nt := time.Now().Unix()
			switch o.orderState {
			case Order_Init:
				if nt-o.orderTime > DefaultAckWaiting {
					o.init(nil)
					continue
				}
			case Order_Running:
				if nt-o.orderTime > DefaultOrderLast {
					o.close()
					continue
				}
				switch o.seqState {
				case OrderSeq_Prepare:
					if nt-o.seqTime > DefaultAckWaiting {
						o.prepare()
					}
				case OrderSeq_Send:
					if nt-o.seqTime > DefaultOrderSeqLast {
						o.commit()
						continue
					}
				case OrderSeq_Lock:
					if nt-o.seqTime > 10 {
						o.commit()
						continue
					}
				case OrderSeq_Commit:
					if nt-o.seqTime > DefaultAckWaiting {
						o.commit()
					}
				case OrderSeq_Finish:
				}

			case Order_Closing:
				if o.seqState == OrderSeq_Finish {
					o.done()
				}
			case Order_Done:
				o.dataLock.Lock()
				if len(o.segs) == 0 {
					o.dataLock.Unlock()
					continue
				}
				o.dataLock.Unlock()
				// init, inform higher
			}

			if nt-o.availTime > 1800 {
				go o.connect()
			}

			if nt-o.availTime > 3600 {
				o.stop()
			}
		case <-o.ctx.Done():
			return
		}
	}
}

func (o *OrderFull) addSeg(sj *types.SegJob) error {

	o.dataLock.Lock()
	if o.inStop {
		o.dataLock.Unlock()
		return ErrState
	}
	o.segs = append(o.segs, sj)
	o.dataLock.Unlock()

	return nil
}

func (o *OrderFull) sendData() {
	for {
		select {
		case <-o.ctx.Done():
			return
		default:
			if o.base == nil || o.orderState != Order_Running {
				continue
			}

			if o.seq == nil || o.seqState != OrderSeq_Send {
				continue
			}

			o.dataLock.Lock()
			if len(o.segs) > 0 {
				o.dataLock.Unlock()
				continue
			}
			sj := o.segs[0]
			o.inflight = true
			o.dataLock.Unlock()

			sid, err := segment.NewSegmentID(o.fsID, sj.BucketID, sj.Start, sj.ChunkID)
			if err != nil {
				o.dataLock.Lock()
				o.inflight = false
				o.dataLock.Unlock()
				continue
			}
			err = o.SendSegmentByID(o.ctx, sid, o.pro)
			if err != nil {
				o.dataLock.Lock()
				o.inflight = false
				o.dataLock.Unlock()
				continue
			}

			o.availTime = time.Now().Unix()

			o.dataLock.Lock()
			o.seq.DataName = append(o.seq.DataName, sid.Bytes())
			// update price and size
			o.segs = o.segs[1:]
			o.inflight = false
			o.dataLock.Unlock()
		}
	}
}

// create a new order
func (o *OrderFull) init(quo *types.Quotation) error {
	if quo != nil && o.base == nil && o.orderState == Order_Done {
		if quo.SegPrice.Cmp(o.segPrice) > 0 {
			o.stop()
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

		go o.getNewOrderAck()
		// send to pro

		return nil
	}

	if o.base != nil && o.orderState == Order_Init {
		o.orderState = Order_Init
		o.orderTime = time.Now().Unix()

		go o.getNewOrderAck()
		// send to pro

		return nil
	}

	return ErrState
}

// confirm base when receive pro ack; init -> running
func (o *OrderFull) run(ob *types.OrderBase) error {
	if o.base == nil || o.orderState != Order_Init {
		return ErrState
	}

	o.orderState = Order_Running
	o.orderTime = time.Now().Unix()
	return nil
}

// time up to close current order
func (o *OrderFull) close() error {
	if o.base == nil || o.orderState != Order_Running {
		return ErrState
	}
	o.orderState = Order_Closing
	o.orderTime = time.Now().Unix()
	// save

	return nil
}

// finish all seqs
func (o *OrderFull) done() error {
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
		go o.getQuotation()
	}

	return nil
}

func (o *OrderFull) stop() {
	o.dataLock.Lock()
	o.inStop = true

	for i := 0; i < len(o.segs); i++ {
		o.segRedoChan <- o.segs[i]
	}

	o.segs = o.segs[:0]
	o.dataLock.Unlock()
}

// create a new orderseq for prepare
func (o *OrderFull) prepare() error {
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
		go o.getNewSeqAck()

		return nil
	}

	return ErrState
}

// init -> send; when receive confirm ack
func (o *OrderFull) send(s *types.OrderSeq) error {
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
func (o *OrderFull) commit() error {
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

	go o.getSeqFinishAck()

	return nil
}

// when recieve pro seq done ack; confirm -> done
func (o *OrderFull) finish(s *types.OrderSeq) error {
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

func (o *OrderFull) connect() {
	_, err := o.SendMetaRequest(o.ctx, o.pro, pb.NetMessage_SayHello, nil, nil)
	if err != nil {
		return
	}

	o.availTime = time.Now().Unix()
}

func (o *OrderFull) getQuotation() error {
	resp, err := o.SendMetaRequest(o.ctx, o.pro, pb.NetMessage_AskPrice, nil, nil)
	if err != nil {
		return err
	}

	if resp.GetHeader().GetFrom() != o.pro {
		return ErrState
	}

	o.availTime = time.Now().Unix()

	quo := new(types.Quotation)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), quo)
	if err != nil {
		return err
	}

	sig := new(types.Signature)
	err = sig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	// verify

	msg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := o.RoleVerify(o.pro, msg[:], *sig)
	if ok {
		o.quoChan <- quo
	}

	return nil
}

func (o *OrderFull) getNewOrderAck() error {
	if o.base == nil || o.orderState != Order_Init {
		return ErrState
	}

	data, err := o.base.Serialize()
	if err != nil {
		return err
	}

	msg := blake3.Sum256(data)
	sig, err := o.RoleSign(msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := o.SendMetaRequest(o.ctx, o.pro, pb.NetMessage_CreateOrder, data, sigByte)
	if err != nil {
		return err
	}

	o.availTime = time.Now().Unix()

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return ErrNotFound
	}

	ob := new(types.OrderBase)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), ob)
	if err != nil {
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := o.RoleVerify(o.pro, pmsg[:], *psig)
	if ok {
		o.orderChan <- ob
	}

	return nil
}

func (o *OrderFull) getNewSeqAck() error {
	if o.seq == nil || o.seqState != OrderSeq_Prepare {
		return ErrState
	}

	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}

	msg := blake3.Sum256(data)
	sig, err := o.RoleSign(msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := o.SendMetaRequest(o.ctx, o.pro, pb.NetMessage_CreateSeq, data, sigByte)
	if err != nil {
		return err
	}

	o.availTime = time.Now().Unix()

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return ErrNotFound
	}

	os := new(types.OrderSeq)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), os)
	if err != nil {
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := o.RoleVerify(o.pro, pmsg[:], *psig)
	if ok {
		o.seqNewChan <- os
	}

	return nil
}

func (o *OrderFull) getSeqFinishAck() error {
	if o.seq == nil || o.seqState != OrderSeq_Commit {
		return ErrState
	}

	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}

	msg := blake3.Sum256(data)
	sig, err := o.RoleSign(msg[:], types.SigSecp256k1)
	if err != nil {
		return err
	}

	sigByte, err := sig.Serialize()
	if err != nil {
		return err
	}

	resp, err := o.SendMetaRequest(o.ctx, o.pro, pb.NetMessage_FinishSeq, data, sigByte)
	if err != nil {
		return err
	}

	o.availTime = time.Now().Unix()

	if resp.GetHeader().GetType() == pb.NetMessage_Err {
		return ErrNotFound
	}

	os := new(types.OrderSeq)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), os)
	if err != nil {
		return err
	}

	psig := new(types.Signature)
	err = psig.Deserialize(resp.GetData().GetSign())
	if err != nil {
		return err
	}

	pmsg := blake3.Sum256(resp.GetData().GetMsgInfo())
	ok := o.RoleVerify(o.pro, pmsg[:], *psig)
	if ok {
		o.seqFinishChan <- os
	}

	return nil
}
