package order

import (
	"context"

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

func (m *OrderMgr) OrderGetPayInfo(ctx context.Context) ([]*types.OrderPayInfo, error) {
	res := make([]*types.OrderPayInfo, 0)
	return res, nil
}

func (m *OrderMgr) OrderGetPayInfoAt(ctx context.Context, id uint64) (*types.OrderPayInfo, error) {
	return new(types.OrderPayInfo), nil
}
