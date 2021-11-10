package user

import (
	"context"
	"math/big"
	"time"

	"github.com/memoio/go-mefs-v2/lib/types"
)

type OrderState uint8

const (
	Order_Init    OrderState = iota
	Order_Confirm            // when recieve pro ack
	Order_Running            // previous is done
	Order_Closing            // no new seq
	Order_Done               // all seq done
)

type StateOrderBase struct {
	types.OrderBase
	State OrderState // 状态
}

type OrderSeqState uint8

const (
	OrderSeq_Init    OrderSeqState = iota //  collect data when order is running; -> runnning when size is enough or time is up and size > 0
	OrderSeq_Running                      // sent to pro
	OrderSeq_Confirm                      // seq confirm;  when receive pro ack
	OrderSeq_Done                         // seq done;
)

type StateOrderSeq struct {
	types.OrderSeq
	State OrderSeqState // 状态
}

// Next -> Current
// Current -> Prev when pro get all data
type OrderFull struct {
	StateOrderBase
	seqtime int64  // time of next seq
	seq     uint32 // next seq
	size    uint64
	price   *big.Int
	active  *StateOrderSeq // -> running
	prepare *StateOrderSeq //
}

// create a new orderseq for prepare
func (f *OrderFull) addOrderSeq() {
	if f.State == Order_Running {

		if f.prepare == nil {
			id := f.OrderBase.GetShortHash()

			s := &StateOrderSeq{
				OrderSeq: types.OrderSeq{
					ID:    id,
					Price: new(big.Int).Set(f.price),
					Size:  f.size,
				},
				State: OrderSeq_Init,
			}
			s.SeqNum = f.seq
			f.prepare = s
		}
	}
}

// change prepare to active when active done
func (f *OrderFull) toActive() {
	if f.prepare != nil && f.prepare.State == OrderSeq_Init {
		f.active = f.prepare
		f.prepare = nil
		f.seq++
		f.active.State = OrderSeq_Running
		f.size = f.active.Size
		f.price.Set(f.active.Price)

		f.addOrderSeq()

		// send to pro
	}
}

// add data to prepare
func (f *OrderFull) addData(d []byte) {
	if f.prepare == nil {
		f.addOrderSeq()
	}

	if f.State == Order_Running && f.prepare != nil && f.prepare.State == OrderSeq_Init {
		//f.prepare.Name = append(f.prepare.Name, d)
		// update size and price
		f.prepare.Size += 0
		f.prepare.Price.Add(f.prepare.Price, big.NewInt(0))
	}
}

// when recieve pro ack
func (f *OrderFull) confirmSeq(s *types.OrderSeq) {
	if f.active == nil {
		return
	}

	if f.active.SeqNum != s.SeqNum {
		return
	}

	// verify sig
	f.active.ProDataSig = s.ProDataSig
	f.active.ProSig = s.ProSig

	f.active.State = OrderSeq_Confirm
}

func (f *OrderFull) doneSeq() {
	if f.active == nil {
		return
	}

	if f.active.State != OrderSeq_Confirm {
		return
	}

	// send usersig to pro
	f.active.State = OrderSeq_Done
	// persist

	// trigger toActive
}

type OrderOne struct {
	pro      uint64
	current  *OrderFull
	next     *OrderFull
	latest   *types.Quotation // lastest quotation
	nonce    uint64           // next nonce
	dataName [][]byte         // buf
}

// get its next nonce
func (o *OrderOne) getNext() uint64 {
	return o.nonce
}

// create a new order
func (o *OrderOne) addOrderBase(of *OrderFull) error {
	if o.next == nil {
		of.Nonce = o.nonce
		o.next = of
		return nil
	}

	// verify next

	return nil
}

// confirm base when receive pro ack
func (o *OrderOne) confirmOrderBase(ob *types.OrderBase) error {
	// state:->confirm
	if o.next != nil {
		// nonce is not right
		if o.next.Nonce != ob.Nonce {
			return nil
		}

		// state is wrong
		if o.next.State == Order_Init {
			return nil
		}

		o.current.State = Order_Confirm
		return nil
	}

	return nil
}

// set state
func (o *OrderOne) setState(st OrderState) error {
	if o.current != nil {
		o.current.State = st
	}

	return nil
}

// confirmed next and done current; then change next to current
// next when time is up
func (o *OrderOne) toCurrent() error {
	if o.current != nil {
		if o.current.State != Order_Done {
			return nil
		}
	}

	if o.next != nil && o.next.State == Order_Confirm {
		o.current = o.next
		o.nonce++
		o.next = nil
	}

	return nil
}

func (o *OrderOne) done() error {
	if o.current != nil {
		if o.current.State != Order_Closing {
			return nil
		}
	}

	// verify current has done all seq

	o.current.State = Order_Done

	return nil
}

type OrderMgr struct {
	// ds for store
	// net for send
	localID uint64
	orders  map[uint64]*OrderOne // key: proID
	direct  map[types.OrderHash]*OrderFull

	pros []uint64   // all pros
	last [][]uint64 // chunkID -> proID per bucket

	data [][]byte // for cache

	dataChan     chan []byte
	newOrderChan chan *types.Quotation
	confirmChan  chan *types.OrderBase
	seqChan      chan *types.OrderSeq
	proChan      chan *OrderOne
}

// add ds + network
func NewOrderMgr() *OrderMgr {
	return &OrderMgr{
		data:         make([][]byte, 16),
		dataChan:     make(chan []byte, 10),
		newOrderChan: make(chan *types.Quotation, 10),
	}
}

// quotation -> base
func (m *OrderMgr) addOrderBase(quo *types.Quotation) error {
	ob := types.OrderBase{
		UserID:     m.localID,
		ProID:      quo.ProID,
		TokenIndex: quo.TokenIndex,
		SegPrice:   quo.SegPrice,
		PiecePrice: quo.PiecePrice,
		Start:      uint64(time.Now().Unix()),
		End:        uint64(time.Now().Unix()) + 100,
	}

	of := &OrderFull{
		StateOrderBase: StateOrderBase{
			OrderBase: ob,
			State:     Order_Init,
		},
	}

	// state:init
	or, ok := m.orders[quo.ProID]
	if ok {
		// non-exist handle
		// get current nonce of proID; and verify
		nonce := uint64(0)
		or = &OrderOne{
			nonce: nonce + 1,
		}

		m.orders[quo.ProID] = or
	}

	or.addOrderBase(of)
	m.direct[ob.GetShortHash()] = of

	return nil
}

// confirm base
func (m *OrderMgr) confirmOrderBase(ob *types.OrderBase) error {
	// state:->confirm
	or, ok := m.orders[ob.ProID]
	if !ok {
		return nil
	}

	or.confirmOrderBase(ob)

	return nil
}

// add seg to proID depends on its chunkID
// add piece to random number of proID
func (m *OrderMgr) addData(dataName []byte) {
	m.data = append(m.data, dataName)
}

func (m *OrderMgr) runSched(ctx context.Context) {
	ticket := time.NewTicker(time.Minute)
	for {
		select {
		case <-ticket.C:
			// handerRunning?
			// handleDone
		// add data
		case data := <-m.dataChan:
			m.addData(data)
		case quo := <-m.newOrderChan:
			m.addOrderBase(quo)
		case ob := <-m.confirmChan:
			m.confirmOrderBase(ob)
		case s := <-m.seqChan:
			f := m.direct[s.ID]
			f.confirmSeq(s)
		case p := <-m.proChan:
			m.orders[p.pro] = p
		case <-ctx.Done():
			return
		}
	}
}

// add a new pro
func (m *OrderMgr) addPro(pro uint64, nonce uint64) {
	// get enough information; and trigger
	o := &OrderOne{
		pro:   pro,
		nonce: nonce,
	}
	m.proChan <- o
}

// get from pro
func (m *OrderMgr) getQuotation(pro uint64) {
	// over network
	quo := new(types.Quotation)
	m.newOrderChan <- quo
}

// receive ack from pro
func (m *OrderMgr) handleConfirmOrder(ob *types.OrderBase) {
	m.confirmChan <- ob
}

func (m *OrderMgr) handleOrderSeqDone(s *types.OrderSeq) {
	m.seqChan <- s
}

func (m *OrderMgr) handleAddData(d []byte) {
	m.dataChan <- d
}

// get information
