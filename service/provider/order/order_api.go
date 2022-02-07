package order

import (
	"context"

	"github.com/memoio/go-mefs-v2/api"
	"golang.org/x/xerrors"
)

func (m *OrderMgr) OrderList(_ context.Context) ([]uint64, error) {
	res := make([]uint64, 0, len(m.users))
	res = append(res, m.users...)
	return res, nil
}

func (m *OrderMgr) OrderGetInfo(ctx context.Context) ([]*api.OrderInfo, error) {
	res := make([]*api.OrderInfo, 0, len(m.users))

	for _, pid := range m.users {
		oi, err := m.OrderGetInfoAt(ctx, pid)
		if err != nil {
			continue
		}

		res = append(res, oi)
	}

	return res, nil
}

func (m *OrderMgr) OrderGetInfoAt(_ context.Context, userID uint64) (*api.OrderInfo, error) {
	of, ok := m.orders[userID]
	if ok {
		oi := &api.OrderInfo{
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
