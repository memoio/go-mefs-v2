package order

import (
	"math"
	"math/big"
	"os"
	"time"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

func (m *OrderMgr) fix(of *proInst) {
	err := m.handleUnConfirm(of)
	logger.Debug("handle re-confirm order sat: ", of.pro, err)
	err = m.handleUnPush(of)
	logger.Debug("handle re-push order sat: ", of.pro, err)
	m.handleMissing(of)
	logger.Debug("handle missing order sat: ", of.pro, err)
}

func (m *OrderMgr) handleMissing(of *proInst) error {
	ns, err := m.StateGetOrderNonce(m.ctx, of.localID, of.pro, math.MaxUint64)
	if err != nil {
		return err
	}

	logger.Debug("fix order for: ", of.pro, ", want than: ", ns.Nonce, ns.SeqNum, ", has: ", of.nonce, of.seqNum, of.orderState, of.seqState)

	if ns.Nonce+1 < of.nonce || (ns.Nonce+1 == of.nonce && ns.SeqNum <= of.seqNum) || (ns.Nonce == of.nonce && ns.SeqNum == of.seqNum && ns.SeqNum == 0) {
		return nil
	}

	if os.Getenv("MEFS_RECOVERY_MODE") == "" {
		logger.Warnf("stop pro %d, nonce %d is smaller than data chain %d", of.pro, of.nonce, ns.Nonce)
		of.failCnt = minFailCnt + 1
		return nil
	}

	// if provider advance more than one step
	// not handle it due to miss too many
	// caused by fail to save local and fail to commit

	// reget from datachain if local has missing
	// 1. get base from datachain if seqNum == 0
	// 2. commit current order if size > 0

	sor, err := m.StateGetOrder(m.ctx, of.localID, of.pro, ns.Nonce)
	if err != nil {
		return err
	}

	of.base = &sor.SignedOrder
	of.prevEnd = sor.End

	of.nonce = sor.Nonce + 1
	of.orderState = Order_Running
	of.orderTime = time.Now().Unix()

	of.seqNum = ns.SeqNum
	of.seqState = OrderSeq_Init
	of.seqTime = time.Now().Unix()

	if ns.SeqNum > 0 {
		// can close
		if sor.Size > 0 {
			of.orderState = Order_Closing
			of.seqState = OrderSeq_Finish
		}

		sos, err := m.StateGetOrderSeq(m.ctx, of.localID, of.pro, ns.Nonce, ns.SeqNum-1)
		if err != nil {
			return err
		}

		of.seq = &types.SignedOrderSeq{
			OrderSeq:   sos.OrderSeq,
			ProDataSig: sor.Psign, // in case seqnum check
		}

		of.sjq = new(types.SegJobsQueue)

		// save seq state
		saveSeqState(of, m.ds)

		of.seqState = OrderSeq_Init
	}

	saveOrderBase(of, m.ds)
	saveOrderState(of, m.ds)

	return nil
}

// re-confirm seq if exit soon after push msg;
func (m *OrderMgr) handleUnConfirm(of *proInst) error {
	ns, err := m.StateGetOrderNonce(m.ctx, of.localID, of.pro, math.MaxUint64)
	if err != nil {
		return err
	}

	if os.Getenv("MEFS_RECOVERY_MODE") != "" {
		of.opi.NeedPay.SetInt64(0)
		of.opi.Size = 0
		of.opi.ConfirmSize = 0

		logger.Debug("handle data-chain: ", of.localID, of.pro, ns.Nonce, ns.SeqNum)
		for i := uint64(0); i <= ns.Nonce; i++ {
			sor, err := m.StateGetOrder(m.ctx, of.localID, of.pro, i)
			if err != nil {
				continue
			}

			// not add last one
			if i != ns.Nonce {
				m.updateBaseSize(of, &sor.SignedOrder, false)
			}

			// handle confirmed seq
			for j := uint32(0); j < sor.SeqNum; j++ {
				sos, err := m.StateGetOrderSeq(m.ctx, of.localID, of.pro, i, j)
				if err != nil {
					continue
				}

				m.replaceSegWithLoc(sos.OrderSeq)
				m.updateConfirmSize(of, sos.OrderSeq, false)
			}
		}

		key := store.NewKey(pb.MetaType_OrderPayInfoKey, of.localID, of.pro)
		val, err := of.opi.Serialize()
		if err == nil {
			m.ds.Put(key, val)
		}

		m.sizelk.Lock()
		if m.opi.OnChainSize == m.opi.Size && m.opi.Size > 0 {
			m.opi.Paid.Set(m.opi.NeedPay)
		}

		key = store.NewKey(pb.MetaType_OrderPayInfoKey, m.localID)
		val, err = m.opi.Serialize()
		if err == nil {
			m.ds.Put(key, val)
		}
		m.sizelk.Unlock()
	} else {
		// handle last order seq
		logger.Debug("re-confirm jobs in last order seq: ", of.localID, of.pro, ns.Nonce, ns.SeqNum)
		key := store.NewKey(pb.MetaType_OrderSeqJobKey, of.localID, of.pro)
		nData, err := m.ds.Get(key)
		if err == nil {
			nval := new(types.NonceSeq)
			err = nval.Deserialize(nData)
			if err == nil {
				if ns.Nonce > nval.Nonce || (ns.Nonce == nval.Nonce && ns.SeqNum > nval.SeqNum) {
					key := store.NewKey(pb.MetaType_OrderSeqKey, of.localID, of.pro, nval.Nonce, nval.SeqNum)
					data, err := m.ds.Get(key)
					if err == nil {
						seq := new(types.SignedOrderSeq)
						err = seq.Deserialize(data)
						if err == nil {
							logger.Debug("re-confirm jobs in last order seq: ", seq.UserID, seq.ProID, seq.Nonce, seq.SeqNum)

							m.replaceSegWithLoc(seq.OrderSeq)
							m.updateConfirmSize(of, seq.OrderSeq, true)
						}
					}
				}
			}
		}
	}

	return nil
}

// push stale order/seq if push fails before
func (m *OrderMgr) handleUnPush(of *proInst) error {
	ns, err := m.StateGetOrderNonce(m.ctx, of.localID, of.pro, math.MaxUint64)
	if err != nil {
		return err
	}

	logger.Debug("resend message for: ", of.pro, ", has: ", ns.Nonce, ns.SeqNum, ", want: ", of.nonce, of.seqNum, of.orderState, of.seqState)

	// local has stale order/seq; need push
	for ns.Nonce < of.nonce {
		// push order base
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

// check local seq is valid: has two valid signs; contain no duplicate chunks
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
