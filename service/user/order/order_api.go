package order

import (
	"context"

	"github.com/memoio/go-mefs-v2/api"
	"golang.org/x/xerrors"
)

func (m *OrderMgr) OrderListPros(_ context.Context) ([]uint64, error) {
	res := make([]uint64, 0, len(m.pros))
	res = append(res, m.pros...)
	return res, nil
}

func (m *OrderMgr) OrderGetInfo(ctx context.Context) ([]*api.OrderInfo, error) {
	res := make([]*api.OrderInfo, 0, len(m.pros))

	for _, pid := range m.pros {
		oi, err := m.OrderGetInfoAt(ctx, pid)
		if err != nil {
			continue
		}

		res = append(res, oi)
	}

	return res, nil
}

func (m *OrderMgr) OrderGetInfoAt(_ context.Context, proID uint64) (*api.OrderInfo, error) {
	of, ok := m.orders[proID]
	if ok {
		oi := &api.OrderInfo{
			ProID: proID,

			AvailTime:  of.availTime,
			Nonce:      of.nonce,
			OrderTime:  of.orderTime,
			OrderState: string(of.orderState),

			SeqNum:   of.seqNum,
			SeqTime:  of.seqTime,
			SeqState: string(of.seqState),

			Ready:  of.ready,
			InStop: of.inStop,
		}

		return oi, nil
	}

	return nil, xerrors.Errorf("not found")
}
