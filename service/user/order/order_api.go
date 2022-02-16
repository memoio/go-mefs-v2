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
			ID: proID,

			AvailTime:  of.availTime,
			Nonce:      of.nonce,
			OrderTime:  of.orderTime,
			OrderState: string(of.orderState),

			SeqNum:   of.seqNum,
			SeqTime:  of.seqTime,
			SeqState: string(of.seqState),

			Jobs: of.segCount(),

			Ready:  of.ready,
			InStop: of.inStop,
		}

		if of.base != nil {
			oi.Nonce = of.base.Nonce
		}

		if of.seq != nil {
			oi.SeqNum = of.seq.SeqNum
		}

		return oi, nil
	}

	return nil, xerrors.Errorf("not found")
}

func (m *OrderMgr) OrderGetDetail(ctx context.Context, proID, nonce uint64, seqNum uint32) (*types.SignedOrderSeq, error) {

	key := store.NewKey(pb.MetaType_OrderSeqKey, m.localID, proID, nonce, seqNum)
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
