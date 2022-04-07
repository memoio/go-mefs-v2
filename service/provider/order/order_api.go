package order

import (
	"context"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"

	"golang.org/x/xerrors"
)

func (m *OrderMgr) OrderList(_ context.Context) ([]uint64, error) {
	res := make([]uint64, 0, len(m.users))
	res = append(res, m.users...)
	return res, nil
}

func (m *OrderMgr) OrderGetJobInfo(ctx context.Context) ([]*api.OrderJobInfo, error) {
	res := make([]*api.OrderJobInfo, 0, len(m.users))

	for _, pid := range m.users {
		oi, err := m.OrderGetJobInfoAt(ctx, pid)
		if err != nil {
			continue
		}

		res = append(res, oi)
	}

	return res, nil
}

func (m *OrderMgr) OrderGetJobInfoAt(_ context.Context, userID uint64) (*api.OrderJobInfo, error) {
	m.lk.RLock()
	defer m.lk.RUnlock()
	of, ok := m.orders[userID]
	if ok {
		oi := &api.OrderJobInfo{
			ID: userID,

			AvailTime:  of.availTime,
			Nonce:      of.nonce,
			OrderTime:  of.orderTime,
			OrderState: string(of.orderState),

			SeqNum:   of.seqNum,
			SeqTime:  of.seqTime,
			SeqState: string(of.seqState),

			Ready:  of.ready,
			InStop: of.pause,
		}

		return oi, nil
	}

	return nil, xerrors.Errorf("not found")
}

func (m *OrderMgr) OrderGetDetail(ctx context.Context, userID, nonce uint64, seqNum uint32) (*types.SignedOrderSeq, error) {

	key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, userID, nonce, seqNum)
	val, err := m.ds.Get(key)
	if err != nil {
		return nil, err
	}

	sos := new(types.SignedOrderSeq)
	err = sos.Deserialize(val)
	if err != nil {
		return nil, err
	}

	return sos, nil
}

func (m *OrderMgr) OrderGetPayInfoAt(ctx context.Context, pid uint64) (*types.OrderPayInfo, error) {
	pi := new(types.OrderPayInfo)
	if pid == 0 {
		pi.Size = m.di.Size - m.di.ExpireSize // may be less than
		pi.ConfirmSize = m.di.ConfirmSize - m.di.ExpireSize
		pi.OnChainSize = m.di.OnChainSize
		pi.NeedPay = new(big.Int).Set(m.di.NeedPay)
		pi.Paid = new(big.Int).Set(m.di.Paid)

		bi, err := m.is.SettleGetBalanceInfo(ctx, m.localID)
		if err != nil {
			return nil, err
		}
		pi.Balance = new(big.Int).Set(bi.ErcValue)
		pi.Balance.Add(pi.Balance, bi.FsValue)
	} else {
		m.lk.RLock()
		of, ok := m.orders[pid]
		if ok {
			pi.ID = pid
			pi.Size = of.di.Received - of.di.ExpireSize
			pi.ConfirmSize = of.di.ConfirmSize - of.di.ExpireSize
			pi.OnChainSize = of.di.OnChainSize
			pi.NeedPay = new(big.Int).Set(of.di.NeedPay)
			pi.Paid = new(big.Int).Set(of.di.Paid)

			if of.seq != nil {
				pi.Size += of.seq.Size
			} else {
				if of.base != nil {
					pi.Size += of.base.Size
				}
			}
		}
		m.lk.RUnlock()
	}

	return pi, nil
}

func (m *OrderMgr) OrderGetPayInfo(ctx context.Context) ([]*types.OrderPayInfo, error) {
	res := make([]*types.OrderPayInfo, 0, len(m.users))

	for _, pid := range m.users {
		oi, err := m.OrderGetPayInfoAt(ctx, pid)
		if err != nil {
			continue
		}

		res = append(res, oi)
	}

	return res, nil
}
