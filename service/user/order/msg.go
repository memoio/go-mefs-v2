package order

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/fxamacker/cbor/v2"
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

			logger.Debug("tx message done: ", mid, st.BlockID, st.Height, st.Status.Err, string(st.Status.Extra))
			break
		}
	}(mid)
}

func (m *OrderMgr) loadUnfinished(of *OrderFull) {
	ns := m.GetOrderState(m.ctx, of.localID, of.pro)
	if of.nonce == 0 {
		return
	}

	logger.Debug("resend message for : ", of.pro, ", has: ", ns.Nonce, ns.SeqNum, ", want: ", of.nonce, of.seqNum)

	if ns.Nonce == 0 {
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
		ns.Nonce++
	}

	for ns.Nonce < of.nonce {
		ss := new(SeqState)
		key := store.NewKey(pb.MetaType_OrderSeqNumKey, of.localID, of.pro, ns.Nonce-1)
		val, err := m.ds.Get(key)
		if err != nil {
			return
		}
		err = cbor.Unmarshal(val, ss)
		if err != nil {
			return
		}

		for i := uint32(0); i <= ss.Number; i++ {
			key := store.NewKey(pb.MetaType_OrderSeqKey, of.localID, of.pro, ns.Nonce-1, i)
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

		key = store.NewKey(pb.MetaType_OrderBaseKey, of.localID, of.pro, ns.Nonce)
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
		ns.Nonce++
	}

	ss := new(SeqState)
	key := store.NewKey(pb.MetaType_OrderSeqNumKey, of.localID, of.pro, ns.Nonce-1)
	val, err := m.ds.Get(key)
	if err != nil {
		return
	}
	err = cbor.Unmarshal(val, ss)
	if err != nil {
		return
	}

	if ss.State == OrderSeq_Finish {
		ss.Number++
	}

	for i := ns.SeqNum; i < ss.Number; i++ {
		key := store.NewKey(pb.MetaType_OrderSeqKey, of.localID, of.pro, ns.Nonce-1, i)
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

	if of.orderState >= Order_Running {
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
