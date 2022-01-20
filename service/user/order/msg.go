package order

import (
	"context"
	"encoding/binary"
	"math/big"
	"time"

	"github.com/jbenet/goprocess"

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
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// run once at start
	m.inCheck = true
	go m.checkBalance()

	for {
		select {
		case <-proc.Closing():
			return
		case <-ticker.C:
			if !m.inCheck {
				m.inCheck = true
				go m.checkBalance()
			}
		}
	}
}

func (m *OrderMgr) checkBalance() {
	defer func() {
		m.inCheck = false
	}()

	needPay := big.NewInt(0)
	pros := m.StateGetProsAt(m.ctx, m.localID)
	for _, proID := range pros {
		ns := m.StateGetOrderState(m.ctx, m.localID, proID)
		si, err := m.is.GetStoreInfo(m.ctx, m.localID, proID)
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
			os.Price.Mul(os.Price, big.NewInt(12))
			os.Price.Div(os.Price, big.NewInt(10))

			needPay.Add(needPay, os.Price)
		}
	}

	bal, err := m.is.GetBalanceInfo(m.ctx, m.localID)
	if err != nil {
		return
	}

	if bal.ErcValue.Cmp(needPay) < 0 {
		needPay.Sub(needPay, bal.ErcValue)
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
			st, err := m.GetTxMsgStatus(ctx, mid)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			if st.Status.Err == 0 {
				logger.Debug("tx message done success: ", mid, st.BlockID, st.Height)
			} else {
				logger.Warn("tx message done fail: ", mid, st.BlockID, st.Height, string(st.Status.Extra))
			}

			break
		}
	}(mid)
}

func (m *OrderMgr) loadUnfinished(of *OrderFull) {
	ns := m.StateGetOrderState(m.ctx, of.localID, of.pro)
	logger.Debug("resend message for : ", of.pro, ", has: ", ns.Nonce, ns.SeqNum, ", want: ", of.nonce, of.seqNum)

	seqNum := ns.SeqNum
	for ns.Nonce < of.nonce {
		// add order base
		if seqNum == 0 {
			key := store.NewKey(pb.MetaType_OrderBaseKey, of.localID, of.pro, ns.Nonce)
			data, err := m.ds.Get(key)
			if err != nil {
				return
			}

			msg := &tx.Message{
				Version: 0,
				From:    of.localID,
				To:      of.pro,
				Method:  tx.PreDataOrder,
				Params:  data,
			}

			m.msgChan <- msg
		}

		ss := new(SeqState)
		key := store.NewKey(pb.MetaType_OrderSeqNumKey, of.localID, of.pro, ns.Nonce)
		val, err := m.ds.Get(key)
		if err != nil {
			return
		}
		err = ss.Deserialize(val)
		if err != nil {
			return
		}

		// add seq
		for i := uint32(seqNum); i <= ss.Number; i++ {
			key := store.NewKey(pb.MetaType_OrderSeqKey, of.localID, of.pro, ns.Nonce, i)
			data, err := m.ds.Get(key)
			if err != nil {
				return
			}
			msg := &tx.Message{
				Version: 0,
				From:    of.localID,
				To:      of.pro,
				Method:  tx.AddDataOrder,
				Params:  data,
			}
			m.msgChan <- msg
		}

		// commit
		// todo: latest one
		if ns.Nonce+1 == of.nonce {
			if of.orderState != Order_Done {
				return
			}
		}
		ocp := tx.OrderCommitParas{
			UserID: of.localID,
			ProID:  of.pro,
			Nonce:  ns.Nonce,
			SeqNum: ss.Number + 1,
		}

		data, err := ocp.Serialize()
		if err != nil {
			return
		}

		msg := &tx.Message{
			Version: 0,
			From:    of.localID,
			To:      of.pro,
			Method:  tx.CommitDataOrder,
			Params:  data,
		}

		m.msgChan <- msg

		ns.Nonce++
	}
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
	for _, proID := range pros {
		ns := m.StateGetOrderState(m.ctx, m.localID, proID)
		si, err := m.is.GetStoreInfo(m.ctx, m.localID, proID)
		if err != nil {
			logger.Debug("addOrder fail to get order info in chain", m.localID, proID, err)
			continue
		}

		logger.Debugf("addOrder user %d pro %d has order %d %d %d", m.localID, proID, si.Nonce, si.SubNonce, ns.Nonce)

		if si.Nonce >= ns.Nonce {
			continue
		}

		logger.Debugf("addOrder user %d pro %d nonce %d", m.localID, proID, si.Nonce)

		// add order here
		of, err := m.GetOrder(m.localID, proID, si.Nonce)
		if err != nil {
			logger.Debug("addOrder fail to get order info", m.localID, proID, err)
			continue
		}

		err = m.addOrder(&of.SignedOrder)
		if err != nil {
			logger.Debug("addOrder fail to add order ", m.localID, proID, err)
			continue
		}
	}

	return nil
}

func (m *OrderMgr) addOrder(so *types.SignedOrder) error {
	avail, err := m.is.GetBalanceInfo(m.ctx, so.UserID)
	if err != nil {
		logger.Debug("addOrder fail to get balance ", so.UserID, so.ProID, err)
		return err
	}

	logger.Debugf("addOrder user %d has balance %d", so.UserID, avail)

	ksigns := make([][]byte, 7)
	err = m.is.AddOrder(so, ksigns)
	if err != nil {
		logger.Debug("addOrder fail to add order ", so.UserID, so.ProID, so.Nonce, err)
		return err
	}

	return nil
}
