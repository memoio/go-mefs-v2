package order

import (
	"context"
	"encoding/binary"
	"math/big"
	"time"

	"github.com/jbenet/goprocess"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib"
	"github.com/memoio/go-mefs-v2/lib/code"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (m *OrderMgr) runPush(proc goprocess.Process) {
	for {
		select {
		case <-proc.Closing():
			return
		case msg := <-m.msgChan:
			m.pushMessage(msg)
		}
	}
}

func (m *OrderMgr) runCheck(proc goprocess.Process) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// run once at start
	go m.checkBalance()

	for {
		select {
		case <-proc.Closing():
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
	pros := m.StateGetProsAt(m.ctx, m.localID)
	for _, proID := range pros {
		ns := m.StateGetOrderState(m.ctx, m.localID, proID)
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
		m.is.Recharge(needPay)
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
		ctx, cancle := context.WithTimeout(m.ctx, 10*time.Minute)
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
				if msg.Method == tx.AddDataOrder {
					// confirm

					logger.Debug("confirm jobs in order seq: ", msg.From, msg.To)

					seq := new(types.SignedOrderSeq)
					err := seq.Deserialize(msg.Params)
					if err != nil {
						return
					}

					logger.Debug("confirm jobs in order seq: ", msg.From, msg.To, seq.Nonce, seq.SeqNum)

					key := store.NewKey(pb.MetaType_OrderSeqJobKey, msg.From, msg.To, seq.Nonce, seq.SeqNum)
					val, err := m.ds.Get(key)
					if err != nil {
						return
					}

					sjq := new(types.SegJobsQueue)
					err = sjq.Deserialize(val)
					if err == nil {
						ss := *sjq
						sLen := sjq.Len()
						size := uint64(0)
						for i := 0; i < sLen; i++ {
							m.segConfirmChan <- ss[i]
							size += ss[i].Length * code.DefaultSegSize
						}

						logger.Debug("confirm jobs in order seq: ", msg.From, msg.To, seq.Nonce, seq.SeqNum, size)

						// update size
						m.sizelk.Lock()
						m.opi.ConfirmSize += size

						key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
						val, _ = m.opi.Serialize()
						m.ds.Put(key, val)

						m.lk.RLock()
						of, ok := m.orders[msg.To]
						m.lk.RUnlock()
						if ok {
							of.opi.ConfirmSize += size
							key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, msg.To)
							val, _ = of.opi.Serialize()
							m.ds.Put(key, val)
						}
						m.sizelk.Unlock()
					}
				}
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}

			break
		}
	}(mid)
}

func (m *OrderMgr) loadUnfinished(of *OrderFull) error {
	ns := m.StateGetOrderState(m.ctx, of.localID, of.pro)
	logger.Debug("resend message for: ", of.pro, ", has: ", ns.Nonce, ns.SeqNum, ", want: ", of.nonce, of.seqNum)

	for ns.Nonce < of.nonce {
		// add order base
		if ns.SeqNum == 0 {
			_, err := m.StateMgr.StateGetOrder(m.ctx, of.localID, of.pro, ns.Nonce)
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

		m.msgChan <- msg

		logger.Debug("push msg: ", msg.From, msg.To, msg.Method, ns.Nonce, nextSeq)

		ns.Nonce++
		ns.SeqNum = 0
	}

	return nil
}

func (m *OrderMgr) AddOrderSeq(seq types.OrderSeq) {
	// filter other
	if seq.UserID != m.localID {
		return
	}
	sid, err := segment.NewSegmentID(m.fsID, 0, 0, 0)
	if err != nil {
		return
	}
	val := make([]byte, 8)
	binary.BigEndian.PutUint64(val, seq.ProID)
	for _, seg := range seq.Segments {
		sid.SetBucketID(seg.BucketID)
		for j := seg.Start; j < seg.Start+seg.Length; j++ {
			sid.SetStripeID(j)
			sid.SetChunkID(seg.ChunkID)
			key := store.NewKey(pb.MetaType_SegLocationKey, sid.ToString())
			// add location map
			m.ds.Put(key, val)
			// delete from local
			m.DeleteSegment(m.ctx, sid)
		}
	}
}

func (m *OrderMgr) RemoveSeg(srp *tx.SegRemoveParas) {
	if srp.UserID != m.localID {
		return
	}

	for _, seg := range srp.Segments {
		for i := seg.Start; i < seg.Start+seg.Length; i++ {
			sid, err := segment.NewSegmentID(m.fsID, seg.BucketID, i, seg.ChunkID)
			if err != nil {
				continue
			}

			key := store.NewKey(pb.MetaType_SegLocationKey, sid.ToString())
			m.ds.Delete(key)
		}
	}
}

func (m *OrderMgr) submitOrders() error {
	logger.Debug("addOrder for user: ", m.localID)

	pros := m.StateGetProsAt(m.ctx, m.localID)
	pLen := len(pros)
	fin := 0
	pMap := make(map[uint64]struct{}, pLen)
	for fin < pLen {
		for _, proID := range pros {
			_, ok := pMap[proID]
			if ok {
				continue
			}
			ns := m.StateGetOrderState(m.ctx, m.localID, proID)
			si, err := m.is.SettleGetStoreInfo(m.ctx, m.localID, proID)
			if err != nil {
				pMap[proID] = struct{}{}
				fin++
				logger.Debug("addOrder fail to get order info in chain", m.localID, proID, err)
				continue
			}

			logger.Debugf("addOrder user %d pro %d has order %d %d %d", m.localID, proID, si.Nonce, si.SubNonce, ns.Nonce)

			if si.Nonce >= ns.Nonce {
				pMap[proID] = struct{}{}
				fin++
				continue
			}
			logger.Debugf("addOrder user %d pro %d nonce %d", m.localID, proID, si.Nonce)

			// add order here
			of, err := m.StateGetOrder(m.ctx, m.localID, proID, si.Nonce)
			if err != nil {
				pMap[proID] = struct{}{}
				fin++
				logger.Debug("addOrder fail to get order info", m.localID, proID, err)
				continue
			}

			err = m.validOrder(of.SignedOrder)
			if err != nil {
				pMap[proID] = struct{}{}
				fin++
				logger.Debug("addOrder params wrong", m.localID, proID, of.DelPart.Size, err)
				continue
			}

			avail, err := m.is.SettleGetBalanceInfo(m.ctx, of.UserID)
			if err != nil {
				pMap[proID] = struct{}{}
				fin++
				logger.Debug("addOrder fail to add order ", m.localID, proID, err)
				continue
			}

			logger.Debugf("addOrder user %d has balance %d", of.UserID, avail)

			err = m.is.AddOrder(&of.SignedOrder)
			if err != nil {
				pMap[proID] = struct{}{}
				fin++
				logger.Debug("addOrder fail to add order ", m.localID, proID, err)
				continue
			}

			if err == nil {
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

		}
		time.Sleep(10 * time.Second)
	}

	return nil
}

func (m *OrderMgr) validOrder(so types.SignedOrder) error {
	err := lib.CheckOrder(so.OrderBase)
	if err != nil {
		return err
	}

	if so.Size == 0 {
		return xerrors.Errorf("size is zero")
	}

	ok, err := m.RoleVerify(m.ctx, so.ProID, so.Hash(), so.Psign)
	if err != nil {
		return xerrors.Errorf("%d order pro sign is wrong %s", so.ProID, err)
	}
	if !ok {
		return xerrors.Errorf("%d order pro sign is wrong", so.ProID)
	}

	ok, err = m.RoleVerify(m.ctx, so.UserID, so.Hash(), so.Usign)
	if err != nil {
		return xerrors.Errorf("%d order user sign is wrong %s", so.UserID, err)
	}
	if !ok {
		return xerrors.Errorf("%d order user sign is wrong", so.UserID)
	}

	return nil
}
