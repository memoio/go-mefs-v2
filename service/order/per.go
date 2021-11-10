package order

import (
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/lib/types"
)

// per provider
type OrderPer struct {
	sync.Mutex
	pro      uint64
	current  *OrderOne
	nonce    uint64 // next nonce
	optime   time.Time
	dataName [][]byte // buf
}

// get its next nonce
func (o *OrderPer) getNext() uint64 {
	return o.nonce
}

func (o *OrderPer) addSeg(seg types.Segs) {

}

func (o *OrderPer) check() {
	// 1. done -> new

	// 2  closing; push seq to done

	// 3  running;
	// 3.1 seq Init,  check to running
	// 3.2 seq running wait confirm or resending;
	// 3.3 seq confirm wait done
	// 3.4 seq done -> new seq

	// 4 init; wait confirm
}

// create a new order
func (o *OrderPer) new(ob *types.OrderBase) (*OrderOne, error) {
	if o.current == nil {
		of := &OrderOne{
			StateOrderBase: StateOrderBase{
				OrderBase: *ob,
				State:     Order_Init,
			},
		}

		of.Nonce = o.nonce
		o.current = of
		o.nonce++
		o.optime = time.Now()
		return of, nil
	}

	return nil, ErrState
}

// confirm base when receive pro ack; init -> running
func (o *OrderPer) confirm(ob *types.OrderBase) error {
	// state:->confirm
	if o.current != nil {
		// nonce is not right
		if o.current.Nonce != ob.Nonce {
			return ErrState
		}

		// state is wrong
		if o.current.State == Order_Init {
			return ErrState
		}

		o.current.State = Order_Running
		o.optime = time.Now()
		return nil
	}

	return ErrState
}

func (o *OrderPer) close() error {
	if o.current != nil && o.current.State != Order_Done {
		o.optime = time.Now()
		o.current.State = Order_Closing
		// save
	}

	return ErrState
}

func (o *OrderPer) finish() error {
	if o.current != nil {
		if o.current.State != Order_Closing {
			return nil
		}
	}

	// verify current has done all seq
	o.current.State = Order_Done
	o.optime = time.Now()

	// save

	o.current = nil

	// trigger a new order

	return nil
}
