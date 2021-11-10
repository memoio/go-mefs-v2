package provider

import (
	"github.com/memoio/go-mefs-v2/lib/types"
)

// current nonce
// currnet seq

type OrderFull struct {
	types.OrderBase
	current  *types.OrderSeq // 当前处理
	nextSeq  uint32          // nextSeq
	complete int             // data completed
}

type OrderOne struct {
	active *OrderFull
	next   *OrderFull
	nonce  uint64 // current nonce
}

// getdata from each user
func (o *OrderOne) getData(userID uint64) {
	for {
		dn := o.active.current.Pieces
		if o.active.complete < len(dn) {
			d := dn[o.active.complete]
			// get from user and store in local
			err := GetData(userID, d)
			if err == nil {
				o.active.complete++
			}
		}
	}
}

func GetData(user uint64, dn []byte) error {
	// over network
	return nil
}

type OrderMgr struct {
	local  uint64
	orders map[uint64]*OrderOne // key: userID
	direct map[types.OrderHash]*OrderFull
}

// handleNewOrder adds orderBase
func (m *OrderMgr) handleNewOrder(ob *types.OrderBase) error {
	or, ok := m.orders[ob.UserID]
	if !ok {

		of := &OrderFull{
			OrderBase: *ob,
		}
		or := &OrderOne{
			active: of,
		}
		m.orders[ob.UserID] = or
	}

	if or.active == nil {
		return nil
	} else {
		// next is nil
		// Nonce is nonce+1
		// start >= active.start
		// end >= active.end
		if or.next != nil {
			return nil
		}
	}
	return nil
}

// currnt done -> next
func (m *OrderMgr) handleNewOrderSeq(ob *types.OrderSeq) error {
	of, ok := m.direct[ob.ID]
	if !ok {
		return nil
	}

	if ob.SeqNum != of.nextSeq {
		return nil
	}

	if of.current != nil {
		// handle currnet, state is completed
	}

	of.current = ob
	of.nextSeq++
	return nil
}
