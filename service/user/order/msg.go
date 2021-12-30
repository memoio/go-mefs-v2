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
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-proc.Closing():
			return
		case msg := <-m.msgChan:
			m.pushMessage(msg)
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
	pros := m.GetProsForUser(m.ctx, m.localID)
	for _, proID := range pros {
		ns := m.GetOrderState(m.ctx, m.localID, proID)
		nonce, subNonce, err := m.is.GetOrderInfo(m.localID, proID)
		if err != nil {
			logger.Debug("fail to get order info in chain", m.localID, proID, err)
			continue
		}

		logger.Debugf("user %d pro %d has order %d %d %d", m.localID, proID, nonce, subNonce, ns.Nonce)
		for i := nonce; i <= ns.Nonce; i++ {
			logger.Debugf("user %d pro %d add order %d %d", m.localID, proID, i, ns.Nonce)

			// load order
			ob := new(types.SignedOrder)
			key := store.NewKey(pb.MetaType_OrderBaseKey, m.localID, proID, i)
			val, err := m.ds.Get(key)
			if err == nil {
				ob.Deserialize(val)
			}

			os := new(types.SignedOrderSeq)
			key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, proID, i, ns.SeqNum)
			val, err = m.ds.Get(key)
			if err == nil {
				os.Deserialize(val)
			}

			os.Price.Mul(os.Price, big.NewInt(ob.End-ob.Start))
			os.Price.Mul(os.Price, big.NewInt(12))
			os.Price.Div(os.Price, big.NewInt(10))

			needPay.Add(needPay, os.Price)
		}
	}

	bal, err := m.is.GetBalance(m.ctx, m.localID)
	if err != nil {
		return
	}

	if bal.Cmp(needPay) < 0 {
		needPay.Sub(needPay, bal)
		m.is.Recharge(needPay)
	}
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
	ns := m.GetOrderState(m.ctx, of.localID, of.pro)
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
				Method:  tx.DataPreOrder,
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
				Method:  tx.DataOrder,
				Params:  data,
			}
			m.msgChan <- msg
		}

		// commit
		ocp := tx.OrderCommitParas{
			UserID: of.localID,
			ProID:  of.pro,
			Nonce:  ns.Nonce,
			SeqNum: ss.Number,
		}

		data, err := ocp.Serialize()
		if err != nil {
			return
		}

		msg := &tx.Message{
			Version: 0,
			From:    of.localID,
			To:      of.pro,
			Method:  tx.DataOrderCommit,
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
