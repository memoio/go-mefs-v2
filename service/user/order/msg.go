package order

import (
	"context"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/lib/pb"
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

					m.replaceSegWithLoc(seq.OrderSeq)

					m.lk.RLock()
					of, ok := m.pInstMap[seq.ProID]
					m.lk.RUnlock()
					if ok {
						m.updateConfirmSize(of, seq.OrderSeq, true)
					}

				default:
				}
			} else {
				logger.Warn("tx message done fail: ", mid, msg.From, msg.To, msg.Method, st.BlockID, st.Height, st.Status)
			}

			break
		}
	}(mid)
}

func (m *OrderMgr) runBalCheck() {
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
		po, ok := m.pInstMap[proID]
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

		// get proInst from state db
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
			po, ok := m.pInstMap[of.ProID]
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
