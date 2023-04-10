package order

import (
	"context"
	"math/big"
	"time"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

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

// check each provider's state
func (m *OrderMgr) runOrderSched(of *proInst) {
	st := time.NewTicker(59 * time.Second)
	defer st.Stop()

	lt := time.NewTicker(10 * time.Minute)
	defer lt.Stop()

	for {
		select {
		case quo := <-of.quoChan:
			if quo.SegPrice.Cmp(m.segPrice) > 0 {
				of.failCnt++
			} else {
				of.failCnt = 0
				of.availTime = time.Now().Unix()
				err := m.createOrder(of, quo)
				if err != nil {
					logger.Debug("fail create new order: ", err)
				}
			}
		case ob := <-of.orderChan:
			of.availTime = time.Now().Unix()
			err := m.runOrder(of, ob)
			if err != nil {
				logger.Debug("fail run new order: ", ob.ProID, err)
			}
		case s := <-of.seqNewChan:
			of.availTime = time.Now().Unix()
			err := m.sendSeq(of, s.os)
			if err != nil {
				logger.Debug("fail send new seq: ", err)
			}
		case s := <-of.seqFinishChan:
			of.availTime = time.Now().Unix()
			err := m.finishSeq(of, s.os)
			if err != nil {
				logger.Debug("fail finish seq: ", err)
			}
		case <-st.C:
			m.check(of)
		case <-lt.C:
			ctx, cancle := context.WithTimeout(m.ctx, 10*time.Second)
			ri, err := m.RoleGet(ctx, of.pro, true)
			if err == nil {
				of.location = string(ri.GetDesc())
			}
			cancle()
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *OrderMgr) check(o *proInst) {
	nt := time.Now().Unix()

	logger.Debugf("check state pro:%d, nonce: %d, seq: %d, order state: %s, seq state: %s, jobs: %d, failCnt: %d, ready: %t, stop: %t", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState, o.jobCount(), o.failCnt, o.ready, o.inStop)

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
			err := m.stopOrder(o)
			logger.Warnf("stop order %d %s", o.pro, err)
		}
	}

	if o.failCnt > minFailCnt || o.failSent > minFailCnt {
		err := m.stopOrder(o)
		logger.Warnf("stop order %d due to fail too many times %s", o.pro, err)
		return
	}

	if !o.ready {
		return
	}

	switch o.orderState {
	case Order_Init:
		if o.jobCount() > 0 {
			o.orderTime = time.Now().Unix()
			go m.getQuotation(o.pro)
		}
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
			if o.jobCount() > 0 {
				err := m.createSeq(o)
				if err != nil {
					logger.Debugf("%d order fail due to create seq %d %s", o.pro, o.failCnt, err)
				}
				return
			}
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
		if o.jobCount() > 0 {
			o.orderState = Order_Init
		}
	}
}

func saveOrderBase(o *proInst, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderBaseKey, o.localID, o.pro, o.base.Nonce)
	data, err := o.base.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

func saveOrderState(o *proInst, ds store.KVStore) error {
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

func saveOrderSeq(o *proInst, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderSeqKey, o.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
	data, err := o.seq.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

func saveSeqState(o *proInst, ds store.KVStore) error {
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
	ds.Put(key, val)

	key = store.NewKey(pb.MetaType_OrderSeqNumKey, o.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
	return ds.Put(key, val)
}

func saveSeqJob(o *proInst, ds store.KVStore) error {
	key := store.NewKey(pb.MetaType_OrderSeqJobKey, o.localID, o.pro, o.base.Nonce, o.seq.SeqNum)
	data, err := o.sjq.Serialize()
	if err != nil {
		return err
	}
	return ds.Put(key, data)
}

// create a new order
func (m *OrderMgr) createOrder(o *proInst, quo *types.Quotation) error {
	logger.Debug("handle create order sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.inStop {
		return xerrors.Errorf("%d is stop", o.pro)
	}

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
		if end-start > 1000*types.Day {
			end -= types.Day
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
		if o.base.End-o.base.Start > build.OrderMax || o.base.End-o.base.Start < build.OrderMin {
			logger.Debugf("re-create order for %d at nonce %d", o.pro, o.nonce)
			o.orderState = Order_Init
			return nil
		}

		nt := time.Now().Unix()
		if (o.base.Start < nt && nt-o.base.Start > types.Hour/2) || (o.base.Start > nt && o.base.Start-nt > types.Hour/2) {
			logger.Debugf("re-create order for %d at nonce %d", o.pro, o.nonce)
			o.orderState = Order_Init
			return nil
		}

		// wait one hour
		if o.failCnt > 30 && o.base.Nonce == 0 {
			err := m.stopOrder(o)
			logger.Warnf("close order %d due to unable create new order %s", o.pro, err)
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
func (m *OrderMgr) runOrder(o *proInst, ob *types.SignedOrder) error {
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
func (m *OrderMgr) closeOrder(o *proInst) error {
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
			err := m.stopOrder(o)
			logger.Warnf("close empty order %d duo to end %s", o.pro, err)
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
func (m *OrderMgr) doneOrder(o *proInst) error {
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
	saveOrderState(o, m.ds)

	// save and reset
	o.orderState = Order_Init
	o.orderTime = time.Now().Unix()
	o.base.Nonce++
	saveOrderState(o, m.ds)

	o.seqState = OrderSeq_Init
	o.seqTime = time.Now().Unix()
	o.seq.SeqNum = 0
	saveSeqState(o, m.ds)

	m.updateBaseSize(o, o.base, true)

	o.base = nil
	o.seq = nil
	o.sjq = new(types.SegJobsQueue)
	o.seqNum = 0

	// trigger a new order
	if o.jobCount() > 0 && !o.inStop {
		go m.getQuotation(o.pro)
	}

	return nil
}

// clean up has some bugs
func (m *OrderMgr) stopOrder(o *proInst) error {
	logger.Debug("handle stop order sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	o.Lock()

	// data is sending, waiting
	if o.inflight {
		o.Unlock()
		return xerrors.Errorf("sending data")
	}

	o.inStop = true
	o.Unlock()

	cnt := uint64(0)
	nsjq := make([]*types.SegJob, 0, 128)

	// add redo current seq
	if o.sjq != nil {
		sLen := o.sjq.Len()
		ss := *o.sjq

		for i := 0; i < sLen; i++ {
			cnt += ss[i].Length
			nsjq = append(nsjq, ss[i])
			//m.redoSegJob(ss[i])
		}

		o.sjq = new(types.SegJobsQueue)
		if o.base != nil && o.seq != nil {
			saveSeqJob(o, m.ds)
		}
	}

	o.Lock()
	for _, bid := range o.buckets {
		bjob, ok := o.jobs[bid]
		if ok && len(bjob.jobs) > 0 {
			nsjq = append(nsjq, bjob.jobs...)
			bjob.jobs = bjob.jobs[:0]
		}
	}
	o.jobCnt = 0
	o.Unlock()

	for _, seq := range nsjq {
		m.segRedoChan <- seq
	}

	if o.base == nil {
		return xerrors.Errorf("order is empty at: %d", o.nonce)
	}

	// size is added to base, so reduce it here
	if o.seqState == OrderSeq_Commit && len(o.seq.UserDataSig.Data) != 0 {
		if o.seq.Size >= cnt*build.DefaultSegSize {
			pr := big.NewInt(int64(cnt))
			pr.Mul(pr, o.base.SegPrice)
			o.seq.Price.Sub(o.seq.Price, pr)
			o.seq.Size -= cnt * build.DefaultSegSize

			o.base.Size = o.seq.Size
			o.base.Price.Set(o.seq.Price)
		} else {
			logger.Warnf("order seq size is large: %d %d %d", o.seq.SeqNum, o.seq.Size, cnt*build.DefaultSegSize)
		}

		//return xerrors.Errorf("order seq size is large: %d %d %d", o.seq.SeqNum, o.seq.Size, cnt*build.DefaultSegSize)
	}

	// clean state; need test
	if o.base.Size == 0 {
		if o.seq == nil {
			return xerrors.Errorf("seq is empty")
		}

		// reset seq state
		if o.seq.Size == 0 && (o.seqState == OrderSeq_Send || o.seqState == OrderSeq_Commit) {
			// reset
			o.seq.Segments = types.AggSegsQueue{}
			o.seqState = OrderSeq_Send
			saveOrderSeq(o, m.ds)
		}

		return nil
	}

	if o.seq != nil {
		o.orderState = Order_Closing
		o.seqState = OrderSeq_Init
		return m.doneOrder(o)
	}

	return nil
}

// create a new orderseq for prepare
func (m *OrderMgr) createSeq(o *proInst) error {
	logger.Debug("handle create seq sat: ", o.pro, o.nonce, o.seqNum, o.orderState, o.seqState)
	if o.base == nil || o.orderState != Order_Running {
		return xerrors.Errorf("%d state: %s is not running", o.pro, o.orderState)
	}

	if o.seqState == OrderSeq_Init {
		// verify again
		if o.jobCount() == 0 {
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

func (m *OrderMgr) sendSeq(o *proInst, s *types.SignedOrderSeq) error {
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
func (m *OrderMgr) commitSeq(o *proInst) error {
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
func (m *OrderMgr) finishSeq(o *proInst, s *types.SignedOrderSeq) error {
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
