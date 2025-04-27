package order

import (
	"context"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/types"

	"golang.org/x/xerrors"
)

func (m *OrderMgr) OrderList(_ context.Context) ([]uint64, error) {
	res := make([]uint64, 0, len(m.pros))
	res = append(res, m.pros...)
	return res, nil
}

func (m *OrderMgr) OrderGetJobInfo(ctx context.Context) ([]*api.OrderJobInfo, error) {
	res := make([]*api.OrderJobInfo, 0, len(m.pros))

	for _, pid := range m.pros {
		oi, err := m.OrderGetJobInfoAt(ctx, pid)
		if err != nil {
			continue
		}

		res = append(res, oi)
	}

	return res, nil
}

func (m *OrderMgr) OrderGetJobInfoAt(_ context.Context, proID uint64) (*api.OrderJobInfo, error) {
	logger.Debug("get jobinfo at: ", proID)
	m.lk.RLock()
	of, ok := m.pInstMap[proID]
	m.lk.RUnlock()
	if ok {
		logger.Debug("get jobinfo at: ", proID)
		oi := &api.OrderJobInfo{
			ID: proID,

			AvailTime:  of.availTime,
			Nonce:      of.nonce,
			OrderTime:  of.orderTime,
			OrderState: string(of.orderState),

			SeqNum:   of.seqNum,
			SeqTime:  of.seqTime,
			SeqState: string(of.seqState),

			Jobs: of.jobCount(),

			Ready:  of.ready,
			InStop: of.inStop,
		}

		if of.base != nil {
			oi.Nonce = of.base.Nonce
		}

		if of.seq != nil {
			oi.SeqNum = of.seq.SeqNum
		}

		logger.Debug("get jobinfo at: ", proID)
		pid, err := m.ns.GetPeerIDAt(m.ctx, proID)
		if err == nil {
			oi.PeerID = string(pid)
		}

		return oi, nil
	}

	return nil, xerrors.Errorf("not found")
}

func (m *OrderMgr) OrderGetPayInfoAt(ctx context.Context, pid uint64) (*types.OrderPayInfo, error) {
	pi := new(types.OrderPayInfo)
	if pid == 0 {
		m.sizelk.RLock()
		pi.Size = m.opi.Size // may be less than
		pi.ConfirmSize = m.opi.ConfirmSize
		pi.OnChainSize = m.opi.OnChainSize
		pi.NeedPay = new(big.Int).Set(m.opi.NeedPay)
		pi.Paid = new(big.Int).Set(m.opi.Paid)
		m.sizelk.RUnlock()

		bi, err := m.is.SettleGetBalanceInfo(ctx, m.localID)
		if err != nil {
			return nil, err
		}
		pi.Balance = new(big.Int).Set(bi.ErcValue)
		pi.Balance.Add(pi.Balance, bi.FsValue)
	} else {
		m.lk.RLock()
		of, ok := m.pInstMap[pid]
		m.lk.RUnlock()
		if ok {
			pi.ID = pid
			m.sizelk.RLock()
			pi.Size = of.opi.Size
			pi.ConfirmSize = of.opi.ConfirmSize
			pi.OnChainSize = of.opi.OnChainSize
			pi.NeedPay = new(big.Int).Set(of.opi.NeedPay)
			pi.Paid = new(big.Int).Set(of.opi.Paid)
			m.sizelk.RUnlock()

			if of.seq != nil {
				pi.Size += of.seq.Size
			} else {
				if of.base != nil {
					pi.Size += of.base.Size
				}
			}
		}
	}

	return pi, nil
}

func (m *OrderMgr) OrderGetPayInfo(ctx context.Context) ([]*types.OrderPayInfo, error) {
	res := make([]*types.OrderPayInfo, 0, len(m.pros))

	for _, pid := range m.pros {
		oi, err := m.OrderGetPayInfoAt(ctx, pid)
		if err != nil {
			continue
		}

		res = append(res, oi)
	}

	return res, nil
}

func (m *OrderMgr) OrderGetProsAt(ctx context.Context, bid uint64) (*api.ProsInBucket, error) {
	m.lk.RLock()
	lp, ok := m.bucMap[bid]
	m.lk.RUnlock()
	if ok {
		lp.lk.RLock()
		defer lp.lk.RUnlock()
		ppb := &api.ProsInBucket{
			InUse:       make([]uint64, 0, len(lp.pros)),
			Deleted:     make([]uint64, 0, len(lp.deleted)),
			DelPerChunk: make([][]uint64, len(lp.delPerChunk)),
		}
		ppb.InUse = append(ppb.InUse, lp.pros...)
		ppb.Deleted = append(ppb.Deleted, lp.deleted...)
		for i := 0; i < len(lp.delPerChunk); i++ {
			ppb.DelPerChunk[i] = make([]uint64, 0, len(lp.delPerChunk[i]))
			ppb.DelPerChunk[i] = append(ppb.DelPerChunk[i], lp.delPerChunk[i]...)
		}
		return ppb, nil
	}

	return nil, xerrors.Errorf("no such bucket")
}
