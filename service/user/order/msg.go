package order

import (
	"context"
	"encoding/binary"
	"math"
	"math/big"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (m *OrderMgr) runPush() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case msg := <-m.msgChan:
			m.pushMessage(msg)
		}
	}
}

func (m *OrderMgr) runCheck() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// run once at start
	go m.checkBalance()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			go m.checkBalance()
		}
	}
}

func (m *OrderMgr) checkBalance() {
	if m.inCheck {
		return
	}
	m.inCheck = true
	defer func() {
		m.inCheck = false
	}()

	needPay := big.NewInt(0)
	pros, err := m.StateGetProsAt(m.ctx, m.localID)
	if err != nil {
		return
	}
	for _, proID := range pros {
		ns, err := m.StateGetOrderNonce(m.ctx, m.localID, proID, math.MaxUint64)
		if err != nil {
			continue
		}
		si, err := m.is.SettleGetStoreInfo(m.ctx, m.localID, proID)
		if err != nil {
			logger.Debug("fail to get order info in chain", m.localID, proID, err)
			continue
		}

		logger.Debugf("user %d pro %d has order %d %d %d", m.localID, proID, si.Nonce, si.SubNonce, ns.Nonce)
		for i := si.Nonce; i < ns.Nonce; i++ {
			logger.Debugf("user %d pro %d add order %d %d", m.localID, proID, i, ns.Nonce)

			// load order
			ob := new(types.SignedOrder)
			key := store.NewKey(pb.MetaType_OrderBaseKey, m.localID, proID, i)
			val, err := m.ds.Get(key)
			if err != nil {
				logger.Debugf("user %d pro %d add order %d %d fail %s", m.localID, proID, i, ns.Nonce, err)
				continue
			}
			err = ob.Deserialize(val)
			if err != nil {
				logger.Debugf("user %d pro %d add order %d %d fail %s", m.localID, proID, i, ns.Nonce, err)
				continue
			}

			ss := new(SeqState)
			key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, proID, i)
			val, err = m.ds.Get(key)
			if err != nil {
				logger.Debugf("user %d pro %d add order %d %d fail %s", m.localID, proID, i, ns.Nonce, err)
				continue
			}
			err = ss.Deserialize(val)
			if err != nil {
				logger.Debugf("user %d pro %d add order %d %d fail %s", m.localID, proID, i, ns.Nonce, err)
				continue
			}

			os := new(types.SignedOrderSeq)
			key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, proID, i, ss.Number)
			val, err = m.ds.Get(key)
			if err != nil {
				logger.Debugf("user %d pro %d add order %d %d fail %s", m.localID, proID, i, ns.Nonce, err)
				continue
			}
			err = os.Deserialize(val)
			if err != nil {
				logger.Debugf("user %d pro %d add order %d %d fail %s", m.localID, proID, i, ns.Nonce, err)
				continue
			}
			os.Price.Mul(os.Price, big.NewInt(ob.End-ob.Start))
			needPay.Add(needPay, os.Price)
		}
	}

	needPay.Mul(needPay, big.NewInt(12))
	needPay.Div(needPay, big.NewInt(10))

	bal, err := m.is.SettleGetBalanceInfo(m.ctx, m.localID)
	if err != nil {
		return
	}

	if bal.FsValue.Cmp(needPay) < 0 {
		m.is.SettleCharge(m.ctx, needPay)
	}

	// after recharge
	// submit orders here
	m.submitOrders()
}

func (m *OrderMgr) pushMessage(msg *tx.Message) {
	var mid types.MsgID
	for {
		id, err := m.PushMessage(m.ctx, msg)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		mid = id
		break
	}

	go func(mid types.MsgID) {
		ctx, cancle := context.WithTimeout(m.ctx, 30*time.Minute)
		defer cancle()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			st, err := m.SyncGetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height)
				switch msg.Method {
				case tx.AddDataOrder:
					// confirm
					logger.Debug("confirm jobs in order seq: ", msg.From, msg.To)

					seq := new(types.SignedOrderSeq)
					err := seq.Deserialize(msg.Params)
					if err != nil {
						return
					}

					logger.Debug("confirm jobs in order seq: ", msg.From, msg.To, seq.Nonce, seq.SeqNum)

					m.ReplaceSegWithLoc(seq.OrderSeq)
					m.updateSize(seq.OrderSeq)
				default:
				}
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}

			break
		}
	}(mid)
}

func (m *OrderMgr) loadUnfinished(of *OrderFull) error {
	ns, err := m.StateGetOrderNonce(m.ctx, of.localID, of.pro, math.MaxUint64)
	if err != nil {
		return err
	}

	logger.Debug("re-confirm jobs: ", of.localID, of.pro, ns.Nonce, ns.SeqNum)
	key := store.NewKey(pb.MetaType_OrderSeqJobKey, of.localID, of.pro)
	nData, err := m.ds.Get(key)
	if err == nil {
		nval := new(types.NonceSeq)
		err = nval.Deserialize(nData)
		if err == nil {
			if ns.Nonce > nval.Nonce || (ns.Nonce == nval.Nonce && ns.SeqNum > nval.SeqNum) {
				key := store.NewKey(pb.MetaType_OrderSeqKey, of.localID, of.pro, nval.Nonce, nval.SeqNum)
				data, err := m.ds.Get(key)
				if err != nil {
					return xerrors.Errorf("found %s error %d", string(key), err)
				}
				seq := new(types.SignedOrderSeq)
				err = seq.Deserialize(data)
				if err != nil {
					return err
				}

				logger.Debug("re-confirm jobs in order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)

				m.ReplaceSegWithLoc(seq.OrderSeq)
				m.updateSize(seq.OrderSeq)
			}
		}
	}

	logger.Debug("resend message for: ", of.pro, ", has: ", ns.Nonce, ns.SeqNum, ", want: ", of.nonce, of.seqNum)

	for ns.Nonce < of.nonce {
		// add order base
		if ns.SeqNum == 0 {
			_, err := m.StateGetOrder(m.ctx, of.localID, of.pro, ns.Nonce)
			if err != nil {
				key := store.NewKey(pb.MetaType_OrderBaseKey, of.localID, of.pro, ns.Nonce)
				data, err := m.ds.Get(key)
				if err != nil {
					return xerrors.Errorf("found %s error %d", string(key), err)
				}

				ob := new(types.SignedOrder)
				err = ob.Deserialize(data)
				if err != nil {
					return err
				}

				// reset size and price
				ob.Size = 0
				ob.Price = new(big.Int)
				data, err = ob.Serialize()
				if err != nil {
					return err
				}

				msg := &tx.Message{
					Version: 0,
					From:    of.localID,
					To:      of.pro,
					Method:  tx.PreDataOrder,
					Params:  data,
				}

				m.msgChan <- msg

				logger.Debug("push msg: ", msg.From, msg.To, msg.Method, ns.Nonce)
			}
		}

		ss := new(SeqState)
		key := store.NewKey(pb.MetaType_OrderSeqNumKey, of.localID, of.pro, ns.Nonce)
		val, err := m.ds.Get(key)
		if err != nil {
			return xerrors.Errorf("found %s error %d", string(key), err)
		}
		err = ss.Deserialize(val)
		if err != nil {
			return err
		}

		logger.Debug("resend message for: ", of.pro, ", has: ", ns.Nonce, ns.SeqNum, ss.Number, ss.State)

		nextSeq := uint32(ns.SeqNum)
		// add seq
		for i := uint32(ns.SeqNum); i <= ss.Number; i++ {
			// wait it finish?
			if i == ss.Number {
				if ss.State != OrderSeq_Finish {
					break
				}
			}

			key := store.NewKey(pb.MetaType_OrderSeqKey, of.localID, of.pro, ns.Nonce, i)
			data, err := m.ds.Get(key)
			if err != nil {
				return xerrors.Errorf("found %s error %d", string(key), err)
			}

			// verify last one
			if i == ss.Number {
				os := new(types.SignedOrderSeq)
				err = os.Deserialize(data)
				if err != nil {
					return err
				}

				if os.ProDataSig.Type == 0 || os.ProSig.Type == 0 {
					return xerrors.Errorf("pro sig empty %d %d", os.ProDataSig.Type, os.ProSig.Type)
				}
			}

			msg := &tx.Message{
				Version: 0,
				From:    of.localID,
				To:      of.pro,
				Method:  tx.AddDataOrder,
				Params:  data,
			}

			m.msgChan <- msg

			key = store.NewKey(pb.MetaType_OrderSeqJobKey, msg.From, msg.To)
			nval := &types.NonceSeq{
				Nonce:  ns.Nonce,
				SeqNum: i,
			}

			nData, err := nval.Serialize()
			if err == nil {
				m.ds.Put(key, nData)
			}

			nextSeq = i + 1

			logger.Debug("push msg: ", msg.From, msg.To, msg.Method, ns.Nonce, i)
		}

		// check order state before commit
		if ns.Nonce+1 == of.nonce {
			nns := new(NonceState)
			key = store.NewKey(pb.MetaType_OrderNonceKey, m.localID, of.pro, ns.Nonce)
			val, err = m.ds.Get(key)
			if err != nil {
				return xerrors.Errorf("found %s error %d", string(key), err)
			}
			err = nns.Deserialize(val)
			if err != nil {
				return err
			}

			if nns.State != Order_Done {
				return xerrors.Errorf("order state %s is not done at %d", nns.State, ns.Nonce)
			}
		}

		ocp := tx.OrderCommitParas{
			UserID: of.localID,
			ProID:  of.pro,
			Nonce:  ns.Nonce,
			SeqNum: nextSeq,
		}

		data, err := ocp.Serialize()
		if err != nil {
			return err
		}

		msg := &tx.Message{
			Version: 0,
			From:    of.localID,
			To:      of.pro,
			Method:  tx.CommitDataOrder,
			Params:  data,
		}

		logger.Debug("push msg: ", msg.From, msg.To, msg.Method, ocp.Nonce, ocp.SeqNum)

		m.msgChan <- msg

		ns.Nonce++
		ns.SeqNum = 0
	}

	return nil
}

func (m *OrderMgr) updateSize(seq types.OrderSeq) {
	logger.Debug("confirm jobs: updateSize in order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)

	key := store.NewKey(pb.MetaType_OrderSeqJobKey, seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)
	val, err := m.ds.Get(key)
	if err != nil {
		return
	}

	sjq := new(types.SegJobsQueue)
	err = sjq.Deserialize(val)
	if err != nil {
		return
	}
	ss := *sjq
	sLen := sjq.Len()
	size := uint64(0)
	for i := 0; i < sLen; i++ {
		m.segConfirmChan <- ss[i]
		size += ss[i].Length * code.DefaultSegSize
	}

	// update size
	m.sizelk.Lock()
	m.opi.ConfirmSize += size

	key = store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
	val, err = m.opi.Serialize()
	if err == nil {
		m.ds.Put(key, val)
	}

	m.lk.RLock()
	of, ok := m.orders[seq.UserID]
	m.lk.RUnlock()
	if ok {
		of.opi.ConfirmSize += size
		key := store.NewKey(pb.MetaType_OrderPayInfoKey, seq.UserID, seq.ProID)
		val, err = of.opi.Serialize()
		if err == nil {
			m.ds.Put(key, val)
		}
	}
	m.sizelk.Unlock()

	key = store.NewKey(pb.MetaType_OrderSeqJobKey, seq.UserID, seq.ProID)
	nData, err := m.ds.Get(key)
	if err == nil {
		nval := new(types.NonceSeq)
		err = nval.Deserialize(nData)
		if err == nil {
			if seq.Nonce == nval.Nonce && seq.SeqNum == nval.SeqNum {
				m.ds.Delete(key)
			}
		}
	}

	logger.Debug("confirm jobs: updateSize done in order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)
}

// remove segment from local when commit
// todo: re-handle at boot
func (m *OrderMgr) ReplaceSegWithLoc(seq types.OrderSeq) {
	logger.Debug("confirm jobs: ReplaceSegWithLoc in order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)

	sid, err := segment.NewSegmentID(m.fsID, 0, 0, 0)
	if err != nil {
		return
	}

	for _, seg := range seq.Segments {
		sid.SetBucketID(seg.BucketID)
		for j := seg.Start; j < seg.Start+seg.Length; j++ {
			sid.SetStripeID(j)
			sid.SetChunkID(seg.ChunkID)

			m.PutSegmentLocation(m.ctx, sid, seq.ProID)
			// delete from local
			m.DeleteSegment(m.ctx, sid)
		}
	}

	logger.Debug("confirm jobs: ReplaceSegWithLoc done in order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)
}

// todo
func (m *OrderMgr) RemoveSegLocation(srp *tx.SegRemoveParas) {
	if srp.UserID != m.localID {
		return
	}

	for _, seg := range srp.Segments {
		for i := seg.Start; i < seg.Start+seg.Length; i++ {
			sid, err := segment.NewSegmentID(m.fsID, seg.BucketID, i, seg.ChunkID)
			if err != nil {
				continue
			}

			m.DeleteSegmentLocation(m.ctx, sid)
		}
	}
}

func (m *OrderMgr) submitOrders() error {
	logger.Debug("addOrder for user: ", m.localID)

	pros, err := m.StateGetProsAt(m.ctx, m.localID)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	// for each provider, do AddOrder
	for _, proID := range pros {
		ns, err := m.StateGetOrderNonce(m.ctx, m.localID, proID, math.MaxUint64)
		if err != nil {
			continue
		}
		si, err := m.is.SettleGetStoreInfo(m.ctx, m.localID, proID)
		if err != nil {
			logger.Debug("addOrder fail to get order info in chain", m.localID, proID, err)
			continue
		}

		logger.Debugf("addOrder user %d pro %d has order %d %d %d", m.localID, proID, si.Nonce, si.SubNonce, ns.Nonce)

		if si.Nonce >= ns.Nonce {
			continue
		}
		logger.Debugf("addOrder user %d pro %d nonce %d", m.localID, proID, si.Nonce)

		// get orderFull from state db
		tof, err := m.StateGetOrder(m.ctx, m.localID, proID, si.Nonce)
		if err != nil {
			logger.Debug("addOrder fail to get order info", m.localID, proID, err)
			continue
		}

		wg.Add(1)
		go func(of *types.SignedOrder) {
			defer wg.Done()

			avail, err := m.is.SettleGetBalanceInfo(m.ctx, of.UserID)
			if err != nil {
				logger.Debug("addOrder fail to add order ", m.localID, of.ProID, err)
			}

			logger.Debugf("addOrder user %d has balance %d", of.UserID, avail)

			// call cm.SettleAddOrder
			err = m.is.SettleAddOrder(m.ctx, of)
			if err != nil {
				logger.Debug("addOrder fail to add order ", m.localID, of.ProID, err)
			} else {
				pay := new(big.Int).SetInt64(of.End - of.Start)
				pay.Mul(pay, of.Price)

				m.sizelk.Lock()

				m.opi.Paid.Add(m.opi.Paid, pay)
				m.opi.OnChainSize += of.Size

				key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
				val, _ := m.opi.Serialize()
				m.ds.Put(key, val)

				m.lk.RLock()
				po, ok := m.orders[of.ProID]
				m.lk.RUnlock()
				if ok {
					po.opi.OnChainSize += of.Size
					po.opi.Paid.Add(po.opi.Paid, pay)
					key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, of.ProID)
					val, _ = po.opi.Serialize()
					m.ds.Put(key, val)
				}
				m.sizelk.Unlock()
			}
		}(&tof.SignedOrder)
	}
	wg.Wait()
	return nil
}
