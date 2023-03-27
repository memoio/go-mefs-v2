package order

import (
	"context"
	"math"
	"math/big"
	"os"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
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

	bal.FsValue.Add(bal.FsValue, bal.LockValue)

	if bal.FsValue.Cmp(needPay) < 0 {
		needPay.Sub(needPay, bal.FsValue)
		if bal.ErcValue.Cmp(needPay) < 0 {
			needPay.Set(bal.ErcValue)
		}
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

	if os.Getenv("MEFS_RECOVERY_MODE") != "" {
		logger.Debug("clean stale data: ", of.localID, of.pro, ns.Nonce, ns.SeqNum)
		sid, err := segment.NewSegmentID(m.fsID, 0, 0, 0)
		if err != nil {
			return err
		}
		for i := uint64(0); i <= ns.Nonce; i++ {
			ss := new(SeqState)
			key := store.NewKey(pb.MetaType_OrderSeqNumKey, of.localID, of.pro, i)
			val, err := m.ds.Get(key)
			if err != nil {
				continue
			}
			err = ss.Deserialize(val)
			if err != nil {
				continue
			}
			for j := uint32(0); j <= ss.Number; j++ {
				sos, err := m.StateGetOrderSeq(m.ctx, of.localID, of.pro, i, j)
				if err != nil {
					continue
				}

				for _, seg := range sos.Segments {
					sid.SetBucketID(seg.BucketID)
					sid.SetChunkID(seg.ChunkID)
					for k := seg.Start; k < seg.Start+seg.Length; k++ {
						sid.SetStripeID(k)

						has, err := m.HasSegment(m.ctx, sid)
						if err == nil && has {
							m.PutSegmentLocation(m.ctx, sid, of.pro)
							m.DeleteSegment(m.ctx, sid)
						}
					}
				}
			}
		}
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

			err = m.checkSeg(of.localID, of.pro, ns.Nonce, i)
			if err != nil {
				return xerrors.Errorf("check seq err: %s", err)
			}

			key = store.NewKey(pb.MetaType_OrderSeqJobKey, of.localID, of.pro)
			nval := &types.NonceSeq{
				Nonce:  ns.Nonce,
				SeqNum: i,
			}

			nData, err := nval.Serialize()
			if err == nil {
				m.ds.Put(key, nData)
			}

			nextSeq = i + 1
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
				logger.Debugf("order state %s is not done at %d", nns.State, ns.Nonce)
				return nil
			}
		}

		if nextSeq == 0 {
			logger.Warn("empty data order at: ", of.pro, ns.Nonce)
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
	of, ok := m.orders[seq.ProID]
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

	logger.Debug("confirm jobs: updateSize done in order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum, size)
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
		sid.SetChunkID(seg.ChunkID)
		for j := seg.Start; j < seg.Start+seg.Length; j++ {
			sid.SetStripeID(j)

			m.PutSegmentLocation(m.ctx, sid, seq.ProID)
			// delete from local
			m.DeleteSegment(m.ctx, sid)
		}
	}

	logger.Debug("confirm jobs: ReplaceSegWithLoc done in order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)
}

func (m *OrderMgr) checkSeg(userID, proID, nonce uint64, seqNum uint32) error {
	logger.Debug("check order seq: ", userID, proID, nonce, seqNum)
	key := store.NewKey(pb.MetaType_OrderSeqKey, userID, proID, nonce, seqNum)
	data, err := m.ds.Get(key)
	if err != nil {
		return xerrors.Errorf("found %s error %d", string(key), err)
	}

	sos := new(types.SignedOrderSeq)
	err = sos.Deserialize(data)
	if err != nil {
		return err
	}

	// verify last one
	if sos.ProDataSig.Type == 0 || sos.ProSig.Type == 0 {
		return xerrors.Errorf("pro sig empty %d %d", sos.ProDataSig.Type, sos.ProSig.Type)
	}

	sid, err := segment.NewSegmentID(m.fsID, 0, 0, 0)
	if err != nil {
		return err
	}

	// contain wrong seg
	wsos := &types.SignedOrderSeq{
		OrderSeq: types.OrderSeq{
			UserID: sos.UserID,
			ProID:  sos.ProID,
			Nonce:  sos.Nonce,
			SeqNum: sos.SeqNum,
		},
		ProDataSig: sos.ProDataSig, // has old sig
		ProSig:     sos.ProSig,
	}

	nsos := &types.SignedOrderSeq{
		OrderSeq: types.OrderSeq{
			UserID: sos.UserID,
			ProID:  sos.ProID,
			Nonce:  sos.Nonce,
			SeqNum: sos.SeqNum,
			Price:  new(big.Int),
		},
	}

	ospr := new(types.SignedOrderSeq)
	segCheck := new(types.AggSegsQueue)

	if seqNum == 0 && nonce > 0 {
		key := store.NewKey(pb.MetaType_OrderSeqNumKey, userID, proID, nonce-1)
		val, err := m.ds.Get(key)
		if err != nil {
			return xerrors.Errorf("found %s error %d", string(key), err)
		}
		ss := new(SeqState)
		err = ss.Deserialize(val)
		if err != nil {
			return err
		}

		i := uint32(ss.Number)
		if os.Getenv("MEFS_RECOVERY_MODE") != "" {
			i = 0
		}

		// change from 0 if not ok
		for ; i <= ss.Number; i++ {
			key = store.NewKey(pb.MetaType_OrderSeqKey, userID, proID, nonce-1, i)
			obdata, err := m.ds.Get(key)
			if err != nil {
				return err
			}
			err = ospr.Deserialize(obdata)
			if err != nil {
				return err
			}
			for _, seg := range ospr.Segments {
				sid.SetBucketID(seg.BucketID)
				sid.SetChunkID(seg.ChunkID)
				for j := seg.Start; j < seg.Start+seg.Length; j++ {
					sid.SetStripeID(j)

					as := &types.AggSegs{
						BucketID: sid.GetBucketID(),
						Start:    sid.GetStripeID(),
						Length:   1,
						ChunkID:  sid.GetChunkID(),
					}
					segCheck.Push(as)
				}
				segCheck.Merge()
			}
		}
	}

	if seqNum > 0 {
		i := uint32(seqNum - 1)
		if os.Getenv("MEFS_RECOVERY_MODE") != "" {
			i = 0
		}
		for ; i < seqNum; i++ {
			key = store.NewKey(pb.MetaType_OrderSeqKey, userID, proID, nonce, i)
			obdata, err := m.ds.Get(key)
			if err != nil {
				return err
			}
			ospr := new(types.SignedOrderSeq)
			err = ospr.Deserialize(obdata)
			if err != nil {
				return err
			}
			for _, seg := range ospr.Segments {
				sid.SetBucketID(seg.BucketID)
				sid.SetChunkID(seg.ChunkID)
				for j := seg.Start; j < seg.Start+seg.Length; j++ {
					sid.SetStripeID(j)

					as := &types.AggSegs{
						BucketID: sid.GetBucketID(),
						Start:    sid.GetStripeID(),
						Length:   1,
						ChunkID:  sid.GetChunkID(),
					}
					segCheck.Push(as)
				}
				segCheck.Merge()
			}
			if i == seqNum-1 {
				nsos.Price.Set(ospr.Price)
				nsos.Size = ospr.Size
			}
		}
	}

	renew := false

	cnt := 0
	for _, seg := range sos.Segments {
		sid.SetBucketID(seg.BucketID)
		sid.SetChunkID(seg.ChunkID)
		for j := seg.Start; j < seg.Start+seg.Length; j++ {
			sid.SetStripeID(j)

			as := &types.AggSegs{
				BucketID: sid.GetBucketID(),
				Start:    sid.GetStripeID(),
				Length:   1,
				ChunkID:  sid.GetChunkID(),
			}

			if segCheck.Has(sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID()) {
				renew = true
				logger.Debug("duplicate chunk: ", sid.String())
				wsos.Segments.Push(as)
				wsos.Segments.Merge()
				continue
			}

			_, err := m.GetSegmentLocation(m.ctx, sid)
			if err != nil {
				// self already has
				if nsos.Segments.Has(sid.GetBucketID(), sid.GetStripeID(), sid.GetChunkID()) {
					logger.Debug("self duplicate chunk: ", sid.String())
				} else {
					cnt++
					nsos.Segments.Push(as)
					nsos.Segments.Merge()
				}
			} else {
				renew = true
				logger.Debug("duplicate chunk: ", sid.String())
				wsos.Segments.Push(as)
				wsos.Segments.Merge()
			}
		}
	}

	key = store.NewKey(pb.MetaType_OrderBaseKey, sos.UserID, sos.ProID, sos.Nonce)
	obdata, err := m.ds.Get(key)
	if err != nil {
		return err
	}

	ob := new(types.SignedOrder)
	err = ob.Deserialize(obdata)
	if err != nil {
		return err
	}

	usize := uint64(cnt) * build.DefaultSegSize
	if ospr.Size+usize != sos.Size {
		renew = true
	}

	if renew {
		// update price and size
		nsos.Size += usize
		price := new(big.Int).Mul(ob.SegPrice, big.NewInt(int64(cnt)))
		nsos.Price.Add(nsos.Price, price)

		// re-sign
		ssig, err := m.RoleSign(m.ctx, m.localID, nsos.Hash().Bytes(), types.SigSecp256k1)
		if err != nil {
			return err
		}

		ob.Size = nsos.Size
		ob.Price.Set(nsos.Price)
		osig, err := m.RoleSign(m.ctx, m.localID, ob.Hash(), types.SigSecp256k1)
		if err != nil {
			return err
		}

		wsos.UserDataSig = ssig // contain new sig
		wsos.UserSig = osig

		// send wsos out
		rsos, err := m.getSeqFixAck(wsos)
		if err != nil {
			return xerrors.Errorf("getSeqFixAck: %s", err)
		}

		ok, err := m.RoleVerify(m.ctx, wsos.ProID, nsos.Hash().Bytes(), rsos.ProDataSig)
		if err != nil || !ok {
			return xerrors.Errorf("fix seq fail invalid pro data sign: %s", err)
		}

		ok, err = m.RoleVerify(m.ctx, wsos.ProID, ob.Hash(), rsos.ProSig)
		if err != nil || !ok {
			return xerrors.Errorf("fix seq fail invalid pro sign: %s", err)
		}

		// save order base
		key := store.NewKey(pb.MetaType_OrderBaseKey, sos.UserID, sos.ProID, sos.Nonce)
		obdata, err := ob.Serialize()
		if err != nil {
			return err
		}
		m.ds.Put(key, obdata)

		// save order seq
		nsos.UserDataSig = ssig
		nsos.UserSig = osig

		nsos.ProDataSig = rsos.ProDataSig
		nsos.ProSig = rsos.ProSig

		data, err = nsos.Serialize()
		if err != nil {
			return err
		}
		key = store.NewKey(pb.MetaType_OrderSeqKey, sos.UserID, sos.ProID, sos.Nonce, sos.SeqNum)
		m.ds.Put(key, data)
	}

	msg := &tx.Message{
		Version: 0,
		From:    nsos.UserID,
		To:      nsos.ProID,
		Method:  tx.AddDataOrder,
		Params:  data,
	}

	m.msgChan <- msg

	logger.Debug("push msg: ", msg.From, msg.To, msg.Method, sos.Nonce, sos.SeqNum)

	return nil
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
	for i, proID := range pros {
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

		// adjust size and paid
		m.sizelk.Lock()
		m.lk.RLock()
		po, ok := m.orders[proID]
		m.lk.RUnlock()
		if ok && po.opi.OnChainSize < si.Size {
			subSize := si.Size - po.opi.OnChainSize
			subP := new(big.Int).Sub(po.opi.NeedPay, po.opi.Paid)

			po.opi.OnChainSize = si.Size
			po.opi.Paid.Set(po.opi.NeedPay)

			m.opi.OnChainSize += subSize
			m.opi.Paid.Add(m.opi.Paid, subP)
		}
		m.sizelk.Unlock()

		logger.Debugf("addOrder user %d pro %d nonce %d", m.localID, proID, si.Nonce)

		// get orderFull from state db
		tof, err := m.StateGetOrder(m.ctx, m.localID, proID, si.Nonce)
		if err != nil {
			logger.Debug("addOrder fail to get order info", m.localID, proID, err)
			continue
		}

		wg.Add(1)
		go func(of *types.SignedOrder, pi int) {
			defer wg.Done()

			time.Sleep(time.Duration(pi) * time.Second)

			avail, err := m.is.SettleGetBalanceInfo(m.ctx, of.UserID)
			if err != nil {
				logger.Debug("addOrder fail to add order ", m.localID, of.ProID, err)
			}

			logger.Debugf("addOrder user %d has balance %d", of.UserID, avail)

			// call cm.SettleAddOrder
			err = m.is.SettleAddOrder(m.ctx, of)
			if err != nil {
				logger.Debug("addOrder fail to add order ", m.localID, of.ProID, err)
				return
			}

			pay := new(big.Int).SetInt64(of.End - of.Start)
			pay.Mul(pay, of.Price)

			m.sizelk.Lock()

			m.opi.Paid.Add(m.opi.Paid, pay)
			if m.opi.Paid.Cmp(m.opi.NeedPay) > 0 {
				m.opi.Paid.Set(m.opi.NeedPay)
			}
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
				if po.opi.Paid.Cmp(po.opi.NeedPay) > 0 {
					po.opi.Paid.Set(po.opi.NeedPay)
				}
				key := store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID, of.ProID)
				val, _ = po.opi.Serialize()
				m.ds.Put(key, val)
			}
			m.sizelk.Unlock()
		}(&tof.SignedOrder, i)
	}
	wg.Wait()
	return nil
}
