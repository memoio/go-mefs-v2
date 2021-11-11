package order

import (
	"context"
	"time"

	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderFull struct {
	ds store.KVStore

	userID uint64
	fsID   []byte

	base *types.OrderBase
	seq  *types.OrderSeq // 当前处理

	nonce  uint64 // next nonce
	seqNum uint32 // next seq

	pk pdpcommon.PublicKey

	dv pdpcommon.DataVerifier
}

type SegReceived struct {
	seg segment.Segment // for verify
	uid uint64
}

type OrderMgr struct {
	local uint64

	quo *types.Quotation

	orders map[uint64]*OrderFull // key: userID

	segChan       chan *SegReceived
	orderChan     chan *types.OrderBase
	seqChan       chan *types.OrderSeq
	seqFinishChan chan *types.OrderSeq
	orderDoneChan chan *types.OrderBase
}

func (m *OrderMgr) runSched(ctx context.Context) {
	st := time.NewTicker(time.Minute)
	defer st.Stop()

	for {
		select {
		case <-st.C:
		case d := <-m.segChan:
			m.handleData(d)
		case ob := <-m.orderChan:
			m.handleNewOrder(ob)
		case seq := <-m.seqChan:
			m.handleNewSeq(seq)
		case seq := <-m.seqFinishChan:
			m.handleFinishSeq(seq)
		case ob := <-m.orderDoneChan:
			m.handleDoneOrder(ob)
		case <-ctx.Done():
			return
		}
	}
}

func (m *OrderMgr) newUserOrder(uid uint64) *OrderFull {
	// get pubkey from user
	return nil
}

func (m *OrderMgr) handleData(d *SegReceived) error {
	or, ok := m.orders[d.uid]
	if !ok {

		m.orders[d.uid] = or
	}

	return nil
}

func (m *OrderMgr) handleNewOrder(ob *types.OrderBase) error {
	or, ok := m.orders[ob.UserID]
	if !ok {

		m.orders[ob.UserID] = or
	}

	return nil
}

func (m *OrderMgr) handleDoneOrder(ob *types.OrderBase) error {
	or, ok := m.orders[ob.UserID]
	if !ok {

		m.orders[ob.UserID] = or
	}

	return nil
}

func (m *OrderMgr) handleNewSeq(os *types.OrderSeq) error {

	return nil
}

func (m *OrderMgr) handleFinishSeq(os *types.OrderSeq) error {

	return nil
}
