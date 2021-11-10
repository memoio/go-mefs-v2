package order

import (
	"errors"
	"math/big"
	"time"

	"github.com/memoio/go-mefs-v2/lib/types"
)

var (
	ErrDataAdded = errors.New("add data fails")
	ErrState     = errors.New("state is wrong")
)

type OrderState uint8

const (
	Order_Init    OrderState = iota
	Order_Running            // when recieve pro ack
	Order_Closing            // no new seq
	Order_Done               // all seq done
)

type NonceState struct {
	Nonce uint64
	State OrderState
}

type StateOrderBase struct {
	types.OrderBase
	State OrderState // 状态
}

type OrderSeqState uint8

const (
	OrderSeq_Init    OrderSeqState = iota //  collect data when order is running; -> runnning when size is enough or time is up and size > 0
	OrderSeq_Running                      // sent to pro
	OrderSeq_Sending                      // seq confirm;  when receive pro ack
	OrderSeq_Done                         // seq done;
)

type SeqState struct {
	Number uint32
	State  OrderSeqState
}

type StateOrderSeq struct {
	types.OrderSeq
	State OrderSeqState // 状态
}

// Next -> Current
// Current -> Prev when pro get all data
type OrderOne struct {
	StateOrderBase           // quotation-> base
	seqtime        time.Time // time of next seq
	seq            uint32    // next seq
	size           uint64
	price          *big.Int
	active         *StateOrderSeq // -> running
}

// add data to prepare
func (f *OrderOne) addSeg(d types.Segs) error {
	if f.State != Order_Running {
		return ErrState
	}

	f.new()
	if f.active != nil && f.active.State == OrderSeq_Init {
		f.active.Segments = append(f.active.Segments, d)
		// update size and price
		f.active.Size += 0
		f.active.Price.Add(f.active.Price, big.NewInt(0))
		return nil
	}

	return ErrDataAdded
}

// create a new orderseq for prepare
func (f *OrderOne) new() {
	if f.State != Order_Running {
		return
	}

	if f.active == nil {
		id := f.OrderBase.GetShortHash()

		s := &StateOrderSeq{
			State: OrderSeq_Init,
			OrderSeq: types.OrderSeq{
				ID:     id,
				SeqNum: f.seq,
				Price:  new(big.Int).Set(f.price),
				Size:   f.size,
			},
		}
		f.seqtime = time.Now()
		f.seq++
		f.active = s
	}
}

// init -> ruuning; when receive confirm ack
func (f *OrderOne) run() {
	if f.active != nil && f.active.State == OrderSeq_Init {
		f.active.State = OrderSeq_Running
		f.seqtime = time.Now()
		f.size = f.active.Size
		f.price.Set(f.active.Price)
		// send to pro
	}
}

// when recieve pro seq ack; active: running -> sending
func (f *OrderOne) confirm(s *types.OrderSeq) {
	if f.active == nil {
		return
	}

	if f.active.SeqNum != s.SeqNum {
		return
	}

	// verify sig
	f.active.ProDataSig = s.ProDataSig
	f.active.ProSig = s.ProSig

	f.active.State = OrderSeq_Sending
	f.seqtime = time.Now()
}

// when recieve pro seq done ack; confirm -> done
func (f *OrderOne) finish() {
	if f.active == nil {
		return
	}

	if f.active.State != OrderSeq_Sending {
		return
	}

	// send usersig to pro
	f.active.State = OrderSeq_Done
	f.seqtime = time.Now()
	// persist

	f.active = nil

	// trigger new()
}
