package order

import (
	"context"
	"encoding/binary"
	"math/big"
	"time"

	"github.com/emirpasic/gods/sets/hashset"
	"github.com/fxamacker/cbor/v2"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type OrderMgr struct {
	api.IRole
	api.INetService
	api.IDataService

	localID uint64
	fsID    []byte

	ds store.KVStore // save order info

	orders map[uint64]*OrderPer          // key: proID
	direct map[types.OrderHash]*OrderOne // key: orderID

	proSet *hashset.Set             // all pros
	proMap map[uint64]*lastProvider // key: bucketID

	pieceChan chan []byte
	segChan   chan *types.Segs

	newOrderChan chan uint64
	quoChan      chan *types.Quotation
	confirmChan  chan *types.OrderBase
	seqChan      chan *types.OrderSeq
}

func NewOrderMgr(ctx context.Context, roleID uint64, ds store.KVStore, ir api.IRole, id api.IDataService) *OrderMgr {
	proset := hashset.New()

	om := &OrderMgr{
		IRole:        ir,
		IDataService: id,
		proSet:       proset,
		pieceChan:    make(chan []byte, 16),
		segChan:      make(chan *types.Segs, 16),
		newOrderChan: make(chan uint64, 16),
		quoChan:      make(chan *types.Quotation, 16),
		confirmChan:  make(chan *types.OrderBase, 16),
		seqChan:      make(chan *types.OrderSeq, 16),
	}

	om.load(ctx)
	om.addPros()

	go om.runSched(ctx)

	return om
}

func (m *OrderMgr) load(ctx context.Context) error {
	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	val, err := m.ds.Get(key)
	if err != nil {
		return err
	}
	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		m.proSet.Add(pid)
		m.loadpro(pid)
	}

	return nil
}

func (m *OrderMgr) save(ctx context.Context) error {
	proVal := m.proSet.Values()
	buf := make([]byte, 8*len(proVal))
	for i := 0; i < len(proVal); i++ {
		binary.BigEndian.PutUint64(buf[8*i:8*(i+1)], proVal[i].(uint64))
	}

	key := store.NewKey(pb.MetaType_OrderProsKey, m.localID)
	return m.ds.Put(key, buf)
}

func (m *OrderMgr) addPros() {
	pros := m.IRole.RoleGetRelated(pb.RoleInfo_Provider)
	for _, pro := range pros {
		has := m.proSet.Contains(pro)
		if !has {
			m.loadpro(pro)
			m.proSet.Add(pro)
		}
	}
}

func (m *OrderMgr) removePro() {
	// remove it if not connected for a long time
}

func (m *OrderMgr) loadpro(id uint64) *OrderPer {
	op := &OrderPer{
		pro: id,
	}

	m.orders[id] = op

	ns := new(NonceState)
	key := store.NewKey(pb.MetaType_OrderNonceKey, m.localID, id)
	val, err := m.ds.Get(key)
	if err != nil {
		return op
	}

	err = cbor.Unmarshal(val, ns)
	if err != nil {
		return op
	}

	if ns.State == Order_Done {
		op.nonce++
		return op
	}

	op.nonce = ns.Nonce

	ob := new(types.OrderBase)
	key = store.NewKey(pb.MetaType_OrderBaseKey, m.localID, id, op.nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}

	err = cbor.Unmarshal(val, ob)
	if err != nil {
		return op
	}

	op.current = &OrderOne{
		StateOrderBase: StateOrderBase{
			*ob, ns.State,
		},
	}

	ss := new(SeqState)
	key = store.NewKey(pb.MetaType_OrderSeqNumKey, m.localID, id, op.nonce)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}

	err = cbor.Unmarshal(val, ss)
	if err != nil {
		return op
	}

	op.current.seq = ss.Number

	os := new(types.OrderSeq)
	key = store.NewKey(pb.MetaType_OrderSeqKey, m.localID, id, op.nonce, ss.Number)
	val, err = m.ds.Get(key)
	if err != nil {
		return op
	}

	err = cbor.Unmarshal(val, os)
	if err != nil {
		return op
	}

	op.current.price = new(big.Int).Set(os.Price)
	op.current.size = os.Size
	op.current.seq = os.SeqNum

	op.current.active = &StateOrderSeq{
		OrderSeq: *os,
		State:    ss.State,
	}

	return op
}

func (m *OrderMgr) runSched(ctx context.Context) {
	st := time.NewTicker(time.Minute)
	defer st.Stop()

	lt := time.NewTicker(time.Minute)
	defer lt.Stop()
	for {
		select {
		case <-st.C:
		case <-lt.C:
			m.addPros() // add providers
			// handerRunning?
			// handleDone
		case <-m.pieceChan:
			// todo
		case <-m.segChan:
			// todo
		case id := <-m.newOrderChan:
			m.getQuotation(ctx, id)
		case quo := <-m.quoChan:
			m.addOrderBase(quo)
		case ob := <-m.confirmChan:
			m.confirmOrderBase(ob)
		case <-m.seqChan:
			// todo
		case <-ctx.Done():
			return
		}
	}
}

// add seg to proID depends on its chunkID
// add piece to random number of proID
func (m *OrderMgr) AddData(dataName []byte) {
}

func (m *OrderMgr) getQuotation(ctx context.Context, id uint64) error {
	resp, err := m.SendMetaRequest(ctx, id, pb.NetMessage_AskPrice, nil)
	if err != nil {
		return err
	}

	quo := new(types.Quotation)
	err = cbor.Unmarshal(resp.GetData().GetMsgInfo(), quo)
	if err != nil {
		return err
	}

	m.quoChan <- quo

	return nil
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

	// state:init
	or, ok := m.orders[quo.ProID]
	if ok {
		or = m.loadpro(quo.ProID)
	}

	of, err := or.new(&ob)
	if err != nil {
		return err
	}

	m.direct[ob.GetShortHash()] = of

	return nil
}

// confirm base
func (m *OrderMgr) confirmOrderBase(ob *types.OrderBase) error {
	// state:->confirm
	or, ok := m.orders[ob.ProID]
	if !ok {
		return ErrState
	}

	or.confirm(ob)

	return nil
}

// handle ack pro pro

// receive order ack; Order_Init -> Order_Running
func (m *OrderMgr) handleOrderConfirm(ob *types.OrderBase) {
	// verify sign; persist
	m.confirmChan <- ob
}

// receive seq ack; OrderSeq_Running -> OrderSeq_Sending
func (m *OrderMgr) handleSeqConfirm(s *types.OrderSeq) {
	m.seqChan <- s
}

// receive seq done ack;OrderSeq_Sending -> OrderSeq_Done
func (m *OrderMgr) handleSeqDoneConfirm(s *types.OrderSeq) {
	m.seqChan <- s
}

// get information
